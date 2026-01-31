package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yoanbernabeu/frankendeploy/internal/config"
	"github.com/yoanbernabeu/frankendeploy/internal/scanner"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize FrankenDeploy configuration",
	Long: `Analyzes your Symfony project and creates a frankendeploy.yaml
configuration file with detected settings.

This command will:
- Detect PHP version from composer.json
- Identify required PHP extensions
- Detect database driver (PostgreSQL, MySQL, SQLite)
- Identify asset build system (Webpack Encore, Vite, AssetMapper)`,
	RunE: runInit,
}

var (
	initName   string
	initForce  bool
	initDomain string
)

func init() {
	rootCmd.AddCommand(initCmd)
	initCmd.Flags().StringVarP(&initName, "name", "n", "", "Project name (default: directory name)")
	initCmd.Flags().BoolVarP(&initForce, "force", "f", false, "Overwrite existing configuration")
	initCmd.Flags().StringVarP(&initDomain, "domain", "d", "", "Domain for the application (e.g., demo.example.com)")
}

func runInit(cmd *cobra.Command, args []string) error {
	// Check if config already exists
	if config.ProjectConfigExists("") && !initForce {
		return fmt.Errorf("frankendeploy.yaml already exists (use --force to overwrite)")
	}

	PrintInfo("Analyzing project...")

	// Create scanner
	s := scanner.New(".")

	// Scan project
	result, err := s.Scan()
	if err != nil {
		return fmt.Errorf("failed to analyze project: %w", err)
	}

	// Determine project name
	projectName := initName
	if projectName == "" {
		cwd, _ := os.Getwd()
		projectName = sanitizeProjectName(filepath.Base(cwd))
	}

	// Convert scan result to config
	cfg := s.ToProjectConfig(result, projectName)

	// Apply domain if provided
	if initDomain != "" {
		cfg.Deploy.Domain = initDomain
	}

	// Validate configuration
	if errors := config.ValidateProjectConfig(cfg); errors.HasErrors() {
		PrintWarning("Configuration has validation issues: %s", errors.Error())
	}

	// Save configuration
	if err := config.SaveProjectConfig(cfg, ""); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	PrintSuccess("Created frankendeploy.yaml")

	// Print summary
	printInitSummary(result, cfg)

	return nil
}

func sanitizeProjectName(name string) string {
	// Convert to lowercase and replace invalid characters
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.ReplaceAll(name, "_", "-")

	// Remove any non-alphanumeric characters except hyphens
	var result strings.Builder
	for _, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			result.WriteRune(c)
		}
	}

	return result.String()
}

func printInitSummary(result *config.ScanResult, cfg *config.ProjectConfig) {
	fmt.Println()
	fmt.Println("📋 Project Configuration:")
	fmt.Printf("   Name:        %s\n", cfg.Name)
	fmt.Printf("   PHP:         %s\n", cfg.PHP.Version)
	fmt.Printf("   Extensions:  %s\n", strings.Join(cfg.PHP.Extensions, ", "))

	if cfg.Database.Driver != "" {
		fmt.Printf("   Database:    %s %s\n", cfg.Database.Driver, cfg.Database.Version)
	}

	if cfg.Assets.BuildTool != "" {
		fmt.Printf("   Assets:      %s\n", cfg.Assets.BuildTool)
	}

	if cfg.Deploy.Domain != "" {
		fmt.Printf("   Domain:      %s\n", cfg.Deploy.Domain)
	}

	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  1. Review frankendeploy.yaml and adjust if needed")
	fmt.Println("  2. Run 'frankendeploy build' to generate Docker files")
	fmt.Println("  3. Run 'frankendeploy dev up' to start local development")
}
