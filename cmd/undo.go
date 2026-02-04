// cmd/undo.go
package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/git"
	"github.com/boneskull/gh-stack/internal/prompt"
	"github.com/boneskull/gh-stack/internal/state"
	"github.com/boneskull/gh-stack/internal/undo"
	"github.com/spf13/cobra"
)

var undoCmd = &cobra.Command{
	Use:   "undo",
	Short: "Undo the last destructive operation",
	Long: `Undo the last cascade, submit, or sync operation by restoring branches
to their previous state. This includes:

- Resetting branch refs to their pre-operation SHAs
- Recreating any deleted branches
- Restoring stack config (stackParent, stackPR, stackForkPoint)
- Restoring any auto-stashed working tree changes

Note: This does not undo remote changes (force pushes). Use with caution.`,
	RunE: runUndo,
}

var (
	undoForce  bool
	undoDryRun bool
)

func init() {
	rootCmd.AddCommand(undoCmd)
	undoCmd.Flags().BoolVarP(&undoForce, "force", "f", false, "Skip confirmation prompt")
	undoCmd.Flags().BoolVar(&undoDryRun, "dry-run", false, "Show what would be restored without making changes")
}

func runUndo(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	g := git.New(cwd)
	gitDir := g.GetGitDir()

	// Check if a cascade/submit is in progress
	if state.Exists(gitDir) {
		return errors.New("cannot undo while a cascade/submit is in progress; run 'gh stack continue' or 'gh stack abort' first")
	}

	// Check if a rebase is in progress
	if g.IsRebaseInProgress() {
		return errors.New("cannot undo while a rebase is in progress; resolve or abort the rebase first")
	}

	// Load the most recent snapshot
	snapshot, snapshotPath, err := undo.LoadLatest(gitDir)
	if err != nil {
		if errors.Is(err, undo.ErrNoSnapshot) {
			fmt.Println("Nothing to undo.")
			return nil
		}
		return fmt.Errorf("failed to load undo state: %w", err)
	}

	// Display what will be restored
	fmt.Printf("About to undo '%s' from %s\n\n", snapshot.Command, snapshot.Timestamp.Local().Format("2006-01-02 15:04:05"))
	fmt.Println("This will restore:")

	// Show branch changes
	for name, bs := range snapshot.Branches {
		currentSHA, tipErr := g.GetTip(name)
		if tipErr != nil {
			fmt.Printf("  - %s: (branch missing) → %s\n", name, bs.SHA[:7])
		} else if currentSHA != bs.SHA {
			fmt.Printf("  - %s: %s → %s\n", name, currentSHA[:7], bs.SHA[:7])
		} else {
			fmt.Printf("  - %s: (unchanged)\n", name)
		}
	}

	// Show deleted branches to recreate
	for name, bs := range snapshot.DeletedBranches {
		if g.BranchExists(name) {
			fmt.Printf("  - %s: (already exists, will skip)\n", name)
		} else {
			fmt.Printf("  - Recreate deleted branch: %s at %s\n", name, bs.SHA[:7])
		}
	}

	// Show stash info
	if snapshot.StashRef != "" {
		fmt.Printf("  - Restore stashed changes\n")
	}

	fmt.Println()

	// Dry run exits here
	if undoDryRun {
		fmt.Println("Dry run: no changes made.")
		return nil
	}

	// Confirm unless forced
	if !undoForce {
		confirmed, confirmErr := prompt.Confirm("Proceed with undo?", false)
		if confirmErr != nil {
			return confirmErr
		}
		if !confirmed {
			fmt.Println("Undo cancelled.")
			return nil
		}
	}

	// Load config for restoring stack metadata
	cfg, err := config.Load(cwd)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Verify all SHAs exist before making changes
	for name, bs := range snapshot.Branches {
		if !g.CommitExists(bs.SHA) {
			return fmt.Errorf("commit %s for branch '%s' no longer exists (reflog may have expired)", bs.SHA, name)
		}
	}
	for name, bs := range snapshot.DeletedBranches {
		if !g.CommitExists(bs.SHA) {
			return fmt.Errorf("commit %s for deleted branch '%s' no longer exists (reflog may have expired)", bs.SHA, name)
		}
	}

	// Perform the undo
	fmt.Println("Restoring branches...")

	// Get current branch - we may need to checkout something else first
	currentBranch, _ := g.CurrentBranch() //nolint:errcheck // empty is fine

	// Build a list of branches we need to reset
	// We need to handle the case where current branch is one we want to reset
	branchesToReset := make(map[string]undo.BranchState)
	for name, bs := range snapshot.Branches {
		currentSHA, err := g.GetTip(name)
		if err != nil {
			// Branch doesn't exist, create it (can be done while checked out)
			fmt.Printf("  Creating %s at %s\n", name, bs.SHA[:7])
			if createErr := g.CreateBranchAt(name, bs.SHA); createErr != nil {
				return fmt.Errorf("failed to create branch %s: %w", name, createErr)
			}
		} else if currentSHA != bs.SHA {
			branchesToReset[name] = bs
		}

		// Restore config
		if configErr := restoreBranchConfig(cfg, name, bs); configErr != nil {
			fmt.Printf("  Warning: failed to restore config for %s: %v\n", name, configErr)
		}
	}

	// If current branch needs resetting, checkout a temp detached HEAD first
	if _, needsReset := branchesToReset[currentBranch]; needsReset && len(branchesToReset) > 0 {
		// Detach HEAD so we can reset the current branch
		currentSHA, _ := g.GetTip("HEAD") //nolint:errcheck // if this fails, checkout will too
		detachErr := g.Checkout(currentSHA)
		if detachErr != nil {
			// Fall back: try to find a branch we don't need to reset
			foundSafe := false
			for name := range snapshot.Branches {
				if _, needsReset := branchesToReset[name]; !needsReset {
					if safeCheckoutErr := g.Checkout(name); safeCheckoutErr == nil {
						foundSafe = true
						break
					}
				}
			}
			if !foundSafe {
				return fmt.Errorf("cannot checkout to reset current branch: %w", detachErr)
			}
		}
	}

	// Now reset all branches that need it
	for name, bs := range branchesToReset {
		fmt.Printf("  Resetting %s to %s\n", name, bs.SHA[:7])
		if resetErr := g.SetBranchRef(name, bs.SHA); resetErr != nil {
			return fmt.Errorf("failed to reset branch %s: %w", name, resetErr)
		}
	}

	// Recreate deleted branches
	for name, bs := range snapshot.DeletedBranches {
		if g.BranchExists(name) {
			fmt.Printf("  Skipping %s (already exists)\n", name)
			continue
		}
		fmt.Printf("  Recreating %s at %s\n", name, bs.SHA[:7])
		if createErr := g.CreateBranchAt(name, bs.SHA); createErr != nil {
			return fmt.Errorf("failed to recreate branch %s: %w", name, createErr)
		}

		// Restore config for deleted branch
		if configErr := restoreBranchConfig(cfg, name, bs); configErr != nil {
			fmt.Printf("  Warning: failed to restore config for %s: %v\n", name, configErr)
		}
	}

	// Checkout original HEAD
	if snapshot.OriginalHead != "" {
		fmt.Printf("  Checking out %s\n", snapshot.OriginalHead)
		if checkoutErr := g.Checkout(snapshot.OriginalHead); checkoutErr != nil {
			fmt.Printf("  Warning: failed to checkout %s: %v\n", snapshot.OriginalHead, checkoutErr)
		}
	}

	// Restore stash
	if snapshot.StashRef != "" {
		fmt.Println("  Restoring stashed changes...")
		if err := g.StashPop(); err != nil {
			fmt.Printf("  Warning: could not cleanly restore stashed changes. Your changes are still in %s.\n", snapshot.StashRef)
		}
	}

	// Archive the snapshot
	if err := undo.Archive(gitDir, snapshotPath); err != nil {
		fmt.Printf("Warning: failed to archive undo state: %v\n", err)
	}

	fmt.Printf("\nUndo complete. Restored state from before '%s'.\n", snapshot.Command)
	return nil
}

// restoreBranchConfig restores the stack config for a branch.
func restoreBranchConfig(cfg *config.Config, branch string, bs undo.BranchState) error {
	// Restore stackParent
	if bs.StackParent != "" {
		if err := cfg.SetParent(branch, bs.StackParent); err != nil {
			return err
		}
	} else {
		// No parent was set - ensure it's removed
		if err := cfg.RemoveParent(branch); err != nil {
			return err
		}
	}

	// Restore stackPR
	if bs.StackPR != 0 {
		if err := cfg.SetPR(branch, bs.StackPR); err != nil {
			return err
		}
	} else {
		if err := cfg.RemovePR(branch); err != nil {
			return err
		}
	}

	// Restore stackForkPoint
	if bs.StackForkPoint != "" {
		if err := cfg.SetForkPoint(branch, bs.StackForkPoint); err != nil {
			return err
		}
	} else {
		if err := cfg.RemoveForkPoint(branch); err != nil {
			return err
		}
	}

	return nil
}
