// cmd/sync.go
package cmd

import (
	"fmt"
	"os"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/git"
	"github.com/boneskull/gh-stack/internal/github"
	"github.com/boneskull/gh-stack/internal/prompt"
	"github.com/boneskull/gh-stack/internal/tree"
	"github.com/spf13/cobra"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Fetch, detect merged PRs, retarget orphaned branches, cascade all",
	Long:  `Fetch from origin, detect merged PRs, retarget orphaned branches to trunk, and cascade all branches.`,
	RunE:  runSync,
}

var (
	syncNoCascadeFlag bool
	syncDryRunFlag    bool
)

func init() {
	syncCmd.Flags().BoolVar(&syncNoCascadeFlag, "no-cascade", false, "skip cascading branches")
	syncCmd.Flags().BoolVar(&syncDryRunFlag, "dry-run", false, "show what would be done")
	rootCmd.AddCommand(syncCmd)
}

// updateStackComments updates the navigation comment on all PRs in the stack.
func updateStackComments(cfg *config.Config, gh *github.Client) error {
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

	// Walk tree and update each PR's comment
	return walkTreeAndUpdateComments(root, root, trunk, gh, prInfo)
}

func walkTreeAndUpdateComments(node, root *tree.Node, trunk string, gh *github.Client, prInfo map[int]github.PRInfo) error {
	if node.PR > 0 {
		comment := github.GenerateStackComment(root, node.Name, trunk, gh.RepoURL(), prInfo)
		if comment != "" {
			if err := gh.CreateOrUpdateStackComment(node.PR, comment); err != nil {
				fmt.Printf("Warning: failed to update comment on PR #%d: %v\n", node.PR, err)
				// Continue with other PRs
			}
		}
	}

	for _, child := range node.Children {
		if err := walkTreeAndUpdateComments(child, root, trunk, gh, prInfo); err != nil {
			return err
		}
	}

	return nil
}

func runSync(cmd *cobra.Command, args []string) error {
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

	// Save undo snapshot of all tracked branches (unless dry-run)
	// This captures state before any modifications (fetch, delete, rebase)
	if !syncDryRunFlag {
		allBranches, listErr := cfg.ListTrackedBranches()
		if listErr == nil && len(allBranches) > 0 {
			if saveErr := saveUndoSnapshotByName(g, cfg, allBranches, nil, "sync", "gh stack sync"); saveErr != nil {
				fmt.Printf("Warning: could not save undo state: %v\n", saveErr)
			}
		}
	}

	// Fetch
	fmt.Println("Fetching from origin...")
	if !syncDryRunFlag {
		if fetchErr := g.Fetch(); fetchErr != nil {
			return fmt.Errorf("fetch failed: %w", fetchErr)
		}
	}

	// Fast-forward trunk
	currentBranch, _ := g.CurrentBranch() //nolint:errcheck // empty string is fine
	fmt.Printf("Fast-forwarding %s...\n", trunk)
	if !syncDryRunFlag {
		if ffErr := g.FastForward(trunk); ffErr != nil {
			fmt.Printf("Warning: could not fast-forward %s: %v\n", trunk, ffErr)
		}
		// Return to original branch
		_ = g.Checkout(currentBranch) //nolint:errcheck // best effort
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
			fmt.Printf("Warning: could not fetch PR #%d: %v\n", prNum, getPRErr)
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
		if sliceContains(merged, branch) {
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
		if sliceContains(merged, parent) {
			continue
		}

		// Check if parent exists on remote
		if !g.RemoteBranchExists(parent) {
			fmt.Printf("\nWarning: parent branch %q of %q does not exist on remote.\n", parent, branch)
			if prompt.IsInteractive() {
				retarget, _ := prompt.Confirm(fmt.Sprintf("Retarget %s to %s?", branch, trunk), true) //nolint:errcheck // default is fine
				if retarget {
					_ = cfg.SetParent(branch, trunk) //nolint:errcheck // best effort
					fmt.Printf("Retargeted %s to %s\n", branch, trunk)

					// Update PR base on GitHub if PR exists
					prNum, _ := cfg.GetPR(branch) //nolint:errcheck // 0 is fine
					if prNum > 0 {
						if updateErr := gh.UpdatePRBase(prNum, trunk); updateErr != nil {
							fmt.Printf("Warning: failed to update PR #%d base: %v\n", prNum, updateErr)
						} else {
							fmt.Printf("Updated PR #%d base to %s\n", prNum, trunk)
						}
					}
				}
			} else {
				fmt.Printf("Run 'git config branch.%s.stackParent %s' to fix.\n", branch, trunk)
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
			fmt.Printf("Would handle merged branch %s\n", branch)
		} else {
			action := handleMergedBranch(g, cfg, branch, trunk, &currentBranch)
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
					fmt.Printf("Warning: could not get fork point for %s: %v\n", child.Name, fpErr)
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
			fmt.Printf("Would retarget %s to %s (fork point: %s)\n", rt.childName, trunk, rt.forkPoint)
			continue
		}

		fmt.Printf("Retargeting %s to %s\n", rt.childName, trunk)
		_ = cfg.SetParent(rt.childName, trunk) //nolint:errcheck // best effort

		// Update PR base on GitHub
		if rt.childPR > 0 {
			if updateErr := gh.UpdatePRBase(rt.childPR, trunk); updateErr != nil {
				fmt.Printf("Warning: failed to update PR #%d base: %v\n", rt.childPR, updateErr)
			}

			// Check if this was a draft and now targets trunk - offer to publish
			pr, getPRErr := gh.GetPR(rt.childPR)
			if getPRErr == nil && pr.Draft {
				fmt.Printf("PR #%d (%s) now targets %s.\n", rt.childPR, rt.childName, trunk)
				ready, _ := prompt.Confirm("Mark as ready for review?", true) //nolint:errcheck // default is fine
				if ready {
					if readyErr := gh.MarkPRReady(rt.childPR); readyErr != nil {
						fmt.Printf("Warning: failed to mark PR ready: %v\n", readyErr)
					} else {
						fmt.Printf("PR #%d marked as ready for review.\n", rt.childPR)
					}
				}
			}
		}

		// Rebase using --onto if we have a fork point
		if rt.forkPoint != "" && g.CommitExists(rt.forkPoint) {
			displayForkPoint := rt.forkPoint
			if len(displayForkPoint) > 8 {
				displayForkPoint = displayForkPoint[:8]
			}
			fmt.Printf("Rebasing %s onto %s (from fork point %s)...\n", rt.childName, trunk, displayForkPoint)
			if rebaseErr := g.RebaseOnto(trunk, rt.forkPoint, rt.childName); rebaseErr != nil {
				fmt.Printf("Warning: --onto rebase failed, will try normal cascade: %v\n", rebaseErr)
				// Don't return error - let cascade try
			} else {
				fmt.Printf("Rebased %s successfully\n", rt.childName)

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
		fmt.Println("\nCascading all branches...")
		// Rebuild tree after modifications
		root, err = tree.Build(cfg)
		if err != nil {
			return err
		}

		// Cascade from trunk's children
		for _, child := range root.Children {
			allBranches := []*tree.Node{child}
			allBranches = append(allBranches, tree.GetDescendants(child)...)
			if err := doCascade(g, cfg, allBranches, syncDryRunFlag); err != nil {
				return err
			}
		}
	}

	// Update stack comments on all PRs
	if !syncDryRunFlag {
		fmt.Println("\nUpdating stack comments...")
		if err := updateStackComments(cfg, gh); err != nil {
			fmt.Printf("Warning: failed to update some comments: %v\n", err)
		}
	}

	fmt.Println("\nSync complete!")
	return nil
}

// sliceContains returns true if slice contains the given string.
func sliceContains(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
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
func handleMergedBranch(g *git.Git, cfg *config.Config, branch, trunk string, currentBranch *string) mergedAction {
	fmt.Printf("\nBranch %q appears to be merged into %s.\n", branch, trunk)

	// Default to delete in non-interactive mode
	if !prompt.IsInteractive() {
		return deleteMergedBranch(g, cfg, branch, trunk, currentBranch)
	}

	// Interactive mode: prompt for action
	choice, _ := prompt.Select("What would you like to do?", []string{ //nolint:errcheck // default is fine on error
		"Delete branch and remove from stack",
		"Orphan (keep branch, remove from stack)",
		"Skip (keep in stack, may cause conflicts)",
	}, 0)

	switch choice {
	case 0: // Delete
		return deleteMergedBranch(g, cfg, branch, trunk, currentBranch)
	case 1: // Orphan
		return orphanMergedBranch(cfg, branch)
	case 2: // Skip
		fmt.Printf("Skipping %s (keeping in stack)\n", branch)
		return mergedActionSkip
	default:
		return deleteMergedBranch(g, cfg, branch, trunk, currentBranch)
	}
}

// deleteMergedBranch deletes a merged branch and removes it from stack config.
// If the user is on the branch, it checks out trunk first.
func deleteMergedBranch(g *git.Git, cfg *config.Config, branch, trunk string, currentBranch *string) mergedAction {
	// If user is on the merged branch, checkout trunk first
	if *currentBranch == branch {
		fmt.Printf("Checking out %s (currently on merged branch)...\n", trunk)
		if err := g.Checkout(trunk); err != nil {
			fmt.Printf("Warning: could not checkout %s: %v\n", trunk, err)
			fmt.Printf("Falling back to orphan instead of delete.\n")
			return orphanMergedBranch(cfg, branch)
		}
		*currentBranch = trunk
	}

	fmt.Printf("Deleting merged branch %s\n", branch)
	_ = cfg.RemoveParent(branch)    //nolint:errcheck // best effort cleanup
	_ = cfg.RemovePR(branch)        //nolint:errcheck // best effort cleanup
	_ = cfg.RemoveForkPoint(branch) //nolint:errcheck // best effort cleanup
	if err := g.DeleteBranch(branch); err != nil {
		fmt.Printf("Warning: could not delete branch %s: %v\n", branch, err)
	}
	return mergedActionDelete
}

// orphanMergedBranch removes a branch from stack config but keeps the git branch.
func orphanMergedBranch(cfg *config.Config, branch string) mergedAction {
	fmt.Printf("Orphaning %s (branch preserved, removed from stack)\n", branch)
	_ = cfg.RemoveParent(branch)    //nolint:errcheck // best effort cleanup
	_ = cfg.RemovePR(branch)        //nolint:errcheck // best effort cleanup
	_ = cfg.RemoveForkPoint(branch) //nolint:errcheck // best effort cleanup
	return mergedActionOrphan
}
