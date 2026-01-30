package cmd

import "github.com/spf13/cobra"

// GetRootCmd returns the root command for documentation generation
func GetRootCmd() *cobra.Command {
	return rootCmd
}
