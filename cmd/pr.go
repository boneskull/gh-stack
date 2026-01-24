// cmd/pr.go
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

	// Create GitHub client
	gh, err := github.NewClient()
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

	// Get trunk for draft decision and comment generation
	trunk, err := cfg.GetTrunk()
	if err != nil {
		return err
	}

	// Check if PR already exists
	existingPR, _ := cfg.GetPR(branch)
	if existingPR > 0 {
		// Update existing PR's base if needed
		fmt.Printf("PR #%d already exists, updating base to %q\n", existingPR, base)
		if err := gh.UpdatePRBase(existingPR, base); err != nil {
			return fmt.Errorf("failed to update PR base: %w", err)
		}

		// Update stack comment
		root, err := tree.Build(cfg)
		if err != nil {
			return fmt.Errorf("build tree: %w", err)
		}
		comment := github.GenerateStackComment(root, branch, trunk)
		if comment != "" {
			if err := gh.CreateOrUpdateStackComment(existingPR, comment); err != nil {
				fmt.Printf("Warning: failed to update stack comment: %v\n", err)
			}
		}
		return nil
	}

	// Create new PR
	fmt.Printf("Creating PR for %q targeting %q...\n", branch, base)

	var prNumber int
	if base != trunk {
		// Create as draft since it's part of a stack
		prNumber, err = gh.CreateDraftPR(branch, base, branch, "")
		if err != nil {
			return err
		}
		fmt.Printf("Created draft PR #%d for %s -> %s\n", prNumber, branch, base)
	} else {
		prNumber, err = gh.CreatePR(branch, base, branch, "")
		if err != nil {
			return err
		}
		fmt.Printf("Created PR #%d for %s -> %s\n", prNumber, branch, base)
	}

	// Store PR number
	if err := cfg.SetPR(branch, prNumber); err != nil {
		return err
	}

	// Post stack navigation comment
	root, err := tree.Build(cfg)
	if err != nil {
		return fmt.Errorf("build tree: %w", err)
	}

	comment := github.GenerateStackComment(root, branch, trunk)
	if comment != "" {
		if err := gh.CreateOrUpdateStackComment(prNumber, comment); err != nil {
			fmt.Printf("Warning: failed to add stack comment: %v\n", err)
			// Don't fail the command for comment issues
		}
	}

	return nil
}
