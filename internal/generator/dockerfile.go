package generator

import (
	"fmt"
	"os"
	"strings"

	"github.com/yoanbernabeu/frankendeploy/internal/config"
)

// DockerfileGenerator generates Dockerfiles for Symfony applications
type DockerfileGenerator struct {
	loader *TemplateLoader
	config *config.ProjectConfig
}

// NewDockerfileGenerator creates a new Dockerfile generator
func NewDockerfileGenerator(cfg *config.ProjectConfig) *DockerfileGenerator {
	return &DockerfileGenerator{
		loader: NewTemplateLoader(),
		config: cfg,
	}
}

// DockerfileData holds data for Dockerfile template
type DockerfileData struct {
	Name              string
	PHP               config.PHPConfig
	Assets            *config.AssetsConfig
	Deploy            config.DeployConfig
	Dockerfile        config.DockerfileConfig
	FrankenPHPVersion string
}

// Generate generates the Dockerfile content
func (g *DockerfileGenerator) Generate() (string, error) {
	data := DockerfileData{
		Name:              g.config.Name,
		PHP:               g.config.PHP,
		Deploy:            g.config.Deploy,
		Dockerfile:        g.config.Dockerfile,
		FrankenPHPVersion: g.config.FrankenPHPVersion,
	}

	if g.config.Assets.BuildTool != "" {
		data.Assets = &g.config.Assets
	}

	return g.loader.Execute("dockerfile.tmpl", data)
}

// WriteDockerfile writes the Dockerfile to the specified path
func (g *DockerfileGenerator) WriteDockerfile(path string) error {
	if path == "" {
		path = "Dockerfile"
	}

	content, err := g.Generate()
	if err != nil {
		return fmt.Errorf("failed to generate Dockerfile: %w", err)
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write Dockerfile: %w", err)
	}

	return nil
}

// GenerateDockerignore generates a .dockerignore file
func (g *DockerfileGenerator) GenerateDockerignore() string {
	ignorePatterns := []string{
		"# Git",
		".git",
		".gitignore",
		".gitattributes",
		"",
		"# IDE",
		".idea",
		".vscode",
		"*.swp",
		"*.swo",
		"",
		"# Docker",
		"Dockerfile*",
		"compose*.yaml",
		"compose*.yml",
		".docker",
		"",
		"# Dependencies (will be installed fresh)",
		"vendor",
		"node_modules",
		"",
		"# Build artifacts",
		"public/build",
		"var/cache",
		"var/log",
		"",
		"# Test files",
		"tests",
		"phpunit.xml*",
		".phpunit*",
		"",
		"# Dev files",
		".env.local",
		".env.*.local",
		"*.md",
		"LICENSE",
		"",
		"# FrankenDeploy",
		"frankendeploy.yaml",
	}

	return strings.Join(ignorePatterns, "\n")
}

// WriteDockerignore writes the .dockerignore file
func (g *DockerfileGenerator) WriteDockerignore(path string) error {
	if path == "" {
		path = ".dockerignore"
	}

	content := g.GenerateDockerignore()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write .dockerignore: %w", err)
	}

	return nil
}

// GenerateEntrypoint generates the docker-entrypoint.sh content
func (g *DockerfileGenerator) GenerateEntrypoint() (string, error) {
	return g.loader.Execute("docker-entrypoint.tmpl", nil)
}

// WriteEntrypoint writes the docker-entrypoint.sh file
func (g *DockerfileGenerator) WriteEntrypoint(path string) error {
	if path == "" {
		path = "docker-entrypoint.sh"
	}

	content, err := g.GenerateEntrypoint()
	if err != nil {
		return fmt.Errorf("failed to generate docker-entrypoint.sh: %w", err)
	}

	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		return fmt.Errorf("failed to write docker-entrypoint.sh: %w", err)
	}

	return nil
}
