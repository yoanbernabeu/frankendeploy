package deploy

// Tests for issue #55: prune old Docker images according to keep_releases.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/yoanbernabeu/frankendeploy/internal/ssh"
)

// pruneMock builds a MockExecutor where `ls releases` returns keptTags and
// `docker images` returns imageTags.
func pruneMock(keptTags, imageTags []string) *ssh.MockExecutor {
	return &ssh.MockExecutor{
		ExecFunc: func(ctx context.Context, command string) (*ssh.ExecResult, error) {
			switch {
			case strings.Contains(command, "releases"):
				return &ssh.ExecResult{Stdout: strings.Join(keptTags, "\n") + "\n", ExitCode: 0}, nil
			case strings.Contains(command, "docker images"):
				return &ssh.ExecResult{Stdout: strings.Join(imageTags, "\n") + "\n", ExitCode: 0}, nil
			default:
				return &ssh.ExecResult{ExitCode: 0}, nil
			}
		},
	}
}

func rmiCommands(mock *ssh.MockExecutor) []string {
	var rmis []string
	for _, cmd := range mock.Commands {
		if strings.Contains(cmd, "docker rmi") {
			rmis = append(rmis, cmd)
		}
	}
	return rmis
}

func TestPruneOldImages_RemovesOnlyUnkeptTags(t *testing.T) {
	mock := pruneMock(
		[]string{"tag3", "tag4", "tag5"},
		[]string{"tag1", "tag2", "tag3", "tag4", "tag5"},
	)

	removed, err := PruneOldImages(context.Background(), mock, "myapp", "/opt/frankendeploy/apps/myapp")
	if err != nil {
		t.Fatalf("PruneOldImages() error = %v", err)
	}

	if len(removed) != 2 {
		t.Fatalf("removed = %v, want [tag1 tag2]", removed)
	}
	rmis := rmiCommands(mock)
	if len(rmis) != 2 {
		t.Fatalf("expected 2 docker rmi commands, got %d: %v", len(rmis), rmis)
	}
	for _, rmi := range rmis {
		if !strings.Contains(rmi, "myapp:tag1") && !strings.Contains(rmi, "myapp:tag2") {
			t.Errorf("unexpected rmi target: %s", rmi)
		}
		if strings.Contains(rmi, "tag3") || strings.Contains(rmi, "tag4") || strings.Contains(rmi, "tag5") {
			t.Errorf("kept tag must never be removed: %s", rmi)
		}
		if strings.Contains(rmi, "-f") {
			t.Errorf("rmi must not force: an image still used by a container must survive: %s", rmi)
		}
	}
}

func TestPruneOldImages_SkipsSpecialTags(t *testing.T) {
	mock := pruneMock(
		[]string{"tag2"},
		[]string{"<none>", "latest", "tag1", "tag2"},
	)

	removed, err := PruneOldImages(context.Background(), mock, "myapp", "/opt/frankendeploy/apps/myapp")
	if err != nil {
		t.Fatalf("PruneOldImages() error = %v", err)
	}
	if len(removed) != 1 || removed[0] != "tag1" {
		t.Errorf("removed = %v, want [tag1]", removed)
	}
}

func TestPruneOldImages_NothingToRemove(t *testing.T) {
	mock := pruneMock([]string{"tag1", "tag2"}, []string{"tag1", "tag2"})

	removed, err := PruneOldImages(context.Background(), mock, "myapp", "/opt/frankendeploy/apps/myapp")
	if err != nil {
		t.Fatalf("PruneOldImages() error = %v", err)
	}
	if len(removed) != 0 {
		t.Errorf("removed = %v, want none", removed)
	}
	if len(rmiCommands(mock)) != 0 {
		t.Error("no rmi command expected")
	}
}

func TestPruneOldImages_NoKeptListNoPrune(t *testing.T) {
	// If the kept-releases list cannot be read, removing anything based on an
	// empty set would wipe every image — it must be an error instead.
	mock := &ssh.MockExecutor{
		ExecFunc: func(ctx context.Context, command string) (*ssh.ExecResult, error) {
			if strings.Contains(command, "releases") {
				return nil, errors.New("connection lost")
			}
			return &ssh.ExecResult{Stdout: "tag1\ntag2\n", ExitCode: 0}, nil
		},
	}

	_, err := PruneOldImages(context.Background(), mock, "myapp", "/opt/frankendeploy/apps/myapp")
	if err == nil {
		t.Fatal("expected error when the kept-releases list is unreadable")
	}
	if len(rmiCommands(mock)) != 0 {
		t.Error("must not remove any image when the kept list is unknown")
	}
}

func TestPruneOldImages_EmptyKeptListNoPrune(t *testing.T) {
	// An empty releases dir means we cannot tell what is safe: do not prune.
	mock := pruneMock([]string{}, []string{"tag1", "tag2"})

	removed, err := PruneOldImages(context.Background(), mock, "myapp", "/opt/frankendeploy/apps/myapp")
	if err != nil {
		t.Fatalf("PruneOldImages() error = %v", err)
	}
	if len(removed) != 0 || len(rmiCommands(mock)) != 0 {
		t.Errorf("must not prune with an empty kept list, removed = %v", removed)
	}
}

func TestPruneOldImages_RmiFailureIsNotFatal(t *testing.T) {
	// docker rmi fails when a container still uses the image (e.g. a worker
	// on an old tag): skip it, keep pruning the rest.
	mock := &ssh.MockExecutor{
		ExecFunc: func(ctx context.Context, command string) (*ssh.ExecResult, error) {
			switch {
			case strings.Contains(command, "releases"):
				return &ssh.ExecResult{Stdout: "tag3\n", ExitCode: 0}, nil
			case strings.Contains(command, "docker images"):
				return &ssh.ExecResult{Stdout: "tag1\ntag2\ntag3\n", ExitCode: 0}, nil
			case strings.Contains(command, "docker rmi myapp:tag1"):
				return &ssh.ExecResult{ExitCode: 1, Stderr: "image is being used"}, nil
			default:
				return &ssh.ExecResult{ExitCode: 0}, nil
			}
		},
	}

	removed, err := PruneOldImages(context.Background(), mock, "myapp", "/opt/frankendeploy/apps/myapp")
	if err != nil {
		t.Fatalf("PruneOldImages() error = %v", err)
	}
	if len(removed) != 1 || removed[0] != "tag2" {
		t.Errorf("removed = %v, want [tag2] (tag1 in use, tag3 kept)", removed)
	}
}
