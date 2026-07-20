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

// RollbackActions returns the commands undoing a failed deploy: removing the
// temporary container. It must NEVER target the live app name — during the
// swap phase a failure leaves either the old container (restored by
// swapContainers) or the new one serving under the app name, and past the
// swap the new version is live.
func (s *DeployState) RollbackActions() []string {
	if s.Phase >= PhaseStartNewContainer && s.Phase <= PhaseSwapContainers {
		return []string{
			fmt.Sprintf("docker stop %s 2>/dev/null || true", s.TempContainerName),
			fmt.Sprintf("docker rm %s 2>/dev/null || true", s.TempContainerName),
		}
	}
	return nil
}
