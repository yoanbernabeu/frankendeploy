package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/yoanbernabeu/frankendeploy/internal/ssh"
)

func TestStopAndRemoveContainer(t *testing.T) {
	mock := &ssh.MockExecutor{}

	stopAndRemoveContainer(context.Background(), mock, "test-app")

	if len(mock.Commands) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(mock.Commands))
	}

	if !strings.Contains(mock.Commands[0], "docker stop test-app") {
		t.Errorf("first command should be docker stop, got: %s", mock.Commands[0])
	}
	if !strings.Contains(mock.Commands[1], "docker rm test-app") {
		t.Errorf("second command should be docker rm, got: %s", mock.Commands[1])
	}
}

func TestForceRemoveContainer(t *testing.T) {
	mock := &ssh.MockExecutor{}

	forceRemoveContainer(context.Background(), mock, "test-app")

	if len(mock.Commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(mock.Commands))
	}

	if !strings.Contains(mock.Commands[0], "docker rm -f test-app") {
		t.Errorf("command should be docker rm -f, got: %s", mock.Commands[0])
	}
}
