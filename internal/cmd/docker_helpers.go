package cmd

import (
	"context"
	"fmt"

	"github.com/yoanbernabeu/frankendeploy/internal/ssh"
)

// stopAndRemoveContainer stops and removes a container (ignores errors).
func stopAndRemoveContainer(ctx context.Context, client ssh.Executor, name string) {
	if _, err := client.Exec(ctx, fmt.Sprintf("docker stop %s 2>/dev/null || true", name)); err != nil {
		PrintVerbose("Could not stop container %s: %v", name, err)
	}
	if _, err := client.Exec(ctx, fmt.Sprintf("docker rm %s 2>/dev/null || true", name)); err != nil {
		PrintVerbose("Could not remove container %s: %v", name, err)
	}
}

// forceRemoveContainer force-removes a container (ignores errors).
func forceRemoveContainer(ctx context.Context, client ssh.Executor, name string) {
	if _, err := client.Exec(ctx, fmt.Sprintf("docker rm -f %s 2>/dev/null || true", name)); err != nil {
		PrintVerbose("Could not force remove container %s: %v", name, err)
	}
}
