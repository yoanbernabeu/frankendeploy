package deploy

import "fmt"

// DeployPhase represents a phase in the deployment process.
type DeployPhase int

const (
	PhaseInit DeployPhase = iota
	PhasePrepareRelease
	PhaseStartNewContainer
	PhasePreDeployHooks
	PhaseHealthCheck
	PhaseSwapContainers
	PhasePostDeployHooks
	PhaseCleanup
	PhaseDone
)

func (p DeployPhase) String() string {
	switch p {
	case PhaseInit:
		return "init"
	case PhasePrepareRelease:
		return "prepare-release"
	case PhaseStartNewContainer:
		return "start-new-container"
	case PhasePreDeployHooks:
		return "pre-deploy-hooks"
	case PhaseHealthCheck:
		return "health-check"
	case PhaseSwapContainers:
		return "swap-containers"
	case PhasePostDeployHooks:
		return "post-deploy-hooks"
	case PhaseCleanup:
		return "cleanup"
	case PhaseDone:
		return "done"
	default:
		return fmt.Sprintf("unknown(%d)", int(p))
	}
}

// DeployState tracks the current state of a deployment for rollback decisions.
type DeployState struct {
	Phase              DeployPhase
	AppName            string
	TempContainerName  string
	OldContainerExists bool
}

// NewDeployState creates a new deploy state for the given app.
func NewDeployState(appName string) *DeployState {
	return &DeployState{
		Phase:             PhaseInit,
		AppName:           appName,
		TempContainerName: appName + "-new",
	}
}

// RollbackActions returns the commands needed to rollback from the current phase.
// The goal is to restore the previous state: if the old container was running,
// it stays running. If a new temp container was started, it gets removed.
func (s *DeployState) RollbackActions() []string {
	var actions []string

	switch {
	case s.Phase >= PhaseStartNewContainer && s.Phase < PhaseSwapContainers:
		// New container was started with temp name, old is still running.
		// Just remove the new temp container.
		actions = append(actions,
			fmt.Sprintf("docker stop %s 2>/dev/null || true", s.TempContainerName),
			fmt.Sprintf("docker rm %s 2>/dev/null || true", s.TempContainerName),
		)

	case s.Phase >= PhaseSwapContainers:
		// Swap already happened: old container is gone, new container is renamed.
		// To rollback we'd need to restart the previous release image.
		// This is handled by the rollback command, not inline.
		actions = append(actions,
			fmt.Sprintf("docker stop %s 2>/dev/null || true", s.AppName),
			fmt.Sprintf("docker rm %s 2>/dev/null || true", s.AppName),
		)
	}

	return actions
}
