// cmd/adopt.go
package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/detect"
	"github.com/boneskull/gh-stack/internal/git"
	"github.com/boneskull/gh-stack/internal/github"
	"github.com/boneskull/gh-stack/internal/prompt"
	"github.com/boneskull/gh-stack/internal/style"
	"github.com/spf13/cobra"
)

var adoptCmd = &cobra.Command{
	Use:   "adopt [parent]",
	Short: "Start tracking an existing branch",
	Long: `Start tracking an existing branch by setting its parent.

When no parent is specified, the parent is auto-detected using PR base branch
and merge-base analysis. If the result is ambiguous, you will be prompted to
choose (interactive) or an error is returned (non-interactive).

By default, adopts the current branch. Use --branch to specify a different branch.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runAdopt,
}

var adoptBranchFlag string

func init() {
	adoptCmd.Flags().StringVar(&adoptBranchFlag, "branch", "", "branch to adopt (default: current branch)")
	rootCmd.AddCommand(adoptCmd)
}

func runAdopt(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	cfg, err := config.Load(cwd)
	if err != nil {
		return err
	}

	g := git.New(cwd)
	s := style.New()

	// Determine branch to adopt (from flag or current branch)
	var branchName string
	if adoptBranchFlag != "" {
		branchName = adoptBranchFlag
	} else {
		branchName, err = g.CurrentBranch()
		if err != nil {
			return err
		}
	}

	// Validate branch exists
	if !g.BranchExists(branchName) {
		return fmt.Errorf("branch %q does not exist", branchName)
	}

	// Check if already tracked
	if _, getParentErr := cfg.GetParent(branchName); getParentErr == nil {
		return fmt.Errorf("branch %q is already tracked", branchName)
	}

	// Validate parent is trunk or tracked
	trunk, err := cfg.GetTrunk()
	if err != nil {
		return err
	}

	var parent string
	var detectedPRNumber int

	if len(args) > 0 {
		// Explicit parent provided
		parent = args[0]
	} else {
		// Auto-detect parent
		tracked, listErr := cfg.ListTrackedBranches()
		if listErr != nil {
			return fmt.Errorf("list tracked branches: %w", listErr)
		}

		// Try to get GitHub client (may fail if no auth -- that's ok)
		gh, _ := github.NewClient() //nolint:errcheck // nil client is fine for local-only detection

		result, detectErr := detect.DetectParent(branchName, tracked, trunk, g, gh)
		if detectErr != nil {
			return fmt.Errorf("auto-detect parent: %w", detectErr)
		}

		switch result.Confidence {
		case detect.High, detect.Medium:
			parent = result.Parent
			fmt.Printf("%s Detected parent %s %s\n",
				s.SuccessIcon(), s.Branch(parent), s.Muted("("+result.Confidence.String()+" confidence)"))
		case detect.Ambiguous:
			if len(result.Candidates) == 0 {
				return fmt.Errorf("could not detect parent for %s; specify one explicitly", s.Branch(branchName))
			}
			if !prompt.IsInteractive() {
				return fmt.Errorf("ambiguous parent for %s (candidates: %v); specify one explicitly",
					s.Branch(branchName), result.Candidates)
			}
			idx, promptErr := prompt.Select(
				fmt.Sprintf("Multiple parent candidates for %s:", branchName),
				result.Candidates, 0)
			if promptErr != nil {
				return fmt.Errorf("prompt: %w", promptErr)
			}
			parent = result.Candidates[idx]
		}

		detectedPRNumber = result.PRNumber
	}

	if parent != trunk {
		if _, parentErr := cfg.GetParent(parent); parentErr != nil {
			return fmt.Errorf("parent %q is not tracked", parent)
		}
	}

	// Check for cycles via config parent chain walk (catches cases the tree
	// model misses when nodes with broken parent links are omitted).
	if wouldCycle(cfg, branchName, parent) {
		return errors.New("cannot adopt: would create a cycle")
	}

	// Set parent
	if err := cfg.SetParent(branchName, parent); err != nil {
		return err
	}

	// Store fork point
	forkPoint, fpErr := g.GetMergeBase(branchName, parent)
	if fpErr == nil {
		_ = cfg.SetForkPoint(branchName, forkPoint) //nolint:errcheck // best effort
	}

	// Store PR number if detected
	if detectedPRNumber > 0 {
		_ = cfg.SetPR(branchName, detectedPRNumber) //nolint:errcheck // best effort
	}

	fmt.Printf("%s Adopted branch %s with parent %s\n", s.SuccessIcon(), s.Branch(branchName), s.Branch(parent))
	return nil
}
