// cmd/cascade.go
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

// ErrConflict indicates a rebase conflict occurred during cascade.
var ErrConflict = errors.New("rebase conflict: resolve and run 'gh stack continue', or 'gh stack abort'")

var cascadeCmd = &cobra.Command{
	Use:     "restack",
	Aliases: []string{"cascade"},
	Short:   "Rebase current branch and descendants onto their parents",
	Long:    `Rebase the current branch onto its parent, then recursively restack descendants.`,
	RunE:    runCascade,
}

var (
	cascadeOnlyFlag      bool
	cascadeDryRunFlag    bool
	cascadeWorktreesFlag bool
)

func init() {
	cascadeCmd.Flags().BoolVar(&cascadeOnlyFlag, "only", false, "only restack current branch, not descendants")
	cascadeCmd.Flags().BoolVar(&cascadeDryRunFlag, "dry-run", false, "show what would be done")
	cascadeCmd.Flags().BoolVar(&cascadeWorktreesFlag, "worktrees", false, "rebase branches checked out in linked worktrees in-place")
	rootCmd.AddCommand(cascadeCmd)
}

func runCascade(cmd *cobra.Command, args []string) error {
	s := style.New()

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	g := git.New(cwd)

	cfg, err := config.New(g)
	if err != nil {
		return err
	}

	// Check if cascade already in progress
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

	// Collect branches to cascade
	var branches []*tree.Node
	branches = append(branches, node)
	if !cascadeOnlyFlag {
		branches = append(branches, tree.GetDescendants(node)...)
	}

	// Save undo snapshot (unless dry-run)
	var stashRef string
	if !cascadeDryRunFlag {
		var saveErr error
		stashRef, saveErr = saveUndoSnapshot(g, cfg, branches, nil, "cascade", "gh stack restack", s)
		if saveErr != nil {
			fmt.Printf("%s could not save undo state: %v\n", s.WarningIcon(), saveErr)
		}
	}

	// Build worktree map if --worktrees flag is set
	var worktrees map[string]string
	if cascadeWorktreesFlag {
		var wtErr error
		worktrees, wtErr = g.ListWorktrees()
		if wtErr != nil {
			return fmt.Errorf("failed to list worktrees: %w", wtErr)
		}
	}

	err = doCascadeWithState(g, cfg, branches, cascadeDryRunFlag, state.OperationCascade, false, false, false, nil, stashRef, worktrees, s)

	// Restore auto-stashed changes after operation (unless conflict, which saves stash in state)
	if stashRef != "" && !errors.Is(err, ErrConflict) {
		fmt.Println("Restoring auto-stashed changes...")
		if popErr := g.StashPop(stashRef); popErr != nil {
			fmt.Printf("%s could not restore stashed changes (commit %s): %v\n", s.WarningIcon(), git.AbbrevSHA(stashRef), popErr)
		}
	}

	return err
}

// doCascadeWithState performs cascade and saves state with the given operation type.
// allBranches is the complete list of branches for submit operations (used for push/PR after continue).
// stashRef is the commit hash of auto-stashed changes (if any), persisted to state on conflict.
// worktrees maps branch names to linked worktree paths. When non-nil, branches in
// the map are rebased directly in their worktree directory instead of being checked out.
func doCascadeWithState(g *git.Git, cfg *config.Config, branches []*tree.Node, dryRun bool, operation string, updateOnly, web, pushOnly bool, allBranches []string, stashRef string, worktrees map[string]string, s *style.Style) error {
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
			continue
		}

		if dryRun {
			fmt.Printf("%s Would rebase %s onto %s\n", s.Muted("dry-run:"), s.Branch(b.Name), s.Branch(parent))
			continue
		}

		// Check if we should use --onto rebase
		// This is needed when parent has been rebased/amended since child was created
		storedForkPoint, fpErr := cfg.GetForkPoint(b.Name)
		useOnto := false

		if fpErr == nil && g.CommitExists(storedForkPoint) {
			// We have a valid stored fork point
			// Use --onto if the stored fork point differs from merge-base
			currentMergeBase, mbErr := g.GetMergeBase(b.Name, parent)
			if mbErr == nil && currentMergeBase != storedForkPoint {
				useOnto = true
			}
		}

		// Determine if this branch lives in a linked worktree
		wtPath := ""
		if worktrees != nil {
			wtPath = worktrees[b.Name]
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

			st := &state.CascadeState{
				Current:      b.Name,
				Pending:      remaining,
				OriginalHead: originalHead,
				Operation:    operation,
				UpdateOnly:   updateOnly,
				Web:          web,
				PushOnly:     pushOnly,
				Branches:     allBranches,
				StashRef:     stashRef,
				Worktrees:    worktrees,
			}
			_ = state.Save(g.GetGitDir(), st) //nolint:errcheck // best effort - user can recover manually

			fmt.Printf("\n%s %s\n", s.FailureIcon(), s.Error("CONFLICT: Resolve conflicts and run 'gh stack continue', or 'gh stack abort' to cancel."))
			if wtPath != "" {
				fmt.Printf("Resolve conflicts in worktree: %s\n", wtPath)
			}
			fmt.Printf("Remaining branches: %v\n", remaining)
			if stashRef != "" {
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
	if !dryRun {
		_ = g.Checkout(originalBranch) //nolint:errcheck // best effort - cascade succeeded
	}

	return nil
}

// displayOperationName maps internal operation constants to user-facing names.
func displayOperationName(op string) string {
	switch op {
	case state.OperationCascade:
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
