// cmd/abort.go
package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/boneskull/gh-stack/internal/git"
	"github.com/boneskull/gh-stack/internal/state"
	"github.com/boneskull/gh-stack/internal/style"
	"github.com/spf13/cobra"
)

var abortCmd = &cobra.Command{
	Use:   "abort",
	Short: "Abort an operation in progress",
	Long:  `Abort a restack, submit, or sync operation and restore the original state.`,
	RunE:  runAbort,
}

func init() {
	rootCmd.AddCommand(abortCmd)
}

func runAbort(cmd *cobra.Command, args []string) error {
	s := style.New()

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	g := git.New(cwd)

	// Check if cascade in progress
	st, err := state.Load(g.GetGitDir())
	if err != nil {
		return errors.New("no operation in progress")
	}

	// Determine the correct git instance for rebase operations.
	// If the conflicting branch is in a linked worktree, operate there.
	rebaseGit := g
	wtPath := ""
	if p, ok := st.Worktrees[st.Current]; ok && p != "" {
		wtPath = p
		rebaseGit = git.New(wtPath)
	}

	// Abort rebase if in progress
	if rebaseGit.IsRebaseInProgress() {
		fmt.Println("Aborting rebase...")
		if err := rebaseGit.RebaseAbort(); err != nil {
			if wtPath != "" {
				return fmt.Errorf("failed to abort rebase in worktree at %s for branch %s: %w", wtPath, st.Current, err)
			}
			return fmt.Errorf("failed to abort rebase: %w", err)
		}
	}

	// Clean up state (ignore error - best effort cleanup)
	_ = state.Remove(g.GetGitDir()) //nolint:errcheck // best effort cleanup

	// Restore auto-stashed changes if any
	if st.StashRef != "" {
		fmt.Println("Restoring auto-stashed changes...")
		if popErr := g.StashPop(st.StashRef); popErr != nil {
			fmt.Printf("%s could not restore stashed changes (commit %s): %v\n", s.WarningIcon(), git.AbbrevSHA(st.StashRef), popErr)
		}
	}

	fmt.Printf("%s %s aborted. Original HEAD was %s\n", s.WarningIcon(), displayOperationName(st.Operation), st.OriginalHead)
	return nil
}
