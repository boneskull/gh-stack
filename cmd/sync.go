// cmd/sync.go
package cmd

import (
	"errors"
	"fmt"
	"os"
	"slices"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/git"
	"github.com/boneskull/gh-stack/internal/github"
	"github.com/boneskull/gh-stack/internal/prompt"
	"github.com/boneskull/gh-stack/internal/state"
	"github.com/boneskull/gh-stack/internal/style"
	"github.com/boneskull/gh-stack/internal/tree"
	"github.com/spf13/cobra"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Fetch, detect merged PRs, retarget orphaned branches, restack all",
	Long:  `Fetch from origin, detect merged PRs, retarget orphaned branches to trunk, and restack all branches.`,
	RunE:  runSync,
}

var (
	syncNoCascadeFlag bool
	syncDryRunFlag    bool
	syncWorktreesFlag bool
	syncNoDetectFlag  bool
)

func init() {
	syncCmd.Flags().BoolVar(&syncNoCascadeFlag, "no-restack", false, "skip restacking branches")
	syncCmd.Flags().BoolVar(&syncDryRunFlag, "dry-run", false, "show what would be done")
	syncCmd.Flags().BoolVar(&syncWorktreesFlag, "worktrees", false, "rebase branches checked out in linked worktrees in-place")
	syncCmd.Flags().BoolVar(&syncNoDetectFlag, "no-detect", false, "skip auto-detection of untracked branches")
	rootCmd.AddCommand(syncCmd)
}

// updateStackComments updates the navigation comment on all PRs in the stack.
func updateStackComments(cfg *config.Config, g *git.Git, gh *github.Client, s *style.Style) error {
	trunk, err := cfg.GetTrunk()
	if err != nil {
		return err
	}

	root, err := tree.Build(cfg)
	if err != nil {
		return err
	}

	// Fetch all PR titles in a single request
	prNumbers := github.CollectPRNumbers(root)
	prInfo, err := gh.GetPRTitles(prNumbers)
	if err != nil {
		// Non-fatal: we can still render without titles
		prInfo = make(map[int]github.PRInfo)
	}

	// Build remote branches set for filtering local-only branches
	remoteBranches, rbErr := g.ListRemoteBranches()
	if rbErr != nil {
		// Non-fatal: render without filtering
		fmt.Printf("%s could not list remote branches: %v\n", s.WarningIcon(), rbErr)
	}

	// Walk tree and update each PR's comment
	return walkTreeAndUpdateComments(root, root, trunk, gh, prInfo, remoteBranches, s)
}

func walkTreeAndUpdateComments(node, root *tree.Node, trunk string, gh *github.Client, prInfo map[int]github.PRInfo, remoteBranches map[string]bool, s *style.Style) error {
	if node.PR > 0 {
		comment := github.GenerateStackComment(root, node.Name, trunk, gh.RepoURL(), prInfo, remoteBranches)
		if comment != "" {
			if err := gh.CreateOrUpdateStackComment(node.PR, comment); err != nil {
				fmt.Printf("%s failed to update comment on PR #%d: %v\n", s.WarningIcon(), node.PR, err)
				// Continue with other PRs
			}
		}
	}

	for _, child := range node.Children {
		if err := walkTreeAndUpdateComments(child, root, trunk, gh, prInfo, remoteBranches, s); err != nil {
			return err
		}
	}

	return nil
}

func runSync(cmd *cobra.Command, args []string) error {
	s := style.New()

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	cfg, err := config.Load(cwd)
	if err != nil {
		return err
	}

	gh, err := github.NewClient()
	if err != nil {
		return err
	}

	g := git.New(cwd)

	trunk, err := cfg.GetTrunk()
	if err != nil {
		return err
	}

	// Capture the starting branch to return to after sync completes
	// Note: CurrentBranch() returns "HEAD" when in detached HEAD state
	startingBranch, _ := g.CurrentBranch() //nolint:errcheck // empty string is fine
	if startingBranch == "HEAD" {
		startingBranch = "" // Treat detached HEAD as "no starting branch"
	}

	// Save undo snapshot of all tracked branches (unless dry-run)
	// This captures state before any modifications (fetch, delete, rebase)
	var stashRef string
	if !syncDryRunFlag {
		allBranches, listErr := cfg.ListTrackedBranches()
		if listErr == nil && len(allBranches) > 0 {
			var saveErr error
			stashRef, saveErr = saveUndoSnapshotByName(g, cfg, allBranches, nil, "sync", "gh stack sync", s)
			if saveErr != nil {
				fmt.Printf("%s could not save undo state: %v\n", s.WarningIcon(), saveErr)
			}
		}
	}

	// Track if we hit a conflict (stash is saved in state for conflicts)
	var hitConflict bool

	// Ensure stash is restored on any exit (success or error, except conflicts)
	// This defer must be after stashRef is set
	defer func() {
		if stashRef != "" && !hitConflict {
			fmt.Println("Restoring auto-stashed changes...")
			if popErr := g.StashPop(stashRef); popErr != nil {
				fmt.Printf("%s could not restore stashed changes (commit %s): %v\n", s.WarningIcon(), git.AbbrevSHA(stashRef), popErr)
			}
		}
	}()

	// Fetch
	fmt.Println("Fetching from origin...")
	if !syncDryRunFlag {
		if fetchErr := g.Fetch(); fetchErr != nil {
			return fmt.Errorf("fetch failed: %w", fetchErr)
		}
	}

	// Fast-forward trunk
	currentBranch, _ := g.CurrentBranch() //nolint:errcheck // empty string is fine
	fmt.Printf("Fast-forwarding %s...\n", s.Branch(trunk))
	if !syncDryRunFlag {
		if ffErr := g.FastForward(trunk); ffErr != nil {
			fmt.Printf("%s could not fast-forward %s: %v\n", s.WarningIcon(), s.Branch(trunk), ffErr)
		}
		// Return to original branch
		_ = g.Checkout(currentBranch) //nolint:errcheck // best effort
	}

	// Auto-detect and adopt untracked branches
	if !syncNoDetectFlag {
		if adoptErr := autoDetectAndAdopt(cfg, g, gh, s); adoptErr != nil {
			fmt.Printf("%s auto-detection: %v\n", s.WarningIcon(), adoptErr)
		}
	}

	// Check for merged PRs
	branches, err := cfg.ListTrackedBranches()
	if err != nil {
		return err
	}

	var merged []string
	for _, branch := range branches {
		prNum, prErr := cfg.GetPR(branch)
		if prErr != nil || prNum == 0 {
			continue
		}

		pr, getPRErr := gh.GetPR(prNum)
		if getPRErr != nil {
			fmt.Printf("%s could not fetch PR #%d: %v\n", s.WarningIcon(), prNum, getPRErr)
			continue
		}

		if pr.Merged {
			merged = append(merged, branch)
		}
	}

	// Content-based detection for squash merges (fallback when PR detection fails)
	// Uses git diff to detect when a branch's content is already in trunk
	for _, branch := range branches {
		// Skip already detected via PR
		if slices.Contains(merged, branch) {
			continue
		}

		// Check if branch content is identical to trunk (squash merge detection)
		isContentMerged, diffErr := g.IsContentMerged(branch, trunk)
		if diffErr != nil {
			// Can't determine, let cascade try
			continue
		}

		if isContentMerged {
			// Tree content is identical - branch was squash-merged
			merged = append(merged, branch)
		}
	}

	// Check for branches whose parent doesn't exist on the remote
	// This can happen if a parent branch was deleted without merging, or never pushed
	for _, branch := range branches {
		parent, parentErr := cfg.GetParent(branch)
		if parentErr != nil {
			continue
		}

		// Skip if parent is trunk (trunk should always exist on remote)
		if parent == trunk {
			continue
		}

		// Skip if parent is already marked as merged (will be handled)
		if slices.Contains(merged, parent) {
			continue
		}

		// Check if parent exists on remote
		if !g.RemoteBranchExists(parent) {
			fmt.Printf("\n%s parent branch %s of %s does not exist on remote.\n", s.WarningIcon(), s.Branch(parent), s.Branch(branch))
			if prompt.IsInteractive() {
				retarget, _ := prompt.Confirm(fmt.Sprintf("Retarget %s to %s?", branch, trunk), true) //nolint:errcheck // default is fine
				if retarget {
					_ = cfg.SetParent(branch, trunk) //nolint:errcheck // best effort
					fmt.Printf("%s Retargeted %s to %s\n", s.SuccessIcon(), s.Branch(branch), s.Branch(trunk))

					// Update PR base on GitHub if PR exists
					prNum, _ := cfg.GetPR(branch) //nolint:errcheck // 0 is fine
					if prNum > 0 {
						if updateErr := gh.UpdatePRBase(prNum, trunk); updateErr != nil {
							fmt.Printf("%s failed to update PR #%d base: %v\n", s.WarningIcon(), prNum, updateErr)
						} else {
							fmt.Printf("%s Updated PR #%d base to %s\n", s.SuccessIcon(), prNum, s.Branch(trunk))
						}
					}
				}
			} else {
				fmt.Println(s.Muted(fmt.Sprintf("Run 'git config branch.%s.stackParent %s' to fix.", branch, trunk)))
			}
		}
	}

	// Handle merged branches
	root, _ := tree.Build(cfg) //nolint:errcheck // nil root is fine, FindNode handles it

	// Collect fork points BEFORE deleting merged branches
	type retargetInfo struct {
		childName string
		forkPoint string
		childPR   int
	}
	var retargets []retargetInfo

	for _, branch := range merged {
		node := tree.FindNode(root, branch)
		if node == nil {
			continue
		}

		// Handle merged branch with interactive prompt
		if syncDryRunFlag {
			fmt.Printf("%s Would handle merged branch %s\n", s.Muted("dry-run:"), s.Branch(branch))
		} else {
			action := handleMergedBranch(g, cfg, branch, trunk, &currentBranch, s)
			if action == mergedActionSkip {
				// User chose to skip - don't collect fork points or retarget children
				continue
			}
		}

		// For each child, get fork point - prefer stored, fall back to calculated
		for _, child := range node.Children {
			// Try stored fork point first
			forkPoint, fpErr := cfg.GetForkPoint(child.Name)
			if fpErr != nil || !g.CommitExists(forkPoint) {
				// Fall back to calculating from parent (before it's deleted)
				forkPoint, fpErr = g.GetMergeBase(child.Name, branch)
				if fpErr != nil {
					fmt.Printf("%s could not get fork point for %s: %v\n", s.WarningIcon(), s.Branch(child.Name), fpErr)
					forkPoint = "" // Will fall back to simple rebase
				}
			}
			childPR, _ := cfg.GetPR(child.Name) //nolint:errcheck // 0 is fine
			retargets = append(retargets, retargetInfo{
				childName: child.Name,
				forkPoint: forkPoint,
				childPR:   childPR,
			})
		}
	}

	// Retarget children to trunk
	for _, rt := range retargets {
		if syncDryRunFlag {
			fmt.Printf("%s Would retarget %s to %s (fork point: %s)\n", s.Muted("dry-run:"), s.Branch(rt.childName), s.Branch(trunk), rt.forkPoint)
			continue
		}

		fmt.Printf("Retargeting %s to %s\n", s.Branch(rt.childName), s.Branch(trunk))
		_ = cfg.SetParent(rt.childName, trunk) //nolint:errcheck // best effort

		// Update PR base on GitHub
		if rt.childPR > 0 {
			if updateErr := gh.UpdatePRBase(rt.childPR, trunk); updateErr != nil {
				fmt.Printf("%s failed to update PR #%d base: %v\n", s.WarningIcon(), rt.childPR, updateErr)
			}
		}

		// Rebase using --onto if we have a fork point
		if rt.forkPoint != "" && g.CommitExists(rt.forkPoint) {
			displayForkPoint := rt.forkPoint
			if len(displayForkPoint) > 8 {
				displayForkPoint = displayForkPoint[:8]
			}
			fmt.Printf("Rebasing %s onto %s (from fork point %s)...\n", s.Branch(rt.childName), s.Branch(trunk), displayForkPoint)
			if rebaseErr := g.RebaseOnto(trunk, rt.forkPoint, rt.childName); rebaseErr != nil {
				fmt.Printf("%s --onto rebase failed, will try normal restack: %v\n", s.WarningIcon(), rebaseErr)
				// Don't return error - let cascade try
			} else {
				fmt.Printf("%s Rebased %s successfully\n", s.SuccessIcon(), s.Branch(rt.childName))

				// Update fork point to new parent tip after successful rebase
				trunkTip, tipErr := g.GetTip(trunk)
				if tipErr == nil {
					_ = cfg.SetForkPoint(rt.childName, trunkTip) //nolint:errcheck // best effort
				}
			}
		}
	}

	// Return to original branch after retargeting
	if !syncDryRunFlag && currentBranch != "" {
		_ = g.Checkout(currentBranch) //nolint:errcheck // best effort
	}

	// Cascade all (if not disabled)
	if !syncNoCascadeFlag {
		fmt.Println(s.Bold("\nRestacking all branches..."))
		// Rebuild tree after modifications
		root, err = tree.Build(cfg)
		if err != nil {
			return err
		}

		// Build worktree map if --worktrees flag is set
		var worktrees map[string]string
		if syncWorktreesFlag {
			var wtErr error
			worktrees, wtErr = g.ListWorktrees()
			if wtErr != nil {
				return fmt.Errorf("failed to list worktrees: %w", wtErr)
			}
		}

		// Cascade from trunk's children
		for _, child := range root.Children {
			allBranches := []*tree.Node{child}
			allBranches = append(allBranches, tree.GetDescendants(child)...)
			if err := doCascadeWithState(g, cfg, allBranches, syncDryRunFlag, state.OperationCascade, false, false, false, nil, stashRef, worktrees, s); err != nil {
				if errors.Is(err, ErrConflict) {
					hitConflict = true
				}
				return err
			}
		}
	}

	// Update stack comments on all PRs
	if !syncDryRunFlag {
		fmt.Println("\nUpdating stack comments...")
		if err := updateStackComments(cfg, g, gh, s); err != nil {
			fmt.Printf("%s failed to update some comments: %v\n", s.WarningIcon(), err)
		}
	}

	// Return to the starting branch
	if !syncDryRunFlag && startingBranch != "" {
		// Only switch if we're not already there
		currentBranch, _ := g.CurrentBranch() //nolint:errcheck // empty string is fine
		if currentBranch != startingBranch {
			// Check if the starting branch still exists (it may have been deleted during sync)
			if g.BranchExists(startingBranch) {
				if checkoutErr := g.Checkout(startingBranch); checkoutErr != nil {
					fmt.Printf("%s could not return to starting branch %s: %v\n", s.WarningIcon(), s.Branch(startingBranch), checkoutErr)
				}
			} else {
				fmt.Printf("%s starting ref %s is not a local branch or no longer exists, staying on %s\n", s.WarningIcon(), s.Branch(startingBranch), s.Branch(currentBranch))
			}
		}
	}

	fmt.Println()
	fmt.Println(s.SuccessMessage("Sync complete!"))
	// Stash restoration handled by defer
	return nil
}

// mergedAction represents the user's choice for handling a merged branch.
type mergedAction int

const (
	mergedActionDelete mergedAction = iota
	mergedActionOrphan
	mergedActionSkip
)

// handleMergedBranch prompts the user for how to handle a merged branch and executes the choice.
// Returns the action taken. If the user is on the merged branch, it will checkout trunk first.
// The currentBranch pointer is updated if a checkout occurs.
func handleMergedBranch(g *git.Git, cfg *config.Config, branch, trunk string, currentBranch *string, s *style.Style) mergedAction {
	fmt.Printf("\nBranch %s appears to be %s into %s.\n", s.Branch(branch), s.Merged("merged"), s.Branch(trunk))

	// Default to delete in non-interactive mode
	if !prompt.IsInteractive() {
		return deleteMergedBranch(g, cfg, branch, trunk, currentBranch, s)
	}

	// Interactive mode: prompt for action
	choice, _ := prompt.Select("What would you like to do?", []string{ //nolint:errcheck // default is fine on error
		"Delete branch and remove from stack",
		"Orphan (keep branch, remove from stack)",
		"Skip (keep in stack, may cause conflicts)",
	}, 0)

	switch choice {
	case 0: // Delete
		return deleteMergedBranch(g, cfg, branch, trunk, currentBranch, s)
	case 1: // Orphan
		return orphanMergedBranch(cfg, branch, s)
	case 2: // Skip
		fmt.Printf("Skipping %s (keeping in stack)\n", s.Branch(branch))
		return mergedActionSkip
	default:
		return deleteMergedBranch(g, cfg, branch, trunk, currentBranch, s)
	}
}

// deleteMergedBranch deletes a merged branch and removes it from stack config.
// If the user is on the branch, it checks out trunk first.
func deleteMergedBranch(g *git.Git, cfg *config.Config, branch, trunk string, currentBranch *string, s *style.Style) mergedAction {
	// If user is on the merged branch, checkout trunk first
	if *currentBranch == branch {
		fmt.Printf("Checking out %s (currently on merged branch)...\n", s.Branch(trunk))
		if err := g.Checkout(trunk); err != nil {
			fmt.Printf("%s could not checkout %s: %v\n", s.WarningIcon(), s.Branch(trunk), err)
			fmt.Println(s.Muted("Falling back to orphan instead of delete."))
			return orphanMergedBranch(cfg, branch, s)
		}
		*currentBranch = trunk
	}

	fmt.Printf("Deleting merged branch %s\n", s.Branch(branch))
	_ = cfg.RemoveParent(branch)    //nolint:errcheck // best effort cleanup
	_ = cfg.RemovePR(branch)        //nolint:errcheck // best effort cleanup
	_ = cfg.RemoveForkPoint(branch) //nolint:errcheck // best effort cleanup
	if err := g.DeleteBranch(branch); err != nil {
		fmt.Printf("%s could not delete branch %s: %v\n", s.WarningIcon(), s.Branch(branch), err)
	}
	return mergedActionDelete
}

// orphanMergedBranch removes a branch from stack config but keeps the git branch.
func orphanMergedBranch(cfg *config.Config, branch string, s *style.Style) mergedAction {
	fmt.Printf("Orphaning %s (branch preserved, removed from stack)\n", s.Branch(branch))
	_ = cfg.RemoveParent(branch)    //nolint:errcheck // best effort cleanup
	_ = cfg.RemovePR(branch)        //nolint:errcheck // best effort cleanup
	_ = cfg.RemoveForkPoint(branch) //nolint:errcheck // best effort cleanup
	return mergedActionOrphan
}
