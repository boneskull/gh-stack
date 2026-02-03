// cmd/continue.go
package cmd

import (
	"fmt"
	"os"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/git"
	"github.com/boneskull/gh-stack/internal/state"
	"github.com/boneskull/gh-stack/internal/tree"
	"github.com/spf13/cobra"
)

var continueCmd = &cobra.Command{
	Use:   "continue",
	Short: "Continue an operation after resolving conflicts",
	Long:  `Continue a cascade or submit operation after resolving rebase conflicts.`,
	RunE:  runContinue,
}

func init() {
	rootCmd.AddCommand(continueCmd)
}

func runContinue(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	g := git.New(cwd)

	// Check if operation in progress
	st, err := state.Load(g.GetGitDir())
	if err != nil {
		return fmt.Errorf("no operation in progress")
	}

	// Complete the in-progress rebase
	if g.IsRebaseInProgress() {
		fmt.Println("Continuing rebase...")
		if rebaseErr := g.RebaseContinue(); rebaseErr != nil {
			return fmt.Errorf("rebase --continue failed; resolve conflicts first")
		}
	}

	fmt.Printf("Completed %s\n", st.Current)

	cfg, err := config.Load(cwd)
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

		if err := doCascadeWithState(g, cfg, branches, false, st.Operation, st.UpdateOnly, st.Branches); err != nil {
			return err // Another conflict - state saved
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

		return doSubmitPushAndPR(g, cfg, root, allBranches, false, st.UpdateOnly, false)
	}

	fmt.Println("Cascade complete!")
	return nil
}
