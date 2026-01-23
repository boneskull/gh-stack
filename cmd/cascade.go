// cmd/cascade.go
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

var cascadeCmd = &cobra.Command{
	Use:   "cascade",
	Short: "Rebase current branch and descendants onto their parents",
	Long:  `Rebase the current branch onto its parent, then recursively cascade to descendants.`,
	RunE:  runCascade,
}

var (
	cascadeOnlyFlag   bool
	cascadeDryRunFlag bool
)

func init() {
	cascadeCmd.Flags().BoolVar(&cascadeOnlyFlag, "only", false, "only cascade current branch, not descendants")
	cascadeCmd.Flags().BoolVar(&cascadeDryRunFlag, "dry-run", false, "show what would be done")
	rootCmd.AddCommand(cascadeCmd)
}

func runCascade(cmd *cobra.Command, args []string) error {
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

	// Check if cascade already in progress
	if state.Exists(g.GetGitDir()) {
		return fmt.Errorf("cascade already in progress; use 'gh stack continue' or 'gh stack abort'")
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

	// Collect branches to cascade
	var branches []*tree.Node
	branches = append(branches, node)
	if !cascadeOnlyFlag {
		branches = append(branches, tree.GetDescendants(node)...)
	}

	return doCascade(g, cfg, branches, cascadeDryRunFlag)
}

func doCascade(g *git.Git, cfg *config.Config, branches []*tree.Node, dryRun bool) error {
	originalBranch, _ := g.CurrentBranch()
	originalHead, _ := g.GetTip(originalBranch)

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
			fmt.Printf("Cascading %s... already up to date\n", b.Name)
			continue
		}

		if dryRun {
			fmt.Printf("Would rebase %s onto %s\n", b.Name, parent)
			continue
		}

		fmt.Printf("Cascading %s onto %s...\n", b.Name, parent)

		// Checkout and rebase
		if err := g.Checkout(b.Name); err != nil {
			return err
		}

		if err := g.Rebase(parent); err != nil {
			// Rebase conflict - save state
			remaining := make([]string, 0, len(branches)-i-1)
			for _, r := range branches[i+1:] {
				remaining = append(remaining, r.Name)
			}

			st := &state.CascadeState{
				Current:      b.Name,
				Pending:      remaining,
				OriginalHead: originalHead,
			}
			state.Save(g.GetGitDir(), st)

			fmt.Printf("\nCONFLICT: Resolve conflicts and run 'gh stack continue', or 'gh stack abort' to cancel.\n")
			fmt.Printf("Remaining branches: %v\n", remaining)
			return nil
		}

		fmt.Printf("Cascading %s... ok\n", b.Name)
	}

	// Return to original branch
	if !dryRun {
		g.Checkout(originalBranch)
	}

	return nil
}
