package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/yoanbernabeu/frankendeploy/internal/config"
	"github.com/yoanbernabeu/frankendeploy/internal/generator"
	"github.com/yoanbernabeu/frankendeploy/internal/scanner"
)

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Generate Dockerfile and compose files",
	Long: `Generates Docker configuration files based on frankendeploy.yaml:
- Dockerfile (multi-stage build with FrankenPHP)
- docker-entrypoint.sh (handles composer install, migrations)
- compose.yaml (development environment)
- compose.prod.yaml (production environment)
- .dockerignore`,
	RunE: runBuild,
}

var (
	buildDockerfile bool
	buildCompose    bool
	buildAll        bool
)

func init() {
	rootCmd.AddCommand(buildCmd)
	buildCmd.Flags().BoolVar(&buildDockerfile, "dockerfile", false, "Generate only Dockerfile")
	buildCmd.Flags().BoolVar(&buildCompose, "compose", false, "Generate only docker-compose files")
	buildCmd.Flags().BoolVar(&buildAll, "all", false, "Generate all files")
}

func runBuild(cmd *cobra.Command, args []string) error {
	// Load configuration
	cfg, err := config.LoadProjectConfig(GetConfigFile())
	if err != nil {
		return err
	}

	// Validate configuration
	if errors := config.ValidateProjectConfig(cfg); errors.HasErrors() {
		return fmt.Errorf("configuration validation failed: %w", errors)
	}

	// Worker mode requires the FrankenPHP Symfony runtime: without it the
	// container would crash-loop at boot (APP_RUNTIME points to a missing class)
	if cfg.FrankenPHP.Worker {
		if err := checkWorkerRuntime(); err != nil {
			return err
		}
	}

	generateAll := buildAll || (!buildDockerfile && !buildCompose)

	// Generate Dockerfile and entrypoint
	if generateAll || buildDockerfile {
		dockerGen := generator.NewDockerfileGenerator(cfg)

		if err := dockerGen.WriteDockerfile(""); err != nil {
			return err
		}
		PrintSuccess("Generated Dockerfile")

		if err := dockerGen.WriteEntrypoint(""); err != nil {
			return err
		}
		PrintSuccess("Generated docker-entrypoint.sh")

		if err := dockerGen.WriteDockerignore(""); err != nil {
			return err
		}
		PrintSuccess("Generated .dockerignore")

		if cfg.FrankenPHP.Worker {
			if err := dockerGen.WriteCaddyfile(""); err != nil {
				return err
			}
			PrintSuccess("Generated Caddyfile (FrankenPHP worker mode)")
		}
	}

	// Generate compose files
	if generateAll || buildCompose {
		composeGen := generator.NewComposeGenerator(cfg)

		if err := composeGen.WriteDevCompose(""); err != nil {
			return err
		}
		PrintSuccess("Generated compose.yaml")

		if err := composeGen.WriteProdCompose(""); err != nil {
			return err
		}
		PrintSuccess("Generated compose.prod.yaml")
	}

	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  Run 'frankendeploy dev up' to start the development environment")

	return nil
}

// checkWorkerRuntime verifies that runtime/frankenphp-symfony is present in
// composer.json before generating worker-mode artifacts.
func checkWorkerRuntime() error {
	comp, err := scanner.New(".").ParseComposer()
	if err != nil {
		return fmt.Errorf("frankenphp.worker is enabled but composer.json cannot be read: %w", err)
	}
	if !comp.HasPackage("runtime/frankenphp-symfony") {
		return fmt.Errorf("frankenphp.worker is enabled but runtime/frankenphp-symfony is missing from composer.json.\n" +
			"Install it with: composer require runtime/frankenphp-symfony\n" +
			"Or disable worker mode: set frankenphp.worker: false in frankendeploy.yaml")
	}
	return nil
}
