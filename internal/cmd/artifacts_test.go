package cmd

import (
	"os"
	"testing"

	"github.com/yoanbernabeu/frankendeploy/internal/config"
)

func artifactsTestConfig() *config.ProjectConfig {
	return &config.ProjectConfig{
		Name: "test-app",
		PHP: config.PHPConfig{
			Version:    "8.3",
			Extensions: []string{"intl", "opcache"},
		},
	}
}

func TestEnsureDockerArtifacts_GeneratesAllWhenMissing(t *testing.T) {
	t.Chdir(t.TempDir())

	if err := ensureDockerArtifacts(artifactsTestConfig()); err != nil {
		t.Fatalf("ensureDockerArtifacts failed: %v", err)
	}

	for _, f := range []string{"Dockerfile", "docker-entrypoint.sh", ".dockerignore"} {
		info, err := os.Stat(f)
		if err != nil {
			t.Fatalf("%s should have been generated: %v", f, err)
		}
		if info.Size() == 0 {
			t.Errorf("%s should not be empty", f)
		}
	}

	info, err := os.Stat("docker-entrypoint.sh")
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0111 == 0 {
		t.Error("docker-entrypoint.sh should be executable")
	}
}

func TestEnsureDockerArtifacts_NeverOverwritesExisting(t *testing.T) {
	t.Chdir(t.TempDir())

	custom := "# customized by user — must not be overwritten\n"
	for _, f := range []string{"Dockerfile", "docker-entrypoint.sh", ".dockerignore"} {
		if err := os.WriteFile(f, []byte(custom), 0644); err != nil {
			t.Fatal(err)
		}
	}

	if err := ensureDockerArtifacts(artifactsTestConfig()); err != nil {
		t.Fatalf("ensureDockerArtifacts failed: %v", err)
	}

	for _, f := range []string{"Dockerfile", "docker-entrypoint.sh", ".dockerignore"} {
		content, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if string(content) != custom {
			t.Errorf("%s was overwritten, user content lost", f)
		}
	}
}

func TestEnsureDockerArtifacts_GeneratesOnlyMissing(t *testing.T) {
	t.Chdir(t.TempDir())

	custom := "# my hand-tuned Dockerfile\n"
	if err := os.WriteFile("Dockerfile", []byte(custom), 0644); err != nil {
		t.Fatal(err)
	}

	if err := ensureDockerArtifacts(artifactsTestConfig()); err != nil {
		t.Fatalf("ensureDockerArtifacts failed: %v", err)
	}

	content, err := os.ReadFile("Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != custom {
		t.Error("existing Dockerfile was overwritten")
	}

	for _, f := range []string{"docker-entrypoint.sh", ".dockerignore"} {
		info, err := os.Stat(f)
		if err != nil {
			t.Fatalf("%s should have been generated: %v", f, err)
		}
		if info.Size() == 0 {
			t.Errorf("%s should not be empty", f)
		}
	}
}
