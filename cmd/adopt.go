// cmd/adopt.go
package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/git"
	"github.com/boneskull/gh-stack/internal/style"
	"github.com/spf13/cobra"
)

var adoptCmd = &cobra.Command{
	Use:   "adopt <parent>",
	Short: "Start tracking an existing branch",
	Long: `Start tracking an existing branch by setting its parent.

By default, adopts the current branch. Use --branch to specify a different branch.`,
	Args: cobra.ExactArgs(1),
	RunE: runAdopt,
}

var adoptBranchFlag string

func init() {
	adoptCmd.Flags().StringVarP(&adoptBranchFlag, "branch", "b", "", "branch to adopt (default: current branch)")
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

	// Parent is the required positional argument
	parent := args[0]

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

	// A branch cannot be its own parent
	if parent == branchName {
		return fmt.Errorf("cannot adopt: branch %q cannot be its own parent", branchName)
	}

	// Check if already tracked, capture old parent if so
	oldParent, alreadyTracked := "", false
	if p, getParentErr := cfg.GetParent(branchName); getParentErr == nil {
		oldParent, alreadyTracked = p, true
	}

	// Validate parent is trunk or tracked
	trunk, err := cfg.GetTrunk()
	if err != nil {
		return err
	}

	if parent != trunk {
		if _, parentErr := cfg.GetParent(parent); parentErr != nil {
			return fmt.Errorf("parent %q is not tracked", parent)
		}
	}

	// Check for cycles by walking configured parent links directly. A broken
	// parent can disconnect branches from the trunk-rooted tree while their
	// stackParent links still form a cycle.
	seen := map[string]bool{}
	for current := parent; current != trunk; {
		if current == branchName {
			return errors.New("cannot adopt: would create a cycle")
		}
		if seen[current] {
			return errors.New("cannot adopt: parent chain contains a cycle")
		}
		seen[current] = true

		next, parentErr := cfg.GetParent(current)
		if parentErr != nil {
			break
		}
		current = next
	}

	s := style.New()

	// No-op: already tracked with the same parent
	if alreadyTracked && oldParent == parent {
		fmt.Printf("%s Branch %s is already tracked with parent %s\n", s.WarningIcon(), s.Branch(branchName), s.Branch(parent))
		return nil
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

	if alreadyTracked {
		fmt.Printf("%s Updated branch %s parent from %s to %s\n", s.SuccessIcon(), s.Branch(branchName), s.Branch(oldParent), s.Branch(parent))
	} else {
		fmt.Printf("%s Adopted branch %s with parent %s\n", s.SuccessIcon(), s.Branch(branchName), s.Branch(parent))
	}
	return nil
}
