package deploy

import (
	"strings"
	"testing"
)

func TestRollbackActions_TargetsOnlyTempContainer(t *testing.T) {
	phases := []DeployPhase{PhaseStartNewContainer, PhasePreDeployHooks, PhaseHealthCheck, PhaseSwapContainers}

	for _, phase := range phases {
		state := NewDeployState("myapp")
		state.Phase = phase

		actions := state.RollbackActions()
		if len(actions) == 0 {
			t.Errorf("phase %s: expected rollback actions", phase)
			continue
		}
		for _, action := range actions {
			if !strings.Contains(action, "myapp-new") {
				t.Errorf("phase %s: rollback must target the temp container, got %q", phase, action)
			}
			// Guards the removed dead branch: rolling back must never stop or
			// remove the container serving under the live app name.
			if strings.Contains(action, "myapp ") || strings.HasSuffix(action, "myapp") {
				t.Errorf("phase %s: rollback must never target the live app container, got %q", phase, action)
			}
		}
	}
}

func TestRollbackActions_NoActionsOutsideDeployWindow(t *testing.T) {
	for _, phase := range []DeployPhase{PhaseInit, PhasePrepareRelease, PhasePostDeployHooks, PhaseCleanup, PhaseDone} {
		state := NewDeployState("myapp")
		state.Phase = phase
		if actions := state.RollbackActions(); len(actions) != 0 {
			t.Errorf("phase %s: expected no rollback actions, got %v", phase, actions)
		}
	}
}
