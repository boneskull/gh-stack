// cmd/pr.go
package cmd

import (
	"fmt"
	"os"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/git"
	"github.com/boneskull/gh-stack/internal/github"
	"github.com/spf13/cobra"
)

var prCmd = &cobra.Command{
	Use:   "pr",
	Short: "Create or update a PR for the current branch",
	Long:  `Create a new PR targeting the parent branch, or update an existing PR's base.`,
	RunE:  runPR,
}

var prBaseFlag string

func init() {
	prCmd.Flags().StringVar(&prBaseFlag, "base", "", "override base branch")
	rootCmd.AddCommand(prCmd)
}

func runPR(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	cfg, err := config.Load(cwd)
	if err != nil {
		return err
	}

	g := git.New(cwd)
	branch, err := g.CurrentBranch()
	if err != nil {
		return err
	}

	// Get parent (base branch)
	parent, err := cfg.GetParent(branch)
	if err != nil {
		return fmt.Errorf("branch %q is not tracked", branch)
	}

	base := prBaseFlag
	if base == "" {
		base = parent
	}

	// Check if PR already exists
	existingPR, _ := cfg.GetPR(branch)
	if existingPR > 0 {
		// Update existing PR's base if needed
		fmt.Printf("PR #%d already exists, updating base to %q\n", existingPR, base)
		if err := github.UpdatePRBase(existingPR, base); err != nil {
			return fmt.Errorf("failed to update PR base: %w", err)
		}
		return nil
	}

	// Create new PR
	fmt.Printf("Creating PR for %q targeting %q...\n", branch, base)
	prNumber, err := github.CreatePR(base, branch, "")
	if err != nil {
		return err
	}

	// Store PR number
	if err := cfg.SetPR(branch, prNumber); err != nil {
		return err
	}

	fmt.Printf("Created PR #%d\n", prNumber)
	return nil
}
