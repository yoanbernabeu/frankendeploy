package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/yoanbernabeu/frankendeploy/internal/security"
)

var (
	// Version is set at build time
	Version = "dev"

	// Global flags
	verbose   bool
	cfgFile   string
	yesFlag   bool // CI/CD: skip confirmations
)

var rootCmd = &cobra.Command{
	Use:   "frankendeploy",
	Short: "Deploy Symfony applications with FrankenPHP",
	Long: `FrankenDeploy is a CLI to deploy Symfony applications on VPS
with FrankenPHP. It analyzes your project, generates Docker files
and orchestrates deployment.

Quick start:
  frankendeploy init         # Initialize configuration
  frankendeploy dev up       # Start dev environment
  frankendeploy deploy prod  # Deploy to production

Commands:
  init          Analyze project and create frankendeploy.yaml
  build         Generate Dockerfile and compose files
  dev           Manage local development environment
  server        Configure deployment servers
  deploy        Deploy application to server
  rollback      Rollback to previous release
  logs          View application logs
  shell         Open shell in container
  exec          Execute command in container
  app           Manage deployed applications

CI/CD Environment Variables:
  FRANKENDEPLOY_SERVER              Default server name
  FRANKENDEPLOY_SSH_KEY             SSH private key content
  FRANKENDEPLOY_KNOWN_HOSTS         SSH known_hosts content
  FRANKENDEPLOY_SKIP_HOST_KEY_CHECK Skip host key verification (true/false)`,
	Version: Version,
}

// Execute runs the root command
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Show detailed logs")
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "Config file (default: frankendeploy.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&yesFlag, "yes", "y", false, "Skip confirmations (CI/CD mode)")

	rootCmd.SetVersionTemplate(`FrankenDeploy {{.Version}}
`)
}

// IsVerbose returns true if verbose mode is enabled
func IsVerbose() bool {
	return verbose
}

// GetConfigFile returns the config file path
func GetConfigFile() string {
	return cfgFile
}

// IsYesMode returns true if --yes flag is set (CI/CD mode)
func IsYesMode() bool {
	return yesFlag
}

// PrintError prints a formatted error message
func PrintError(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "❌ "+msg+"\n", args...)
}

// PrintSuccess prints a success message
func PrintSuccess(msg string, args ...interface{}) {
	fmt.Printf("✅ "+msg+"\n", args...)
}

// PrintInfo prints an info message
func PrintInfo(msg string, args ...interface{}) {
	fmt.Printf("ℹ️  "+msg+"\n", args...)
}

// PrintWarning prints a warning message
func PrintWarning(msg string, args ...interface{}) {
	fmt.Printf("⚠️  "+msg+"\n", args...)
}

// PrintVerbose prints a message only in verbose mode
func PrintVerbose(msg string, args ...interface{}) {
	if verbose {
		fmt.Printf("   "+msg+"\n", args...)
	}
}

// PrintVerboseCommand prints a command in verbose mode with sensitive values masked
func PrintVerboseCommand(command string) {
	if verbose {
		fmt.Printf("   Running: %s\n", security.SanitizeCommandForLog(command))
	}
}
