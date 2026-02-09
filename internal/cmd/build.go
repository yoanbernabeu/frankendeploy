package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/yoanbernabeu/frankendeploy/internal/config"
	"github.com/yoanbernabeu/frankendeploy/internal/generator"
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
