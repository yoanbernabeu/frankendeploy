package deploy

import (
	"context"
	"fmt"
	"strings"

	"github.com/yoanbernabeu/frankendeploy/internal/constants"
	"github.com/yoanbernabeu/frankendeploy/internal/ssh"
)

// caddyContainerName is the front proxy container started by server setup.
const caddyContainerName = "caddy"

// EnsureAppNetwork makes the application's dedicated Docker network ready to
// receive containers: the network exists, the Caddy front proxy is attached
// to it (so it can still reach <app>:8080 by name), and a managed database
// created before per-app networks existed is attached too. Every step checks
// the current state first, so the function is safe to call on every deploy,
// rollback and env reload, and an interrupted migration resumes on the next
// run.
func EnsureAppNetwork(ctx context.Context, client ssh.Executor, appName string, log Logger) error {
	if log == nil {
		log = NopLogger{}
	}
	network := constants.AppNetworkName(appName)

	if !networkExists(ctx, client, network) {
		if err := createNetwork(ctx, client, network, appName); err != nil {
			return err
		}
		log.Success("Created network %s", network)
	}

	// Caddy must reach the app: without the proxy on this network the app
	// runs but nobody can reach it.
	switch attached, err := attachContainer(ctx, client, caddyContainerName, network); {
	case err != nil:
		return fmt.Errorf("the Caddy front proxy could not join network %s: %w", network, err)
	case !attached:
		log.Warning("Caddy container not found: the application will not be reachable until 'frankendeploy server setup' is run")
	}

	// A managed database started on the shared network (before per-app
	// networks) is reused, never recreated: bring it onto the app network so
	// the new containers can reach it. It stays on the shared network until
	// the old app container is gone (see DetachFromSharedNetwork).
	if _, err := attachContainer(ctx, client, appName+"-db", network); err != nil {
		return fmt.Errorf("the database container could not join network %s: %w", network, err)
	}

	return nil
}

// DetachFromSharedNetwork finishes the migration to the per-app network:
// any container of the app still attached to the shared network is
// disconnected from it, but only once it is attached to the app network, so
// a container is never left without a network. Best-effort: a failure is
// reported and retried by the next deploy.
func DetachFromSharedNetwork(ctx context.Context, client ssh.Executor, appName string, log Logger) {
	if log == nil {
		log = NopLogger{}
	}
	network := constants.AppNetworkName(appName)

	for _, container := range []string{appName, appName + "-worker", appName + "-db"} {
		networks, exists := containerNetworks(ctx, client, container)
		if !exists || !contains(networks, constants.NetworkName) || !contains(networks, network) {
			continue
		}
		result, err := client.Exec(ctx, fmt.Sprintf("docker network disconnect %s %s", constants.NetworkName, container))
		if err == nil {
			err = result.Err()
		}
		if err != nil {
			log.Warning("Could not detach %s from the shared network %s (will retry on next deploy): %v", container, constants.NetworkName, err)
			continue
		}
		log.Info("Detached %s from the shared network %s", container, constants.NetworkName)
	}
}

// RemoveAppNetwork detaches Caddy from the app network and removes it. Used
// by app remove, after the app containers are gone. Best-effort.
func RemoveAppNetwork(ctx context.Context, client ssh.Executor, appName string) error {
	network := constants.AppNetworkName(appName)
	if !networkExists(ctx, client, network) {
		return nil
	}
	_, _ = client.Exec(ctx, fmt.Sprintf("docker network disconnect %s %s 2>/dev/null || true", network, caddyContainerName))
	result, err := client.Exec(ctx, fmt.Sprintf("docker network rm %s", network))
	if err != nil {
		return err
	}
	return result.Err()
}

// networkExists reports whether a Docker network exists on the server.
func networkExists(ctx context.Context, client ssh.Executor, network string) bool {
	result, err := client.Exec(ctx, fmt.Sprintf("docker network inspect %s --format '{{.Name}}' 2>/dev/null", network))
	return err == nil && result != nil && result.ExitCode == 0 && strings.TrimSpace(result.Stdout) != ""
}

// createNetwork creates the app network. The Docker default address pools
// only cover about 30 bridge networks: past that, the error is explained
// instead of surfacing a cryptic "non-overlapping IPv4 address pool".
func createNetwork(ctx context.Context, client ssh.Executor, network, appName string) error {
	result, err := client.Exec(ctx, fmt.Sprintf("docker network create --label frankendeploy.app=%s %s", appName, network))
	if err != nil {
		return fmt.Errorf("failed to create network %s: %w", network, err)
	}
	if result.ExitCode != 0 {
		if strings.Contains(result.Stderr, "non-overlapping") {
			return fmt.Errorf("failed to create network %s: Docker ran out of address pools for bridge networks (one per application).\n"+
				"Remove networks of applications no longer deployed, or extend the pools in /etc/docker/daemon.json:\n"+
				`  {"default-address-pools": [{"base": "10.200.0.0/16", "size": 24}]}`+"\n"+
				"then restart Docker", network)
		}
		return fmt.Errorf("failed to create network %s: %s", network, strings.TrimSpace(result.Stderr))
	}
	return nil
}

// attachContainer connects a container to a network unless it already is.
// Returns false without error when the container does not exist.
func attachContainer(ctx context.Context, client ssh.Executor, container, network string) (bool, error) {
	networks, exists := containerNetworks(ctx, client, container)
	if !exists {
		return false, nil
	}
	if contains(networks, network) {
		return true, nil
	}
	result, err := client.Exec(ctx, fmt.Sprintf("docker network connect %s %s", network, container))
	if err != nil {
		return false, err
	}
	if err := result.Err(); err != nil {
		return false, err
	}
	return true, nil
}

// containerNetworks lists the networks a container is attached to, from the
// container side: unlike "docker network inspect", it also covers stopped
// containers (a stopped managed database keeps its network configuration).
func containerNetworks(ctx context.Context, client ssh.Executor, container string) ([]string, bool) {
	result, err := client.Exec(ctx, fmt.Sprintf(`docker inspect %s --format '{{range $name, $_ := .NetworkSettings.Networks}}{{$name}} {{end}}' 2>/dev/null`, container))
	if err != nil || result == nil || result.ExitCode != 0 {
		return nil, false
	}
	return strings.Fields(result.Stdout), true
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}
