package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var devCmd = &cobra.Command{
	Use:   "dev",
	Short: "Manage local development environment",
	Long:  `Commands to manage the local Docker development environment.`,
}

var devUpCmd = &cobra.Command{
	Use:   "up",
	Short: "Start development environment",
	Long:  `Starts the Docker Compose development environment.`,
	RunE:  runDevUp,
}

var devDownCmd = &cobra.Command{
	Use:   "down",
	Short: "Stop development environment",
	Long:  `Stops and removes the Docker Compose development environment.`,
	RunE:  runDevDown,
}

var devLogsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Show development environment logs",
	Long:  `Shows logs from the Docker Compose development environment.`,
	RunE:  runDevLogs,
}

var devRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart development environment",
	Long:  `Restarts the Docker Compose development environment.`,
	RunE:  runDevRestart,
}

var (
	devBuild  bool
	devDetach bool
	devFollow bool
	devTail   string
)

func init() {
	rootCmd.AddCommand(devCmd)
	devCmd.AddCommand(devUpCmd)
	devCmd.AddCommand(devDownCmd)
	devCmd.AddCommand(devLogsCmd)
	devCmd.AddCommand(devRestartCmd)

	devUpCmd.Flags().BoolVarP(&devBuild, "build", "b", false, "Build images before starting")
	devUpCmd.Flags().BoolVarP(&devDetach, "detach", "d", true, "Run in background")

	devLogsCmd.Flags().BoolVarP(&devFollow, "follow", "f", true, "Follow log output")
	devLogsCmd.Flags().StringVar(&devTail, "tail", "100", "Number of lines to show")
}

func runDevUp(cmd *cobra.Command, args []string) error {
	if err := checkComposeFile(); err != nil {
		return err
	}

	PrintInfo("Starting development environment...")

	composeArgs := []string{"compose", "up"}
	if devBuild {
		composeArgs = append(composeArgs, "--build")
	}
	if devDetach {
		composeArgs = append(composeArgs, "-d")
	}

	if err := runDockerCompose(composeArgs...); err != nil {
		return err
	}

	if devDetach {
		PrintSuccess("Development environment started")
		fmt.Println()
		fmt.Println("🌐 Application: http://localhost:8000")
		fmt.Println()
		fmt.Println("Commands:")
		fmt.Println("  frankendeploy dev logs    - View logs")
		fmt.Println("  frankendeploy dev down    - Stop environment")
	}

	return nil
}

func runDevDown(cmd *cobra.Command, args []string) error {
	if err := checkComposeFile(); err != nil {
		return err
	}

	PrintInfo("Stopping development environment...")

	if err := runDockerCompose("compose", "down"); err != nil {
		return err
	}

	PrintSuccess("Development environment stopped")
	return nil
}

func runDevLogs(cmd *cobra.Command, args []string) error {
	if err := checkComposeFile(); err != nil {
		return err
	}

	composeArgs := []string{"compose", "logs"}
	if devFollow {
		composeArgs = append(composeArgs, "-f")
	}
	composeArgs = append(composeArgs, "--tail", devTail)

	return runDockerCompose(composeArgs...)
}

func runDevRestart(cmd *cobra.Command, args []string) error {
	if err := checkComposeFile(); err != nil {
		return err
	}

	PrintInfo("Restarting development environment...")

	if err := runDockerCompose("compose", "restart"); err != nil {
		return err
	}

	PrintSuccess("Development environment restarted")
	return nil
}

func checkComposeFile() error {
	if _, err := os.Stat("compose.yaml"); os.IsNotExist(err) {
		return fmt.Errorf("compose.yaml not found (run 'frankendeploy build' first)")
	}
	return nil
}

func runDockerCompose(args ...string) error {
	dockerCmd := exec.Command("docker", args...)
	dockerCmd.Stdout = os.Stdout
	dockerCmd.Stderr = os.Stderr
	dockerCmd.Stdin = os.Stdin

	if err := dockerCmd.Run(); err != nil {
		return fmt.Errorf("docker compose failed: %w", err)
	}

	return nil
}
