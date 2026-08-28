package deploy

import "fmt"

// Logger abstracts user-facing output so the orchestration can live outside
// the cmd package and be tested without printing.
type Logger interface {
	Info(format string, args ...interface{})
	Success(format string, args ...interface{})
	Warning(format string, args ...interface{})
}

// NopLogger discards all output (tests).
type NopLogger struct{}

func (NopLogger) Info(string, ...interface{})    {}
func (NopLogger) Success(string, ...interface{}) {}
func (NopLogger) Warning(string, ...interface{}) {}

// Steps holds the injectable operations of a blue-green deployment. Each
// field is provided by the caller (production wiring in cmd/deploy.go, fakes
// in tests): the orchestration below decides ordering, rollback and
// force/skip semantics — the exact logic that was previously untestable
// inside a 1000-line runDeploy.
type Steps struct {
	// PrepareRelease creates release directories and shared volumes.
	PrepareRelease func() error
	// OldContainerExists reports whether the app container currently runs.
	OldContainerExists func() bool
	// StartNewContainer starts the new version under a temporary name.
	StartNewContainer func() error
	// BackupDatabase dumps the managed database and returns the backup path.
	BackupDatabase func() (string, error)
	// RunPreDeployHooks runs pre_deploy hooks on the NEW container.
	RunPreDeployHooks func() error
	// CheckMigrationState warns about pending/empty migrations (informational).
	CheckMigrationState func()
	// HealthCheck probes the NEW container.
	HealthCheck func() error
	// ShowContainerLogs prints the new container logs after a failed health check.
	ShowContainerLogs func()
	// SwapContainers promotes the new container to the live name.
	SwapContainers func(oldExists bool) error
	// DeployWorkers starts the Messenger worker container.
	DeployWorkers func() error
	// RunPostDeployHooks runs post_deploy hooks on the live container.
	RunPostDeployHooks func() error
	// CaddyAppConfigExists reports whether the app already has a Caddy config.
	CaddyAppConfigExists func() bool
	// UpdateCaddy writes the app Caddy config and reloads the proxy.
	UpdateCaddy func() error
	// Cleanup removes old releases and images (best-effort).
	Cleanup func()
	// RollbackNewContainer removes the temporary container after a failure.
	RollbackNewContainer func(state *DeployState)
	// WarnMigrationRollback explains that a rollback does NOT undo the schema.
	WarnMigrationRollback func(situation, backupPath string)
}

// Options carries the deployment decisions the orchestration needs.
type Options struct {
	Force             bool
	SkipHealthcheck   bool
	HasPreDeployHooks bool
	HasMigrationHook  bool
	// BackupEligible is true for a managed database with a known URL.
	BackupEligible     bool
	HasPostDeployHooks bool
	MessengerEnabled   bool
	Domain             string
	Logger             Logger
}

// RunPipeline executes the blue-green deployment orchestration: prepare →
// start temp container → pre-deploy hooks (with automatic DB backup) →
// health check → swap → workers → post-deploy hooks → Caddy → cleanup.
// Failure semantics:
//   - backup/pre-hooks/health failures roll back the temp container and abort
//     (unless --force);
//   - a swap failure rolls back and aborts (force does not apply: a failed
//     swap means the old container is still, or again, serving);
//   - worker/post-hooks failures only warn;
//   - a Caddy failure aborts on the app's FIRST public exposure (the app
//     would be running but unreachable), warns otherwise.
func RunPipeline(state *DeployState, steps Steps, opts Options) error {
	log := opts.Logger
	if log == nil {
		log = NopLogger{}
	}

	// Step 4: Prepare release directories and shared volumes
	log.Info("Preparing release...")
	state.Phase = PhasePrepareRelease
	if err := steps.PrepareRelease(); err != nil {
		return fmt.Errorf("deployment failed: %w", err)
	}

	// Check if old container exists (for swap phase)
	state.OldContainerExists = steps.OldContainerExists()

	// Step 5: Start new container with temporary name (old still running)
	log.Info("Starting new version (blue-green)...")
	state.Phase = PhaseStartNewContainer
	if err := steps.StartNewContainer(); err != nil {
		return fmt.Errorf("deployment failed: %w", err)
	}

	// Step 6: Pre-deploy hooks on the NEW container
	migrationAttempted := false
	var dbBackupPath string
	if opts.HasPreDeployHooks {
		// Step 6a: Automatic database backup before any migration. Migrations
		// run while the old code still serves traffic: if anything fails
		// afterwards, the container rollback does NOT roll the schema back —
		// the dump is the only safety net.
		if opts.HasMigrationHook && opts.BackupEligible {
			log.Info("Backing up database before migration...")
			backupPath, err := steps.BackupDatabase()
			if err != nil {
				if !opts.Force {
					log.Warning("Database backup failed, rolling back...")
					steps.RollbackNewContainer(state)
					return fmt.Errorf("database backup failed (use --force to deploy without a backup): %w", err)
				}
				log.Warning("Database backup failed but continuing (--force): %v", err)
			} else {
				dbBackupPath = backupPath
				log.Success("Database backup: %s", backupPath)
			}
		}

		log.Info("Running pre-deploy hooks...")
		state.Phase = PhasePreDeployHooks
		migrationAttempted = opts.HasMigrationHook
		if err := steps.RunPreDeployHooks(); err != nil {
			if !opts.Force {
				log.Warning("Pre-deploy hooks failed, rolling back...")
				steps.RollbackNewContainer(state)
				if opts.HasMigrationHook {
					steps.WarnMigrationRollback("The migration may have been partially applied (non-transactional DDL on MySQL/MariaDB leaves a partial schema).", dbBackupPath)
				}
				return fmt.Errorf("pre-deploy hooks failed: %w", err)
			}
			log.Warning("Pre-deploy hooks failed but continuing (--force)")
		}

		if opts.HasMigrationHook {
			steps.CheckMigrationState()
		}
	}

	// Step 7: Health check on the NEW container (old still running = zero downtime)
	if opts.SkipHealthcheck {
		log.Warning("Health check skipped (--skip-healthcheck)")
	} else {
		log.Info("Running health check...")
		state.Phase = PhaseHealthCheck
		if err := steps.HealthCheck(); err != nil {
			steps.ShowContainerLogs()
			if !opts.Force {
				log.Warning("Health check failed, rolling back...")
				steps.RollbackNewContainer(state)
				if migrationAttempted {
					steps.WarnMigrationRollback("The database was already migrated during this deploy.", dbBackupPath)
				}
				return fmt.Errorf("deployment failed health check: %w", err)
			}
			log.Warning("Health check failed but continuing (--force)")
		} else {
			log.Success("Health check passed")
		}
	}

	// Step 8: Swap containers
	log.Info("Swapping containers...")
	state.Phase = PhaseSwapContainers
	if err := steps.SwapContainers(state.OldContainerExists); err != nil {
		steps.RollbackNewContainer(state)
		if migrationAttempted {
			steps.WarnMigrationRollback("The database was already migrated during this deploy.", dbBackupPath)
		}
		return fmt.Errorf("swap failed: %w", err)
	}

	// Step 8b: Messenger worker
	if opts.MessengerEnabled {
		log.Info("Starting Messenger worker...")
		if err := steps.DeployWorkers(); err != nil {
			log.Warning("Failed to start Messenger worker: %v", err)
		} else {
			log.Success("Messenger worker started")
		}
	}

	// Step 9: Post-deploy hooks (the new version is live: failures only warn)
	state.Phase = PhasePostDeployHooks
	if opts.HasPostDeployHooks {
		log.Info("Running post-deploy hooks...")
		if err := steps.RunPostDeployHooks(); err != nil {
			log.Warning("Post-deploy hooks failed: %v", err)
		}
	}

	// Step 10: Caddy. On the app's FIRST public exposure this is THE
	// condition for reachability: failing must fail the deploy instead of
	// printing a success message with an unreachable https URL.
	firstExposure := opts.Domain != "" && !steps.CaddyAppConfigExists()
	log.Info("Updating reverse proxy...")
	if err := steps.UpdateCaddy(); err != nil {
		if firstExposure {
			return fmt.Errorf("reverse proxy configuration failed — the application is running on the server but NOT publicly reachable: %w", err)
		}
		log.Warning("Failed to update Caddy: %v", err)
	}

	// Step 11: Cleanup (best-effort)
	state.Phase = PhaseCleanup
	log.Info("Cleaning up old releases...")
	steps.Cleanup()
	state.Phase = PhaseDone

	return nil
}
