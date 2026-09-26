package cmd

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/yoanbernabeu/frankendeploy/internal/constants"
	"github.com/yoanbernabeu/frankendeploy/internal/ssh"
)

// Docker bridges default to an MTU of 1500. On a VPS whose network interface
// has a smaller one (1460 on Google Cloud, 1450 on VXLAN clouds, 1400 on some
// Kubernetes-backed VMs), large packets from containers are silently dropped:
// DNS and TCP handshakes work, downloads hang. A build then stalls on
// apt-get and ends with "Unable to locate package", and Caddy cannot reach
// Let's Encrypt.

const (
	defaultMTU       = 1500
	dockerDaemonJSON = "/etc/docker/daemon.json"
	mtuNetworkOption = "com.docker.network.driver.mtu"
)

// hostMTUCommand prints the MTU of the interface carrying the default route.
const hostMTUCommand = `dev=$(ip route get 1.1.1.1 2>/dev/null | sed -n 's/.* dev \([^ ]*\).*/\1/p'); [ -n "$dev" ] && cat "/sys/class/net/$dev/mtu"`

// detectHostMTU returns the MTU of the server's default route interface, or
// 0 when it cannot be determined (the caller then leaves Docker alone).
func detectHostMTU(ctx context.Context, client ssh.Executor) int {
	return execInt(ctx, client, hostMTUCommand)
}

// dockerBridgeMTU returns the MTU of docker0, the network builds run on.
func dockerBridgeMTU(ctx context.Context, client ssh.Executor) int {
	return execInt(ctx, client, "cat /sys/class/net/docker0/mtu")
}

// networkMTU returns the MTU of a Docker network, defaultMTU when the network
// has no explicit option, or 0 when the network does not exist.
func networkMTU(ctx context.Context, client ssh.Executor, network string) int {
	result, err := client.Exec(ctx, fmt.Sprintf(`docker network inspect %s --format '{{index .Options "%s"}}' 2>/dev/null`, network, mtuNetworkOption))
	if err != nil || result.ExitCode != 0 {
		return 0
	}
	value := strings.TrimSpace(result.Stdout)
	if value == "" || value == "<no value>" {
		return defaultMTU
	}
	mtu, err := strconv.Atoi(value)
	if err != nil {
		return defaultMTU
	}
	return mtu
}

func execInt(ctx context.Context, client ssh.Executor, command string) int {
	result, err := client.Exec(ctx, command)
	if err != nil || result.ExitCode != 0 {
		return 0
	}
	value, err := strconv.Atoi(strings.TrimSpace(result.Stdout))
	if err != nil {
		return 0
	}
	return value
}

// dockerDaemonConfig is the daemon.json written for a host MTU below 1500:
// docker0 (builds) and every network created afterwards use it.
func dockerDaemonConfig(mtu int) string {
	return fmt.Sprintf(`{"mtu": %d, "default-network-opts": {"bridge": {"%s": "%d"}}}`, mtu, mtuNetworkOption, mtu)
}

// configureDockerMTU aligns Docker with a host MTU below 1500 and returns the
// MTU networks must be created with (0 when nothing is needed). An existing
// daemon.json written by someone else is never modified: the user gets the
// lines to add instead.
func configureDockerMTU(ctx context.Context, client ssh.Executor) (int, error) {
	mtu := detectHostMTU(ctx, client)
	if mtu == 0 || mtu >= defaultMTU {
		return 0, nil
	}

	want := dockerDaemonConfig(mtu)
	result, err := client.Exec(ctx, "cat "+dockerDaemonJSON+" 2>/dev/null")
	if err != nil {
		return 0, fmt.Errorf("failed to read %s: %w", dockerDaemonJSON, err)
	}
	current := strings.TrimSpace(result.Stdout)

	switch {
	case current == want:
		return mtu, nil
	case result.ExitCode == 0 && current != "":
		PrintWarning("The server network MTU is %d but %s already exists: it was left untouched.", mtu, dockerDaemonJSON)
		fmt.Printf("   Containers may fail to download anything (builds hang on apt-get). Add to %s:\n", dockerDaemonJSON)
		fmt.Printf("     \"mtu\": %d,\n     \"default-network-opts\": {\"bridge\": {\"%s\": \"%d\"}}\n", mtu, mtuNetworkOption, mtu)
		fmt.Println("   then run: sudo systemctl restart docker")
		return mtu, nil
	}

	PrintInfo("Server network MTU is %d: configuring Docker to match", mtu)
	writeCmd := fmt.Sprintf("sudo mkdir -p /etc/docker && echo '%s' | sudo tee %s > /dev/null", want, dockerDaemonJSON)
	if result, err := client.Exec(ctx, writeCmd); err != nil {
		return 0, fmt.Errorf("failed to write %s: %w", dockerDaemonJSON, err)
	} else if err := result.Err(); err != nil {
		return 0, fmt.Errorf("failed to write %s: %w", dockerDaemonJSON, err)
	}
	if result, err := client.Exec(ctx, "sudo systemctl restart docker"); err != nil {
		return 0, fmt.Errorf("failed to restart Docker: %w", err)
	} else if err := result.Err(); err != nil {
		return 0, fmt.Errorf("failed to restart Docker: %w", err)
	}
	return mtu, nil
}

// ensureSharedNetwork creates the shared network, with the given MTU when it
// is non-zero. A network left with a larger MTU by an earlier setup is
// recreated; Docker refuses to remove a network still in use, so an app
// container attached to it keeps it (with a warning) rather than breaking.
// Caddy is removed first: setup starts it again right after.
func ensureSharedNetwork(ctx context.Context, client ssh.Executor, mtu int) error {
	network := constants.NetworkName
	current := networkMTU(ctx, client, network)

	if current != 0 && mtu != 0 && current > mtu {
		forceRemoveContainer(ctx, client, "caddy")
		result, err := client.Exec(ctx, "docker network rm "+network)
		if err != nil || result.ExitCode != 0 {
			PrintWarning("Network %q uses MTU %d but is still used by other containers: recreate it by hand to fix outgoing traffic.", network, current)
			return nil
		}
		current = 0
	}
	if current != 0 {
		return nil
	}

	createCmd := "docker network create " + network
	if mtu != 0 {
		createCmd = fmt.Sprintf("docker network create -o %s=%d %s", mtuNetworkOption, mtu, network)
	}
	result, err := client.Exec(ctx, createCmd)
	if err != nil {
		return fmt.Errorf("failed to create network %s: %w", network, err)
	}
	if err := result.Err(); err != nil {
		return fmt.Errorf("failed to create network %s: %w", network, err)
	}
	return nil
}

// checkNetworkMTU reports Docker networks whose MTU exceeds the host's.
func checkNetworkMTU(ctx context.Context, client ssh.Executor) doctorResult {
	name := "Network MTU"
	hostMTU := detectHostMTU(ctx, client)
	if hostMTU == 0 {
		return doctorResult{Name: name, OK: true, Warning: true, Detail: "could not determine the server MTU"}
	}
	if hostMTU >= defaultMTU {
		return doctorResult{Name: name, OK: true, Detail: strconv.Itoa(hostMTU)}
	}

	var mismatched []string
	if bridge := dockerBridgeMTU(ctx, client); bridge > hostMTU {
		mismatched = append(mismatched, fmt.Sprintf("docker0 (builds) uses %d", bridge))
	}
	if shared := networkMTU(ctx, client, constants.NetworkName); shared > hostMTU {
		mismatched = append(mismatched, fmt.Sprintf("%s uses %d", constants.NetworkName, shared))
	}
	if len(mismatched) == 0 {
		return doctorResult{Name: name, OK: true, Detail: fmt.Sprintf("%d, Docker aligned", hostMTU)}
	}

	return doctorResult{
		Name:   name,
		OK:     false,
		Detail: fmt.Sprintf("server interface is %d but %s: containers cannot download (builds hang on apt-get, no HTTPS certificate)", hostMTU, strings.Join(mismatched, ", ")),
		Advice: "Run 'frankendeploy server setup <name> --email you@example.com' again: it configures Docker for this MTU.",
	}
}
