// cmd/submit.go
package cmd

import (
	"fmt"
	"os"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/git"
	"github.com/boneskull/gh-stack/internal/github"
	"github.com/boneskull/gh-stack/internal/state"
	"github.com/boneskull/gh-stack/internal/tree"
	"github.com/spf13/cobra"
)

var submitCmd = &cobra.Command{
	Use:   "submit",
	Short: "Cascade, push, and create/update PRs for current branch and descendants",
	Long: `Submit rebases the current branch and its descendants onto their parents,
pushes all affected branches, and creates or updates pull requests.

This is the typical workflow command after making changes in a stack:
1. Cascade: rebase current branch + descendants onto their parents
2. Push: force-push all affected branches (with --force-with-lease)
3. PR: create PRs for branches without them, update PR bases for those that have them

If a rebase conflict occurs, resolve it and run 'gh stack continue'.`,
	RunE: runSubmit,
}

var (
	submitDryRunFlag      bool
	submitCurrentOnlyFlag bool
	submitUpdateOnlyFlag  bool
)

func init() {
	submitCmd.Flags().BoolVar(&submitDryRunFlag, "dry-run", false, "show what would be done without doing it")
	submitCmd.Flags().BoolVar(&submitCurrentOnlyFlag, "current-only", false, "only submit current branch, not descendants")
	submitCmd.Flags().BoolVar(&submitUpdateOnlyFlag, "update-only", false, "only update existing PRs, don't create new ones")
	rootCmd.AddCommand(submitCmd)
}

func runSubmit(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	cfg, err := config.Load(cwd)
	if err != nil {
		return err
	}

	g := git.New(cwd)

	// Check for dirty working tree
	dirty, err := g.IsDirty()
	if err != nil {
		return err
	}
	if dirty {
		return fmt.Errorf("working tree has uncommitted changes; commit or stash first")
	}

	// Check if operation already in progress
	if state.Exists(g.GetGitDir()) {
		return fmt.Errorf("operation already in progress; use 'gh stack continue' or 'gh stack abort'")
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
		return fmt.Errorf("branch %q is not tracked", currentBranch)
	}

	// Collect branches to submit (current + descendants)
	var branches []*tree.Node
	branches = append(branches, node)
	if !submitCurrentOnlyFlag {
		branches = append(branches, tree.GetDescendants(node)...)
	}

	// Phase 1: Cascade
	fmt.Println("=== Phase 1: Cascade ===")
	if err := doCascadeWithState(g, cfg, branches, submitDryRunFlag, state.OperationSubmit, submitUpdateOnlyFlag); err != nil {
		return err // Conflict or error - state saved, user can continue
	}

	// Phases 2 & 3
	return doSubmitPushAndPR(g, cfg, root, branches, submitDryRunFlag, submitUpdateOnlyFlag)
}

// doSubmitPushAndPR handles push and PR creation/update phases.
// This is called after cascade succeeds (or from continue after conflict resolution).
func doSubmitPushAndPR(g *git.Git, cfg *config.Config, root *tree.Node, branches []*tree.Node, dryRun, updateOnly bool) error {
	// Phase 2: Push all branches
	fmt.Println("\n=== Phase 2: Push ===")
	for _, b := range branches {
		if dryRun {
			fmt.Printf("Would push %s -> origin/%s (forced)\n", b.Name, b.Name)
		} else {
			fmt.Printf("Pushing %s -> origin/%s (forced)... ", b.Name, b.Name)
			if err := g.Push(b.Name, true); err != nil {
				fmt.Println("failed")
				return fmt.Errorf("failed to push %s: %w", b.Name, err)
			}
			fmt.Println("ok")
		}
	}

	// Phase 3: Create/update PRs
	return doSubmitPRs(cfg, root, branches, dryRun, updateOnly)
}

// doSubmitPRs handles PR creation/update for all branches.
func doSubmitPRs(cfg *config.Config, root *tree.Node, branches []*tree.Node, dryRun, updateOnly bool) error {
	fmt.Println("\n=== Phase 3: PRs ===")

	trunk, err := cfg.GetTrunk()
	if err != nil {
		return err
	}

	// In dry-run mode, we don't need a GitHub client
	var ghClient *github.Client
	if !dryRun {
		var clientErr error
		ghClient, clientErr = github.NewClient()
		if clientErr != nil {
			return clientErr
		}
	}

	for _, b := range branches {
		parent, _ := cfg.GetParent(b.Name) //nolint:errcheck // empty is fine
		if parent == "" {
			parent = trunk
		}

		existingPR, _ := cfg.GetPR(b.Name) //nolint:errcheck // 0 is fine

		if existingPR > 0 {
			// Update existing PR
			if dryRun {
				fmt.Printf("Would update PR #%d base to %q\n", existingPR, parent)
			} else {
				fmt.Printf("Updating PR #%d for %s (base: %s)... ", existingPR, b.Name, parent)
				if err := ghClient.UpdatePRBase(existingPR, parent); err != nil {
					fmt.Println("failed")
					fmt.Printf("Warning: failed to update PR #%d base: %v\n", existingPR, err)
				} else {
					fmt.Println("ok")
				}
				// Update stack comment
				if err := ghClient.GenerateAndPostStackComment(root, b.Name, trunk, existingPR); err != nil {
					fmt.Printf("Warning: failed to update stack comment for PR #%d: %v\n", existingPR, err)
				}
			}
		} else if !updateOnly {
			// Create new PR
			if dryRun {
				fmt.Printf("Would create PR for %s (base: %s)\n", b.Name, parent)
			} else {
				prNum, err := createPRForBranch(ghClient, cfg, root, b.Name, parent, trunk)
				if err != nil {
					fmt.Printf("Warning: failed to create PR for %s: %v\n", b.Name, err)
				} else {
					fmt.Printf("Created PR #%d for %s (%s)\n", prNum, b.Name, ghClient.PRURL(prNum))
				}
			}
		} else {
			fmt.Printf("Skipping %s (no existing PR, --update-only)\n", b.Name)
		}
	}

	return nil
}

// createPRForBranch creates a PR for the given branch and stores the PR number.
func createPRForBranch(ghClient *github.Client, cfg *config.Config, root *tree.Node, branch, base, trunk string) (int, error) {
	// Determine if draft (not targeting trunk = middle of stack)
	draft := base != trunk

	pr, err := ghClient.CreateSubmitPR(branch, base, draft)
	if err != nil {
		return 0, err
	}

	// Store PR number in config
	if err := cfg.SetPR(branch, pr.Number); err != nil {
		return pr.Number, fmt.Errorf("PR created but failed to store number: %w", err)
	}

	// Update the tree node's PR number so stack comments render correctly
	if node := tree.FindNode(root, branch); node != nil {
		node.PR = pr.Number
	}

	// Add stack navigation comment
	if err := ghClient.GenerateAndPostStackComment(root, branch, trunk, pr.Number); err != nil {
		fmt.Printf("Warning: failed to add stack comment to PR #%d: %v\n", pr.Number, err)
	}

	return pr.Number, nil
}
