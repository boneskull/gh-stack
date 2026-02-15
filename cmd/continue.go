// cmd/continue.go
package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/git"
	"github.com/boneskull/gh-stack/internal/state"
	"github.com/boneskull/gh-stack/internal/style"
	"github.com/boneskull/gh-stack/internal/tree"
	"github.com/spf13/cobra"
)

var continueCmd = &cobra.Command{
	Use:   "continue",
	Short: "Continue an operation after resolving conflicts",
	Long:  `Continue a restack, submit, or sync operation after resolving rebase conflicts.`,
	RunE:  runContinue,
}

func init() {
	rootCmd.AddCommand(continueCmd)
}

func runContinue(cmd *cobra.Command, args []string) error {
	s := style.New()

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	g := git.New(cwd)

	// Check if operation in progress
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

	// Complete the in-progress rebase
	if rebaseGit.IsRebaseInProgress() {
		fmt.Println("Continuing rebase...")
		if rebaseErr := rebaseGit.RebaseContinue(); rebaseErr != nil {
			if wtPath != "" {
				return fmt.Errorf("rebase --continue failed in worktree at %s for branch %s; resolve conflicts there first", wtPath, st.Current)
			}
			return errors.New("rebase --continue failed; resolve conflicts first")
		}
	}

	fmt.Printf("%s Completed %s\n", s.SuccessIcon(), s.Branch(st.Current))

	cfg, err := config.New(g)
	if err != nil {
		return err
	}

	// Build tree to get node objects
	root, err := tree.Build(cfg)
	if err != nil {
		return err
	}

	// If there are more branches to cascade, continue cascading
	if len(st.Pending) > 0 {
		var branches []*tree.Node
		for _, name := range st.Pending {
			if node := tree.FindNode(root, name); node != nil {
				branches = append(branches, node)
			}
		}

		// Remove state file before continuing (will be recreated if conflict)
		_ = state.Remove(g.GetGitDir()) //nolint:errcheck // cleanup

		if cascadeErr := doCascadeWithState(g, cfg, branches, false, st.Operation, st.UpdateOnly, st.Web, st.PushOnly, st.Branches, st.StashRef, st.Worktrees, s); cascadeErr != nil {
			// Stash handling is done by doCascadeWithState (conflict saves in state, errors restore)
			if !errors.Is(cascadeErr, ErrConflict) && st.StashRef != "" {
				fmt.Println("Restoring auto-stashed changes...")
				if popErr := g.StashPop(st.StashRef); popErr != nil {
					fmt.Printf("%s could not restore stashed changes (commit %s): %v\n", s.WarningIcon(), git.AbbrevSHA(st.StashRef), popErr)
				}
			}
			return cascadeErr // Another conflict - state saved
		}
	} else {
		// No more branches to cascade - cleanup state
		_ = state.Remove(g.GetGitDir()) //nolint:errcheck // cleanup
	}

	// If this was a submit operation, continue with push + PR phases
	if st.Operation == state.OperationSubmit {
		// Rebuild branches list from the original set of submit branches if available.
		// Fall back to the current + pending branches for backward compatibility.
		var branchNames []string
		if len(st.Branches) > 0 {
			branchNames = st.Branches
		} else {
			branchNames = append(branchNames, st.Current)
			branchNames = append(branchNames, st.Pending...)
		}

		var allBranches []*tree.Node
		for _, name := range branchNames {
			node := tree.FindNode(root, name)
			if node == nil {
				// Preserve existing behaviour: fail fast if a branch from state
				// cannot be found in the current tree.
				if name == st.Current {
					return fmt.Errorf("branch %q not found in tree", st.Current)
				}
				continue
			}
			allBranches = append(allBranches, node)
		}

		err = doSubmitPushAndPR(g, cfg, root, allBranches, false, st.UpdateOnly, st.Web, st.PushOnly, s)
		// Restore stash after submit completes
		if st.StashRef != "" {
			fmt.Println("Restoring auto-stashed changes...")
			if popErr := g.StashPop(st.StashRef); popErr != nil {
				fmt.Printf("%s could not restore stashed changes (commit %s): %v\n", s.WarningIcon(), git.AbbrevSHA(st.StashRef), popErr)
			}
		}
		return err
	}

	// Restore stash after cascade completes
	if st.StashRef != "" {
		fmt.Println("Restoring auto-stashed changes...")
		if popErr := g.StashPop(st.StashRef); popErr != nil {
			fmt.Printf("%s could not restore stashed changes (commit %s): %v\n", s.WarningIcon(), git.AbbrevSHA(st.StashRef), popErr)
		}
	}

	fmt.Println(s.SuccessMessage("Restack complete!"))
	return nil
}
