package cmd

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/yoanbernabeu/frankendeploy/internal/ssh"
)

// scriptedMock returns a MockExecutor answering per command substring.
func scriptedMock(responses map[string]ssh.ExecResult) *ssh.MockExecutor {
	return &ssh.MockExecutor{
		ExecFunc: func(_ context.Context, command string) (*ssh.ExecResult, error) {
			for pattern, result := range responses {
				if strings.Contains(command, pattern) {
					r := result
					return &r, nil
				}
			}
			return &ssh.ExecResult{ExitCode: 0}, nil
		},
	}
}

func TestDoctorRemoteDocker_MissingSuggestsServerSetup(t *testing.T) {
	mock := scriptedMock(map[string]ssh.ExecResult{
		"docker version": {ExitCode: 127, Stderr: "docker: command not found"},
	})

	res := checkRemoteDocker(context.Background(), mock)
	if res.OK {
		t.Fatal("expected failure when docker is missing")
	}
	if !strings.Contains(res.Advice, "server setup") {
		t.Errorf("the fix must point at 'frankendeploy server setup', got %q", res.Advice)
	}
}

func TestDoctorRemoteDocker_PermissionDenied(t *testing.T) {
	mock := scriptedMock(map[string]ssh.ExecResult{
		"docker version": {ExitCode: 1, Stderr: "permission denied while trying to connect to the Docker daemon socket"},
	})

	res := checkRemoteDocker(context.Background(), mock)
	if res.OK {
		t.Fatal("expected failure on permission denied")
	}
	if !strings.Contains(res.Advice, "docker") || !strings.Contains(res.Advice, "group") {
		t.Errorf("the fix must mention the docker group, got %q", res.Advice)
	}
}

func TestDoctorNetwork(t *testing.T) {
	ok := scriptedMock(map[string]ssh.ExecResult{
		"docker network ls": {Stdout: "frankendeploy\n", ExitCode: 0},
	})
	if res := checkDockerNetwork(context.Background(), ok); !res.OK {
		t.Errorf("expected OK when the network exists, got %+v", res)
	}

	missing := scriptedMock(map[string]ssh.ExecResult{
		"docker network ls": {Stdout: "", ExitCode: 0},
	})
	res := checkDockerNetwork(context.Background(), missing)
	if res.OK {
		t.Fatal("expected failure when the network is missing")
	}
	if !strings.Contains(res.Advice, "server setup") && !strings.Contains(res.Advice, "network create") {
		t.Errorf("the fix must explain how to create the network, got %q", res.Advice)
	}
}

func TestDoctorCaddy(t *testing.T) {
	up := scriptedMock(map[string]ssh.ExecResult{
		"docker ps": {Stdout: "Up 3 hours\n", ExitCode: 0},
	})
	if res := checkCaddyRunning(context.Background(), up); !res.OK {
		t.Errorf("expected OK when Caddy is up, got %+v", res)
	}

	down := scriptedMock(map[string]ssh.ExecResult{
		"docker ps": {Stdout: "", ExitCode: 0},
	})
	res := checkCaddyRunning(context.Background(), down)
	if res.OK {
		t.Fatal("expected failure when Caddy is not running")
	}
	if !strings.Contains(res.Advice, "server setup") {
		t.Errorf("the fix must point at server setup, got %q", res.Advice)
	}
}

func TestDoctorDiskSpace(t *testing.T) {
	// df -Pk output: available in KB (column 4)
	plenty := scriptedMock(map[string]ssh.ExecResult{
		"df -Pk": {Stdout: "Filesystem 1024-blocks Used Available Capacity Mounted\n/dev/sda1 41152736 8123456 31029280 21% /\n", ExitCode: 0},
	})
	if res := checkDiskSpace(context.Background(), plenty); !res.OK {
		t.Errorf("expected OK with 31GB free, got %+v", res)
	}

	tight := scriptedMock(map[string]ssh.ExecResult{
		"df -Pk": {Stdout: "Filesystem 1024-blocks Used Available Capacity Mounted\n/dev/sda1 41152736 40100000 1052736 98% /\n", ExitCode: 0},
	})
	res := checkDiskSpace(context.Background(), tight)
	if res.OK {
		t.Fatal("expected failure with ~1GB free (a Docker build needs more)")
	}
}

func TestDoctorSudo_RootPasses(t *testing.T) {
	root := scriptedMock(map[string]ssh.ExecResult{
		"id -u": {Stdout: "0\n", ExitCode: 0},
	})
	if res := checkRemoteSudo(context.Background(), root); !res.OK {
		t.Errorf("root must pass the sudo check, got %+v", res)
	}
}

func TestDoctorSudo_NoPasswordlessSudo(t *testing.T) {
	mock := scriptedMock(map[string]ssh.ExecResult{
		"id -u":        {Stdout: "1000\n", ExitCode: 0},
		"sudo -n true": {ExitCode: 1, Stderr: "sudo: a password is required"},
	})
	res := checkRemoteSudo(context.Background(), mock)
	if res.OK {
		t.Fatal("expected failure without passwordless sudo")
	}
	if !strings.Contains(res.Advice, "NOPASSWD") {
		t.Errorf("the fix must show the NOPASSWD line, got %q", res.Advice)
	}
}

func TestDoctorDNS(t *testing.T) {
	serverIP := "91.108.184.120"

	match := checkDomainDNS("demo.example.com", serverIP, func(host string) ([]string, error) {
		return []string{"91.108.184.120"}, nil
	})
	if !match.OK {
		t.Errorf("expected OK when DNS matches the server IP, got %+v", match)
	}

	mismatch := checkDomainDNS("demo.example.com", serverIP, func(host string) ([]string, error) {
		return []string{"10.0.0.7"}, nil
	})
	if mismatch.OK {
		t.Fatal("expected failure when DNS points elsewhere (the #1 cause of Let's Encrypt failures)")
	}
	if !strings.Contains(mismatch.Advice, "91.108.184.120") {
		t.Errorf("the fix must show the expected IP, got %q", mismatch.Advice)
	}

	nxdomain := checkDomainDNS("demo.example.com", serverIP, func(host string) ([]string, error) {
		return nil, fmt.Errorf("no such host")
	})
	if nxdomain.OK {
		t.Fatal("expected failure when the domain does not resolve")
	}
}
