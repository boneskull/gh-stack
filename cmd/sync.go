// cmd/sync.go
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

func runSync(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	cfg, err := config.Load(cwd)
	if err != nil {
		return err
	}

	g := git.New(cwd)

	trunk, err := cfg.GetTrunk()
	if err != nil {
		return err
	}

	// Fetch
	fmt.Println("Fetching from origin...")
	if !syncDryRunFlag {
		if err := g.Fetch(); err != nil {
			return fmt.Errorf("fetch failed: %w", err)
		}
	}

	// Fast-forward trunk
	currentBranch, _ := g.CurrentBranch()
	fmt.Printf("Fast-forwarding %s...\n", trunk)
	if !syncDryRunFlag {
		if err := g.FastForward(trunk); err != nil {
			fmt.Printf("Warning: could not fast-forward %s: %v\n", trunk, err)
		}
		// Return to original branch
		g.Checkout(currentBranch)
	}

	// Check for merged PRs
	branches, err := cfg.ListTrackedBranches()
	if err != nil {
		return err
	}

	var merged []string
	for _, branch := range branches {
		prNum, err := cfg.GetPR(branch)
		if err != nil || prNum == 0 {
			continue
		}

		pr, err := github.GetPR(prNum)
		if err != nil {
			fmt.Printf("Warning: could not fetch PR #%d: %v\n", prNum, err)
			continue
		}

		if pr.Merged {
			merged = append(merged, branch)
		}
	}

	// Handle merged branches
	root, _ := tree.Build(cfg)
	for _, branch := range merged {
		node := tree.FindNode(root, branch)
		if node == nil {
			continue
		}

		// Retarget children to trunk
		for _, child := range node.Children {
			if syncDryRunFlag {
				fmt.Printf("Would retarget %s from %s to %s\n", child.Name, branch, trunk)
			} else {
				fmt.Printf("Retargeting %s from %s to %s\n", child.Name, branch, trunk)
				cfg.SetParent(child.Name, trunk)
			}
		}

		// Prompt to delete merged branch
		if syncDryRunFlag {
			fmt.Printf("Would delete merged branch %s\n", branch)
		} else {
			fmt.Printf("Deleting merged branch %s (PR was merged)\n", branch)
			cfg.RemoveParent(branch)
			cfg.RemovePR(branch)
			g.DeleteBranch(branch)
		}
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

	fmt.Println("\nSync complete!")
	return nil
}
