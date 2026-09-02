package deploy

import (
	"errors"
	"strings"
	"testing"
)

// fakeSteps builds a Steps where every operation succeeds and records its
// name; individual tests override the operations they want to fail.
type stepRecorder struct {
	calls      []string
	rolledBack bool
	warnedMig  []string
}

func (r *stepRecorder) record(name string) { r.calls = append(r.calls, name) }

func (r *stepRecorder) called(name string) bool {
	for _, c := range r.calls {
		if c == name {
			return true
		}
	}
	return false
}

func makeSteps(r *stepRecorder) Steps {
	return Steps{
		PrepareRelease:       func() error { r.record("prepare"); return nil },
		OldContainerExists:   func() bool { r.record("old-exists"); return true },
		StartNewContainer:    func() error { r.record("start"); return nil },
		BackupDatabase:       func() (string, error) { r.record("backup"); return "/backups/dump.sql.gz", nil },
		RunPreDeployHooks:    func() error { r.record("pre-hooks"); return nil },
		CheckMigrationState:  func() { r.record("migration-check") },
		HealthCheck:          func() error { r.record("health"); return nil },
		ShowContainerLogs:    func() { r.record("logs") },
		SwapContainers:       func(oldExists bool) error { r.record("swap"); return nil },
		DeployWorkers:        func() error { r.record("workers"); return nil },
		IsolateNetwork:       func() { r.record("isolate-network") },
		RunPostDeployHooks:   func() error { r.record("post-hooks"); return nil },
		CaddyAppConfigExists: func() bool { r.record("caddy-exists"); return true },
		UpdateCaddy:          func() error { r.record("caddy"); return nil },
		Cleanup:              func() { r.record("cleanup") },
		RollbackNewContainer: func(state *DeployState) { r.record("rollback"); r.rolledBack = true },
		WarnMigrationRollback: func(situation, backupPath string) {
			r.record("warn-migration")
			r.warnedMig = append(r.warnedMig, situation)
		},
	}
}

func fullOptions() Options {
	return Options{
		HasPreDeployHooks:  true,
		HasMigrationHook:   true,
		BackupEligible:     true,
		HasPostDeployHooks: true,
		MessengerEnabled:   true,
		Domain:             "example.com",
	}
}

func TestRunPipeline_HappyPathOrder(t *testing.T) {
	r := &stepRecorder{}
	state := NewDeployState("myapp")

	if err := RunPipeline(state, makeSteps(r), fullOptions()); err != nil {
		t.Fatalf("RunPipeline: %v", err)
	}

	want := []string{"prepare", "old-exists", "start", "backup", "pre-hooks", "migration-check", "health", "swap", "workers", "isolate-network", "post-hooks", "caddy-exists", "caddy", "cleanup"}
	got := strings.Join(r.calls, ",")
	if got != strings.Join(want, ",") {
		t.Errorf("unexpected step order:\n got %s\nwant %s", got, strings.Join(want, ","))
	}
	if state.Phase != PhaseDone {
		t.Errorf("expected PhaseDone, got %s", state.Phase)
	}
}

func TestRunPipeline_PrepareFailureAborts(t *testing.T) {
	r := &stepRecorder{}
	steps := makeSteps(r)
	steps.PrepareRelease = func() error { return errors.New("disk full") }

	err := RunPipeline(NewDeployState("myapp"), steps, fullOptions())
	if err == nil {
		t.Fatal("expected error")
	}
	if r.called("start") {
		t.Error("must not start a container after a failed prepare")
	}
}

func TestRunPipeline_BackupFailureRollsBack(t *testing.T) {
	r := &stepRecorder{}
	steps := makeSteps(r)
	steps.BackupDatabase = func() (string, error) { return "", errors.New("dump failed") }

	err := RunPipeline(NewDeployState("myapp"), steps, fullOptions())
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected backup error mentioning --force, got %v", err)
	}
	if !r.rolledBack {
		t.Error("temp container must be rolled back after a failed backup")
	}
	if r.called("pre-hooks") {
		t.Error("pre-deploy hooks (migrations!) must not run without a backup")
	}
}

func TestRunPipeline_BackupFailureForceContinues(t *testing.T) {
	r := &stepRecorder{}
	steps := makeSteps(r)
	steps.BackupDatabase = func() (string, error) { return "", errors.New("dump failed") }
	opts := fullOptions()
	opts.Force = true

	if err := RunPipeline(NewDeployState("myapp"), steps, opts); err != nil {
		t.Fatalf("--force must continue after a failed backup, got %v", err)
	}
	if !r.called("swap") {
		t.Error("expected the deploy to reach the swap with --force")
	}
}

func TestRunPipeline_PreHookFailureRollsBackAndWarnsMigration(t *testing.T) {
	r := &stepRecorder{}
	steps := makeSteps(r)
	steps.RunPreDeployHooks = func() error { return errors.New("migration exploded") }

	err := RunPipeline(NewDeployState("myapp"), steps, fullOptions())
	if err == nil {
		t.Fatal("expected error")
	}
	if !r.rolledBack {
		t.Error("expected rollback")
	}
	if len(r.warnedMig) == 0 || !strings.Contains(r.warnedMig[0], "partially applied") {
		t.Errorf("expected the partial-migration warning, got %v", r.warnedMig)
	}
	if r.called("health") {
		t.Error("health check must not run after failed pre-hooks")
	}
}

func TestRunPipeline_HealthFailureShowsLogsAndRollsBack(t *testing.T) {
	r := &stepRecorder{}
	steps := makeSteps(r)
	steps.HealthCheck = func() error { return errors.New("503") }

	err := RunPipeline(NewDeployState("myapp"), steps, fullOptions())
	if err == nil {
		t.Fatal("expected error")
	}
	if !r.called("logs") {
		t.Error("container logs must be shown on health check failure")
	}
	if !r.rolledBack {
		t.Error("expected rollback")
	}
	// Migration already ran: the user must be told the schema is not rolled back
	if len(r.warnedMig) == 0 || !strings.Contains(r.warnedMig[0], "already migrated") {
		t.Errorf("expected the already-migrated warning, got %v", r.warnedMig)
	}
	if r.called("swap") {
		t.Error("must not swap after a failed health check")
	}
}

func TestRunPipeline_SkipHealthcheck(t *testing.T) {
	r := &stepRecorder{}
	opts := fullOptions()
	opts.SkipHealthcheck = true

	if err := RunPipeline(NewDeployState("myapp"), makeSteps(r), opts); err != nil {
		t.Fatalf("RunPipeline: %v", err)
	}
	if r.called("health") {
		t.Error("health check must be skipped with --skip-healthcheck")
	}
	if !r.called("swap") {
		t.Error("swap must still happen")
	}
}

func TestRunPipeline_HealthFailureForceContinues(t *testing.T) {
	r := &stepRecorder{}
	steps := makeSteps(r)
	steps.HealthCheck = func() error { return errors.New("503") }
	opts := fullOptions()
	opts.Force = true

	if err := RunPipeline(NewDeployState("myapp"), steps, opts); err != nil {
		t.Fatalf("--force must continue after failed health check, got %v", err)
	}
	if r.rolledBack {
		t.Error("no rollback with --force")
	}
	if !r.called("swap") {
		t.Error("expected swap with --force")
	}
}

func TestRunPipeline_SwapFailureRollsBack(t *testing.T) {
	r := &stepRecorder{}
	steps := makeSteps(r)
	steps.SwapContainers = func(bool) error { return errors.New("rename failed") }

	err := RunPipeline(NewDeployState("myapp"), steps, fullOptions())
	if err == nil || !strings.Contains(err.Error(), "swap failed") {
		t.Fatalf("expected swap error, got %v", err)
	}
	if !r.rolledBack {
		t.Error("expected rollback after swap failure")
	}
	if r.called("caddy") {
		t.Error("Caddy must not be updated after a failed swap")
	}
}

func TestRunPipeline_WorkerAndPostHookFailuresOnlyWarn(t *testing.T) {
	r := &stepRecorder{}
	steps := makeSteps(r)
	steps.DeployWorkers = func() error { return errors.New("worker boom") }
	steps.RunPostDeployHooks = func() error { return errors.New("hook boom") }

	if err := RunPipeline(NewDeployState("myapp"), steps, fullOptions()); err != nil {
		t.Fatalf("worker/post-hook failures must not fail the deploy, got %v", err)
	}
	if !r.called("cleanup") {
		t.Error("cleanup must still run")
	}
}

func TestRunPipeline_CaddyFailureFirstExposureAborts(t *testing.T) {
	r := &stepRecorder{}
	steps := makeSteps(r)
	steps.CaddyAppConfigExists = func() bool { return false } // first exposure
	steps.UpdateCaddy = func() error { return errors.New("reload failed") }

	err := RunPipeline(NewDeployState("myapp"), steps, fullOptions())
	if err == nil || !strings.Contains(err.Error(), "NOT publicly reachable") {
		t.Fatalf("first-exposure Caddy failure must fail the deploy, got %v", err)
	}
}

func TestRunPipeline_CaddyFailureLaterDeployOnlyWarns(t *testing.T) {
	r := &stepRecorder{}
	steps := makeSteps(r)
	steps.UpdateCaddy = func() error { return errors.New("reload failed") }
	// CaddyAppConfigExists returns true: existing config still routes traffic

	if err := RunPipeline(NewDeployState("myapp"), makePatchedSteps(steps), fullOptions()); err != nil {
		t.Fatalf("Caddy failure on a later deploy must only warn, got %v", err)
	}
}

// makePatchedSteps is an identity helper keeping the test above readable.
func makePatchedSteps(s Steps) Steps { return s }

func TestRunPipeline_NoDomainNeverFirstExposure(t *testing.T) {
	r := &stepRecorder{}
	steps := makeSteps(r)
	steps.CaddyAppConfigExists = func() bool { return false }
	steps.UpdateCaddy = func() error { return errors.New("reload failed") }
	opts := fullOptions()
	opts.Domain = ""

	if err := RunPipeline(NewDeployState("myapp"), steps, opts); err != nil {
		t.Fatalf("without a domain a Caddy failure must only warn, got %v", err)
	}
}

func TestRunPipeline_NoHooksSkipsBackupAndHooks(t *testing.T) {
	r := &stepRecorder{}
	opts := fullOptions()
	opts.HasPreDeployHooks = false
	opts.HasPostDeployHooks = false
	opts.MessengerEnabled = false

	if err := RunPipeline(NewDeployState("myapp"), makeSteps(r), opts); err != nil {
		t.Fatalf("RunPipeline: %v", err)
	}
	for _, forbidden := range []string{"backup", "pre-hooks", "post-hooks", "workers"} {
		if r.called(forbidden) {
			t.Errorf("step %q must not run without hooks/messenger", forbidden)
		}
	}
}
