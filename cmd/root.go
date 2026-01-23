// cmd/root.go
package cmd

import (
	"github.com/spf13/cobra"
)

// version is set at build time via ldflags.
var version = "dev"

func init() {
	rootCmd.Version = version
}

var rootCmd = &cobra.Command{
	Use:   "gh-stack",
	Short: "Manage stacked pull requests",
	Long:  `gh-stack tracks parent-child relationships between branches, enabling workflows where PRs target other PRs.`,
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}
