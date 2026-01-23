// cmd/abort.go
package cmd

import (
	"fmt"
	"os"

	"github.com/boneskull/gh-stack/internal/git"
	"github.com/boneskull/gh-stack/internal/state"
	"github.com/spf13/cobra"
)

var abortCmd = &cobra.Command{
	Use:   "abort",
	Short: "Abort a cascade in progress",
	Long:  `Abort a cascade operation and restore the original state.`,
	RunE:  runAbort,
}

func init() {
	rootCmd.AddCommand(abortCmd)
}

func runAbort(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	g := git.New(cwd)

	// Check if cascade in progress
	st, err := state.Load(g.GetGitDir())
	if err != nil {
		return fmt.Errorf("no cascade in progress")
	}

	// Abort rebase if in progress
	if g.IsRebaseInProgress() {
		fmt.Println("Aborting rebase...")
		if err := g.RebaseAbort(); err != nil {
			return fmt.Errorf("failed to abort rebase: %w", err)
		}
	}

	// Clean up state
	state.Remove(g.GetGitDir())

	fmt.Printf("Cascade aborted. Original HEAD was %s\n", st.OriginalHead)
	return nil
}
