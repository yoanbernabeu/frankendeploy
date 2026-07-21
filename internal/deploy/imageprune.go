package deploy

import (
	"context"
	"fmt"
	"strings"

	"github.com/yoanbernabeu/frankendeploy/internal/ssh"
)

// PruneOldImages removes the app's Docker images whose tag no longer matches
// a kept release directory, keeping the rollback window and the disk usage
// aligned with keep_releases. Each deploy leaves a full 500MB–1GB image
// behind: without pruning, a small VPS fills up after a few dozen deploys.
//
// Safety rules:
//   - nothing is removed when the kept-releases list cannot be read or is
//     empty (an unknown kept set would make every image "old")
//   - docker rmi is never forced: an image still used by a container (the
//     running app, a worker on an older tag) survives and is retried on the
//     next deploy
//
// Returns the list of removed tags.
func PruneOldImages(ctx context.Context, client ssh.Executor, appName, appPath string) ([]string, error) {
	keptResult, err := client.Exec(ctx, fmt.Sprintf("ls -1 %s/releases 2>/dev/null", appPath))
	if err != nil || keptResult == nil {
		return nil, fmt.Errorf("cannot list kept releases: %w", err)
	}
	kept := make(map[string]bool)
	for _, line := range strings.Split(keptResult.Stdout, "\n") {
		if tag := strings.TrimSpace(line); tag != "" {
			kept[tag] = true
		}
	}
	if len(kept) == 0 {
		return nil, nil
	}

	imagesResult, err := client.Exec(ctx, fmt.Sprintf("docker images %s --format '{{.Tag}}'", appName))
	if err != nil || imagesResult == nil {
		return nil, fmt.Errorf("cannot list images: %w", err)
	}

	var removed []string
	for _, line := range strings.Split(imagesResult.Stdout, "\n") {
		tag := strings.TrimSpace(line)
		if tag == "" || tag == "<none>" || tag == "latest" || kept[tag] {
			continue
		}
		result, err := client.Exec(ctx, fmt.Sprintf("docker rmi %s:%s", appName, tag))
		if err != nil || result == nil || result.ExitCode != 0 {
			// In use or transient error: skip, the next deploy retries
			continue
		}
		removed = append(removed, tag)
	}

	return removed, nil
}
