// cmd/pr.go
package cmd

import (
	"context"
	"fmt"
	"os"

	gh "github.com/cli/go-gh/v2"
	"github.com/spf13/cobra"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/git"
	"github.com/boneskull/gh-stack/internal/github"
	"github.com/boneskull/gh-stack/internal/tree"
)

var prCmd = &cobra.Command{
	Use:   "pr [-- <gh-pr-create-flags>...]",
	Short: "Create or update a PR for the current branch",
	Long: `Create a new PR targeting the parent branch, or update an existing PR's base.

This command wraps 'gh pr create', automatically setting the base branch to the
stack parent. Any additional flags after '--' are passed through to 'gh pr create'.

Examples:
  gh stack pr                          # Interactive PR creation
  gh stack pr -- --title "My PR"       # With title
  gh stack pr -- --fill --web          # Fill from commits, open in browser
  gh stack pr --base main              # Override base branch`,
	RunE:               runPR,
	DisableFlagParsing: false,
}

var prBaseFlag string

func init() {
	prCmd.Flags().StringVar(&prBaseFlag, "base", "", "override base branch (default: stack parent)")
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

	ghClient, err := github.NewClient()
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
		return fmt.Errorf("branch %q is not tracked; use 'gh stack create' or 'gh stack track' first", branch)
	}

	trunk, err := cfg.GetTrunk()
	if err != nil {
		return err
	}

	base := prBaseFlag
	if base == "" {
		base = parent
	}

	// Check if PR already exists
	existingPR, _ := cfg.GetPR(branch) //nolint:errcheck // 0 is fine if no PR
	if existingPR > 0 {
		return updateExistingPR(ghClient, cfg, existingPR, branch, base, trunk)
	}

	// Build args for gh pr create
	ghArgs := []string{"pr", "create", "--base", base}

	// Auto-draft if not targeting trunk (middle of stack)
	if base != trunk {
		ghArgs = append(ghArgs, "--draft")
		fmt.Printf("Creating draft PR (base %q is not trunk %q)\n", base, trunk)
	}

	// Pass through any additional args from user
	ghArgs = append(ghArgs, args...)

	// Let user interact with gh pr create
	ctx := context.Background()
	if execErr := gh.ExecInteractive(ctx, ghArgs...); execErr != nil {
		return fmt.Errorf("gh pr create failed: %w", execErr)
	}

	// Find the PR we just created
	pr, err := ghClient.FindPRByHead(branch)
	if err != nil {
		return fmt.Errorf("failed to find created PR: %w", err)
	}
	if pr == nil {
		// User might have cancelled
		fmt.Println("No PR was created.")
		return nil
	}

	// Store PR number
	if setErr := cfg.SetPR(branch, pr.Number); setErr != nil {
		return setErr
	}

	// Post stack navigation comment
	root, err := tree.Build(cfg)
	if err != nil {
		return fmt.Errorf("build tree: %w", err)
	}

	if err := ghClient.GenerateAndPostStackComment(root, branch, trunk, pr.Number); err != nil {
		fmt.Printf("Warning: failed to add stack comment: %v\n", err)
	}

	fmt.Printf("Stored PR #%d for branch %q\n", pr.Number, branch)
	return nil
}

// updateExistingPR updates the base branch and stack comment for an existing PR.
func updateExistingPR(ghClient *github.Client, cfg *config.Config, prNumber int, branch, base, trunk string) error {
	fmt.Printf("PR #%d already exists, updating base to %q\n", prNumber, base)

	if err := ghClient.UpdatePRBase(prNumber, base); err != nil {
		return fmt.Errorf("failed to update PR base: %w", err)
	}

	// Update stack comment
	root, err := tree.Build(cfg)
	if err != nil {
		return fmt.Errorf("build tree: %w", err)
	}

	if err := ghClient.GenerateAndPostStackComment(root, branch, trunk, prNumber); err != nil {
		fmt.Printf("Warning: failed to update stack comment: %v\n", err)
	}

	fmt.Println(ghClient.PRURL(prNumber))
	return nil
}
