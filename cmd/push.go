// cmd/push.go
package cmd

import (
	"fmt"
	"os"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/git"
	"github.com/boneskull/gh-stack/internal/github"
	"github.com/boneskull/gh-stack/internal/tree"
	"github.com/spf13/cobra"
)

var pushCmd = &cobra.Command{
	Use:   "push",
	Short: "Force-push branches from trunk to current branch",
	Long:  `Force-push all branches in the stack from trunk to current branch, updating PR base branches as needed.`,
	RunE:  runPush,
}

var pushDryRunFlag bool

func init() {
	pushCmd.Flags().BoolVar(&pushDryRunFlag, "dry-run", false, "show what would be pushed without pushing")
	rootCmd.AddCommand(pushCmd)
}

func runPush(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	cfg, err := config.Load(cwd)
	if err != nil {
		return err
	}

	g := git.New(cwd)

	currentBranch, err := g.CurrentBranch()
	if err != nil {
		return err
	}

	// Build tree
	root, err := tree.Build(cfg)
	if err != nil {
		return err
	}

	// Find current branch in tree
	node := tree.FindNode(root, currentBranch)
	if node == nil {
		return fmt.Errorf("branch %q is not tracked", currentBranch)
	}

	// Get downstack (ancestors from current to trunk, reversed)
	ancestors := tree.GetAncestors(node)
	trunk, err := cfg.GetTrunk()
	if err != nil {
		return err
	}

	// Build list: current + ancestors (excluding trunk)
	var branches []*tree.Node
	branches = append(branches, node)
	for _, a := range ancestors {
		if a.Name != trunk {
			branches = append(branches, a)
		}
	}

	// Reverse to go from trunk-adjacent to current
	for i, j := 0, len(branches)-1; i < j; i, j = i+1, j-1 {
		branches[i], branches[j] = branches[j], branches[i]
	}

	// Validate all branches are properly rebased onto their parents
	for _, b := range branches {
		parent, _ := cfg.GetParent(b.Name) //nolint:errcheck // empty string is fine
		if parent == "" {
			continue
		}

		needsRebase, err := g.NeedsRebase(b.Name, parent)
		if err != nil {
			return fmt.Errorf("failed to check rebase status for %s: %w", b.Name, err)
		}
		if needsRebase {
			return fmt.Errorf("branch %q is not rebased onto %q; run 'gh stack cascade' first", b.Name, parent)
		}
	}

	// Update PR bases and push
	for _, b := range branches {
		parent, _ := cfg.GetParent(b.Name) //nolint:errcheck // empty string is fine

		// Update PR base if needed
		if b.PR > 0 {
			if pushDryRunFlag {
				fmt.Printf("Would update PR #%d base to %q\n", b.PR, parent)
			} else {
				fmt.Printf("Updating PR #%d base to %q\n", b.PR, parent)
				if err := github.UpdatePRBase(b.PR, parent); err != nil {
					fmt.Printf("Warning: failed to update PR base: %v\n", err)
				}
			}
		}

		// Push
		if pushDryRunFlag {
			fmt.Printf("Would push %s -> origin/%s (forced)\n", b.Name, b.Name)
		} else {
			fmt.Printf("Pushing %s -> origin/%s (forced)\n", b.Name, b.Name)
			if err := g.Push(b.Name, true); err != nil {
				return fmt.Errorf("failed to push %s: %w", b.Name, err)
			}
		}
	}

	return nil
}
