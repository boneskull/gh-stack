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
	// SilenceUsage prevents Cobra from printing the help/usage text when a
	// command's RunE returns an error.  Without this, any runtime error
	// (e.g. a rebase conflict) causes the full usage output to be shown,
	// which is confusing and unhelpful.
	SilenceUsage: true,
}

// Execute runs the root command.
func Execute() error {
	if err := expandCommandShortcut(rootCmd); err != nil {
		rootCmd.PrintErrln(err)
		return err
	}
	return rootCmd.Execute()
}
