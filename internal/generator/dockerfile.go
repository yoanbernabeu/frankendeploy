package generator

import (
	"encoding/json"
	"fmt"
	"os"

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
	Dockerfile        config.DockerfileConfig
	FrankenPHPVersion string
	// HealthcheckPath is the app endpoint probed by the container
	// HEALTHCHECK (default "/")
	HealthcheckPath string
	// HasPreload enables opcache.preload when the project ships a
	// config/preload.php
	HasPreload bool
	// Worker enables FrankenPHP worker mode: the generated Caddyfile is
	// copied into the prod stage (dev keeps the image default, classic mode)
	Worker bool
}

// Generate generates the Dockerfile content
func (g *DockerfileGenerator) Generate() (string, error) {
	data := DockerfileData{
		Name:              g.config.Name,
		PHP:               g.config.PHP,
		Dockerfile:        g.config.Dockerfile,
		FrankenPHPVersion: g.config.FrankenPHPVersion,
		HealthcheckPath:   g.config.Deploy.HealthcheckPath,
		HasPreload:        hasPreloadFile(),
		Worker:            g.config.FrankenPHP.Worker,
	}

	if g.config.Assets.BuildTool != "" {
		assets := g.config.Assets
		// Sensible default so a hand-written frankendeploy.yaml with a
		// BuildTool but no output_dir still produces a valid Dockerfile.
		// The scanner populates OutputDir, so this only kicks in for manual
		// configs.
		if assets.OutputDir == "" {
			assets.OutputDir = "public/build"
		}
		data.Assets = &assets
	}

	if err := ValidateDockerfileData(data); err != nil {
		return "", fmt.Errorf("validation failed: %w", err)
	}

	return g.loader.Execute("dockerfile.tmpl", data)
}

// hasPreloadFile reports whether the project (current directory, where
// generation runs) ships a config/preload.php for OPcache preloading.
func hasPreloadFile() bool {
	info, err := os.Stat("config/preload.php")
	return err == nil && !info.IsDir()
}

// hasLegacyFrankenPHPRuntime reports whether composer.json (in the current
// directory, like hasPreloadFile) requires runtime/frankenphp-symfony, the
// only runtime that needs the APP_RUNTIME override in the worker Caddyfile.
func hasLegacyFrankenPHPRuntime() bool {
	data, err := os.ReadFile("composer.json")
	if err != nil {
		return false
	}
	var composer struct {
		Require map[string]string `json:"require"`
	}
	if err := json.Unmarshal(data, &composer); err != nil {
		return false
	}
	_, ok := composer.Require["runtime/frankenphp-symfony"]
	return ok
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
func (g *DockerfileGenerator) GenerateDockerignore() (string, error) {
	return g.loader.Execute("dockerignore.tmpl", nil)
}

// WriteDockerignore writes the .dockerignore file
func (g *DockerfileGenerator) WriteDockerignore(path string) error {
	if path == "" {
		path = ".dockerignore"
	}

	content, err := g.GenerateDockerignore()
	if err != nil {
		return fmt.Errorf("failed to generate .dockerignore: %w", err)
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write .dockerignore: %w", err)
	}

	return nil
}

// CaddyfileData holds data for the app Caddyfile template (worker mode).
type CaddyfileData struct {
	Name string
	// LegacyRuntime is true when the app relies on runtime/frankenphp-symfony,
	// whose Runtime class must be forced through APP_RUNTIME. With
	// symfony/runtime >= 7.4 the worker runner is picked automatically and the
	// override would point to a class that does not exist.
	LegacyRuntime bool
}

// GenerateCaddyfile generates the app-level Caddyfile enabling FrankenPHP
// worker mode. Only relevant when config frankenphp.worker is true.
func (g *DockerfileGenerator) GenerateCaddyfile() (string, error) {
	return g.loader.Execute("caddyfile.tmpl", CaddyfileData{
		Name:          g.config.Name,
		LegacyRuntime: hasLegacyFrankenPHPRuntime(),
	})
}

// WriteCaddyfile writes the app Caddyfile to the specified path.
func (g *DockerfileGenerator) WriteCaddyfile(path string) error {
	if path == "" {
		path = "Caddyfile"
	}

	content, err := g.GenerateCaddyfile()
	if err != nil {
		return fmt.Errorf("failed to generate Caddyfile: %w", err)
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write Caddyfile: %w", err)
	}

	return nil
}

// EntrypointData holds data for the docker-entrypoint.sh template.
type EntrypointData struct {
	MaxDBWaitAttempts int
	DBWaitInterval    int
}

// DefaultEntrypointData returns entrypoint data with default values.
func DefaultEntrypointData() EntrypointData {
	return EntrypointData{
		MaxDBWaitAttempts: DefaultDBWaitMaxAttempts,
		DBWaitInterval:    DefaultDBWaitInterval,
	}
}

// GenerateEntrypoint generates the docker-entrypoint.sh content
func (g *DockerfileGenerator) GenerateEntrypoint() (string, error) {
	return g.loader.Execute("docker-entrypoint.tmpl", DefaultEntrypointData())
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
