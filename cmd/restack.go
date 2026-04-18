// cmd/restack.go
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
	"github.com/boneskull/gh-stack/internal/undo"
	"github.com/spf13/cobra"
)

// ErrConflict indicates a rebase conflict occurred during restack.
var ErrConflict = errors.New("rebase conflict: resolve and run 'gh stack continue', or 'gh stack abort'")

var restackCmd = &cobra.Command{
	Use:     "restack",
	Aliases: []string{"cascade"},
	Short:   "Rebase current branch and descendants onto their parents",
	Long:    `Rebase the current branch onto its parent, then recursively restack descendants.`,
	RunE:    runRestack,
}

var (
	restackOnlyFlag      bool
	restackDryRunFlag    bool
	restackWorktreesFlag bool
)

func init() {
	restackCmd.Flags().BoolVarP(&restackOnlyFlag, "current", "c", false, "only restack current branch, not descendants")
	restackCmd.Flags().BoolVarP(&restackDryRunFlag, "dry-run", "D", false, "show what would be done")
	restackCmd.Flags().BoolVarP(&restackWorktreesFlag, "worktrees", "w", false, "rebase branches checked out in linked worktrees in-place")
	rootCmd.AddCommand(restackCmd)
}

func runRestack(cmd *cobra.Command, args []string) error {
	s := style.New()

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	cfg, err := config.Load(cwd)
	if err != nil {
		return err
	}

	g := git.New(cwd)

	// Check if restack already in progress
	if state.Exists(g.GetGitDir()) {
		return errors.New("operation already in progress; use 'gh stack continue' or 'gh stack abort'")
	}

	currentBranch, err := g.CurrentBranch()
	if err != nil {
		return err
	}

	// Build tree
	root, err := tree.Build(cfg)
	if err != nil {
		return err
	}

	node := tree.FindNode(root, currentBranch)
	if node == nil {
		trunk, _ := cfg.GetTrunk() //nolint:errcheck // empty is fine for error message
		return fmt.Errorf("branch %q is not tracked in the stack\n\nTo add it, run:\n  gh stack adopt %s    # to stack on %s\n  gh stack adopt -p <parent>    # to stack on a different branch", currentBranch, trunk, trunk)
	}

	// Collect branches to restack
	var branches []*tree.Node
	branches = append(branches, node)
	if !restackOnlyFlag {
		branches = append(branches, tree.GetDescendants(node)...)
	}

	// Save undo snapshot (unless dry-run)
	var stashRef string
	if !restackDryRunFlag {
		var saveErr error
		stashRef, saveErr = saveUndoSnapshot(g, cfg, branches, nil, "restack", "gh stack restack", s)
		if saveErr != nil {
			fmt.Printf("%s could not save undo state: %v\n", s.WarningIcon(), saveErr)
		}
	}

	// Build worktree map if --worktrees flag is set
	var worktrees map[string]string
	if restackWorktreesFlag {
		var wtErr error
		worktrees, wtErr = g.ListWorktrees()
		if wtErr != nil {
			return fmt.Errorf("failed to list worktrees: %w", wtErr)
		}
	}

	err = doRestackWithState(g, cfg, branches, RestackOptions{
		DryRun:    restackDryRunFlag,
		Operation: state.OperationRestack,
		StashRef:  stashRef,
		Worktrees: worktrees,
	}, s)

	// Restore auto-stashed changes after operation (unless conflict, which saves stash in state)
	if stashRef != "" && !errors.Is(err, ErrConflict) {
		fmt.Println("Restoring auto-stashed changes...")
		if popErr := g.StashPop(stashRef); popErr != nil {
			fmt.Printf("%s could not restore stashed changes (commit %s): %v\n", s.WarningIcon(), git.AbbrevSHA(stashRef), popErr)
		}
	}

	return err
}

// RestackOptions configures the behaviour of doRestackWithState.
//
// The submit-specific fields (UpdateOnly, OpenWeb, PushOnly, Branches) are
// only meaningful when Operation is state.OperationSubmit; they are persisted
// to restack state so that the push/PR phases can be resumed after a conflict.
type RestackOptions struct {
	// DryRun prints what would be done without actually rebasing.
	DryRun bool
	// Operation is the type of operation being performed (state.OperationRestack
	// or state.OperationSubmit).
	Operation string
	// UpdateOnly skips creating new PRs; only existing PRs are updated.
	// Submit-only.
	UpdateOnly bool
	// OpenWeb opens PRs in the browser after creation/update. Submit-only.
	OpenWeb bool
	// PushOnly skips the PR creation/update phase entirely. Submit-only.
	PushOnly bool
	// Branches is the complete list of branch names being submitted, used
	// to rebuild the full set for push/PR phases after restack completes.
	// Submit-only. Mirrors state.RestackState.Branches.
	Branches []string
	// StashRef is the commit hash of auto-stashed changes (if any), persisted
	// to state so they can be restored when the operation completes or is aborted.
	StashRef string
	// Worktrees maps branch names to linked worktree paths. When non-nil, branches
	// present in the map are rebased directly in their worktree directory instead
	// of being checked out in the main working tree.
	Worktrees map[string]string
}

// doRestackWithState performs restack and saves state with the given operation type.
func doRestackWithState(g *git.Git, cfg *config.Config, branches []*tree.Node, opts RestackOptions, s *style.Style) error {
	originalBranch, err := g.CurrentBranch()
	if err != nil {
		return err
	}
	originalHead, err := g.GetTip(originalBranch)
	if err != nil {
		return err
	}

	for i, b := range branches {
		parent, err := cfg.GetParent(b.Name)
		if err != nil {
			continue // trunk or untracked
		}

		// Check if rebase needed
		needsRebase, err := g.NeedsRebase(b.Name, parent)
		if err != nil {
			return err
		}

		if !needsRebase {
			fmt.Printf("Restacking %s... %s\n", s.Branch(b.Name), s.Muted("already up to date"))

			// Refresh fork point even when no rebase is needed. If the branch
			// was rebased outside gh-stack the stored fork point would be stale;
			// keeping it current prevents a future --onto rebase from replaying
			// too many commits.
			if !opts.DryRun {
				parentTip, tipErr := g.GetTip(parent)
				if tipErr == nil {
					_ = cfg.SetForkPoint(b.Name, parentTip) //nolint:errcheck // best effort
				}
			}
			continue
		}

		if opts.DryRun {
			fmt.Printf("%s Would rebase %s onto %s\n", s.Muted("dry-run:"), s.Branch(b.Name), s.Branch(parent))
			continue
		}

		// Check if we should use --onto rebase.
		// This is needed when the parent has been rebased/amended since the child was created.
		storedForkPoint, fpErr := cfg.GetForkPoint(b.Name)
		useOnto := false

		if fpErr == nil {
			if !g.CommitExists(storedForkPoint) {
				// The stored fork point no longer exists — it may have been
				// garbage collected after a history rewrite or a sufficiently
				// aggressive `git gc`. Without it we cannot use --onto, so we
				// fall back to a plain rebase against the parent tip. If the
				// parent's history was genuinely rewritten this fallback may
				// produce spurious conflicts or silently re-apply commits that
				// were already in the parent; the user should resolve conflicts
				// as normal or re-run `gh stack restack` after history settles.
				fmt.Printf("  %s\n", s.Muted(fmt.Sprintf(
					"warning: stored fork point %s is no longer available (garbage collected?); falling back to simple rebase",
					git.AbbrevSHA(storedForkPoint),
				)))
			} else {
				// Fork point is reachable — determine whether --onto is appropriate.
				currentMergeBase, mbErr := g.GetMergeBase(b.Name, parent)
				if mbErr == nil && currentMergeBase != storedForkPoint {
					// Fork point differs from merge-base. Determine why:
					//
					// If the stored fork point is an ancestor of the merge-base,
					// it's just stale (e.g. branch was rebased outside gh-stack,
					// or fork point wasn't updated after a conflict resolution).
					// A simple rebase using the merge-base is correct; refresh the
					// fork point so it stays current.
					//
					// If the stored fork point is NOT an ancestor of the merge-base,
					// the parent's history was rewritten (squash merge, force push).
					// We need --onto with the stored fork point to identify the
					// correct commit range.
					if g.IsAncestor(storedForkPoint, currentMergeBase) {
						_ = cfg.SetForkPoint(b.Name, currentMergeBase) //nolint:errcheck // best effort
					} else {
						useOnto = true
					}
				}
			}
		}

		// Determine if this branch lives in a linked worktree
		wtPath := ""
		if opts.Worktrees != nil {
			wtPath = opts.Worktrees[b.Name]
		}

		if useOnto {
			fmt.Printf("Restacking %s onto %s %s...\n", s.Branch(b.Name), s.Branch(parent), s.Muted("(using fork point)"))
		} else {
			fmt.Printf("Restacking %s onto %s...\n", s.Branch(b.Name), s.Branch(parent))
		}

		var rebaseErr error
		if wtPath != "" {
			// Branch is checked out in a linked worktree -- rebase there directly
			fmt.Printf("  %s\n", s.Muted(fmt.Sprintf("Using worktree at %s for %s", wtPath, b.Name)))
			gitWt := git.New(wtPath)
			if useOnto {
				rebaseErr = gitWt.RebaseOntoHere(parent, storedForkPoint)
			} else {
				rebaseErr = gitWt.RebaseHere(parent)
			}
			// If git failed for a non-conflict reason (e.g. worktree dir was removed),
			// wrap the error with context so the user knows which worktree we tried.
			if rebaseErr != nil && !gitWt.IsRebaseInProgress() {
				return fmt.Errorf("rebase of %s in worktree at %s failed (was the worktree removed or moved?): %w", b.Name, wtPath, rebaseErr)
			}
		} else {
			// Normal flow: checkout + rebase in the main repo
			if err := g.Checkout(b.Name); err != nil {
				return err
			}

			if useOnto {
				rebaseErr = g.RebaseOnto(parent, storedForkPoint, b.Name)
			} else {
				rebaseErr = g.Rebase(parent)
			}
		}

		if rebaseErr != nil {
			// Rebase conflict - save state
			remaining := make([]string, 0, len(branches)-i-1)
			for _, r := range branches[i+1:] {
				remaining = append(remaining, r.Name)
			}

			st := &state.RestackState{
				Current:      b.Name,
				Pending:      remaining,
				OriginalHead: originalHead,
				Operation:    opts.Operation,
				UpdateOnly:   opts.UpdateOnly,
				Web:          opts.OpenWeb,
				PushOnly:     opts.PushOnly,
				Branches:     opts.Branches,
				StashRef:     opts.StashRef,
				Worktrees:    opts.Worktrees,
			}
			_ = state.Save(g.GetGitDir(), st) //nolint:errcheck // best effort - user can recover manually

			fmt.Printf("\n%s %s\n", s.FailureIcon(), s.Error("CONFLICT: Resolve conflicts and run 'gh stack continue', or 'gh stack abort' to cancel."))
			if wtPath != "" {
				fmt.Printf("Resolve conflicts in worktree: %s\n", wtPath)
			}
			fmt.Printf("Remaining branches: %v\n", remaining)
			if opts.StashRef != "" {
				fmt.Println(s.Muted("Note: Your uncommitted changes are stashed and will be restored when you continue or abort."))
			}
			return ErrConflict
		}

		fmt.Printf("Restacking %s... %s\n", s.Branch(b.Name), s.Success("ok"))

		// Update fork point to current parent tip
		parentTip, tipErr := g.GetTip(parent)
		if tipErr == nil {
			_ = cfg.SetForkPoint(b.Name, parentTip) //nolint:errcheck // best effort
		}
	}

	// Return to original branch
	if !opts.DryRun {
		_ = g.Checkout(originalBranch) //nolint:errcheck // best effort - restack succeeded
	}

	return nil
}

// displayOperationName maps internal operation constants to user-facing names.
func displayOperationName(op string) string {
	switch op {
	case state.OperationRestack:
		return "Restack"
	case state.OperationSubmit:
		return "Submit"
	default:
		return "Operation"
	}
}

// saveUndoSnapshot captures the current state of branches before a destructive operation.
// It auto-stashes any uncommitted changes if the working tree is dirty.
// branches: branches that will be modified (rebased)
// deletedBranches: branches that will be deleted (for sync)
// Returns the stash ref (commit hash) if changes were stashed, empty string otherwise.
func saveUndoSnapshot(g *git.Git, cfg *config.Config, branches []*tree.Node, deletedBranches []*tree.Node, operation, command string, s *style.Style) (string, error) {
	gitDir := g.GetGitDir()

	// Get current branch for original head
	currentBranch, err := g.CurrentBranch()
	if err != nil {
		return "", err
	}

	// Create snapshot
	snapshot := undo.NewSnapshot(operation, command, currentBranch)

	// Auto-stash if dirty
	var stashRef string
	dirty, err := g.IsDirty()
	if err != nil {
		return "", err
	}
	if dirty {
		var stashErr error
		stashRef, stashErr = g.Stash("gh-stack auto-stash before " + operation)
		if stashErr != nil {
			return "", fmt.Errorf("failed to stash changes: %w", stashErr)
		}
		if stashRef != "" {
			snapshot.StashRef = stashRef
			fmt.Println(s.Muted("Auto-stashed uncommitted changes"))
		}
	}

	// Capture state of branches that will be modified
	for _, node := range branches {
		bs, captureErr := captureBranchState(g, cfg, node.Name)
		if captureErr != nil {
			// Non-fatal: log warning and continue
			fmt.Printf("%s could not capture state for %s: %v\n", s.WarningIcon(), s.Branch(node.Name), captureErr)
			continue
		}
		snapshot.Branches[node.Name] = bs
	}

	// Capture state of branches that will be deleted
	for _, node := range deletedBranches {
		bs, captureErr := captureBranchState(g, cfg, node.Name)
		if captureErr != nil {
			fmt.Printf("%s could not capture state for deleted branch %s: %v\n", s.WarningIcon(), s.Branch(node.Name), captureErr)
			continue
		}
		snapshot.DeletedBranches[node.Name] = bs
	}

	// Save snapshot
	if saveErr := undo.Save(gitDir, snapshot); saveErr != nil {
		return "", saveErr
	}
	return stashRef, nil
}

// saveUndoSnapshotByName is like saveUndoSnapshot but takes branch names instead of tree nodes.
// Useful for sync where we don't always have tree nodes.
// Returns the stash ref (commit hash) if changes were stashed, empty string otherwise.
func saveUndoSnapshotByName(g *git.Git, cfg *config.Config, branchNames []string, deletedBranchNames []string, operation, command string, s *style.Style) (string, error) {
	gitDir := g.GetGitDir()

	// Get current branch for original head
	currentBranch, err := g.CurrentBranch()
	if err != nil {
		return "", err
	}

	// Create snapshot
	snapshot := undo.NewSnapshot(operation, command, currentBranch)

	// Auto-stash if dirty
	var stashRef string
	dirty, err := g.IsDirty()
	if err != nil {
		return "", err
	}
	if dirty {
		var stashErr error
		stashRef, stashErr = g.Stash("gh-stack auto-stash before " + operation)
		if stashErr != nil {
			return "", fmt.Errorf("failed to stash changes: %w", stashErr)
		}
		if stashRef != "" {
			snapshot.StashRef = stashRef
			fmt.Println(s.Muted("Auto-stashed uncommitted changes"))
		}
	}

	// Capture state of branches that will be modified
	for _, name := range branchNames {
		bs, captureErr := captureBranchState(g, cfg, name)
		if captureErr != nil {
			fmt.Printf("%s could not capture state for %s: %v\n", s.WarningIcon(), s.Branch(name), captureErr)
			continue
		}
		snapshot.Branches[name] = bs
	}

	// Capture state of branches that will be deleted
	for _, name := range deletedBranchNames {
		bs, captureErr := captureBranchState(g, cfg, name)
		if captureErr != nil {
			fmt.Printf("%s could not capture state for deleted branch %s: %v\n", s.WarningIcon(), s.Branch(name), captureErr)
			continue
		}
		snapshot.DeletedBranches[name] = bs
	}

	// Save snapshot
	if saveErr := undo.Save(gitDir, snapshot); saveErr != nil {
		return "", saveErr
	}
	return stashRef, nil
}

// captureBranchState captures the current state of a single branch.
func captureBranchState(g *git.Git, cfg *config.Config, branch string) (undo.BranchState, error) {
	bs := undo.BranchState{}

	sha, err := g.GetTip(branch)
	if err != nil {
		return bs, err
	}
	bs.SHA = sha

	// Capture config (errors are non-fatal - empty values are fine)
	bs.StackParent, _ = cfg.GetParent(branch)       //nolint:errcheck // empty is fine
	bs.StackPR, _ = cfg.GetPR(branch)               //nolint:errcheck // 0 is fine
	bs.StackForkPoint, _ = cfg.GetForkPoint(branch) //nolint:errcheck // empty is fine

	return bs, nil
}
