package cmd

import (
	"fmt"
	"os"

	"github.com/yoanbernabeu/frankendeploy/internal/config"
	"github.com/yoanbernabeu/frankendeploy/internal/generator"
)

// ensureDockerArtifacts generates the Docker build artifacts (Dockerfile,
// docker-entrypoint.sh, .dockerignore) if they are missing, so that
// `deploy` works right after `init` without requiring a manual `build`.
// Existing files are never overwritten: users may have customized them.
func ensureDockerArtifacts(cfg *config.ProjectConfig) error {
	type artifact struct {
		path  string
		write func(*generator.DockerfileGenerator) error
	}

	artifacts := []artifact{
		{"Dockerfile", func(g *generator.DockerfileGenerator) error { return g.WriteDockerfile("") }},
		{"docker-entrypoint.sh", func(g *generator.DockerfileGenerator) error { return g.WriteEntrypoint("") }},
		{".dockerignore", func(g *generator.DockerfileGenerator) error { return g.WriteDockerignore("") }},
	}

	var missing []artifact
	for _, a := range artifacts {
		if _, err := os.Stat(a.path); os.IsNotExist(err) {
			missing = append(missing, a)
		}
	}
	if len(missing) == 0 {
		return nil
	}

	PrintInfo("Docker artifacts missing — generating them (equivalent to 'frankendeploy build')")
	gen := generator.NewDockerfileGenerator(cfg)
	for _, a := range missing {
		if err := a.write(gen); err != nil {
			return fmt.Errorf("failed to generate %s: %w", a.path, err)
		}
		PrintSuccess("Generated %s", a.path)
	}

	return nil
}
