// cmd/adopt.go
package cmd

import (
	"fmt"
	"os"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/git"
	"github.com/boneskull/gh-stack/internal/tree"
	"github.com/spf13/cobra"
)

var adoptCmd = &cobra.Command{
	Use:   "adopt [branch]",
	Short: "Start tracking an existing branch",
	Long:  `Start tracking an existing branch by setting its parent.`,
	Args:  cobra.MaximumNArgs(1),
	RunE:  runAdopt,
}

var adoptParentFlag string

func init() {
	adoptCmd.Flags().StringVar(&adoptParentFlag, "parent", "", "parent branch")
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

	// Determine branch to adopt
	var branchName string
	if len(args) > 0 {
		branchName = args[0]
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

	// Determine parent
	parent := adoptParentFlag
	if parent == "" {
		return fmt.Errorf("--parent is required")
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

	// Check for cycles (branch can't be ancestor of parent)
	root, err := tree.Build(cfg)
	if err != nil {
		return err
	}

	parentNode := tree.FindNode(root, parent)
	if parentNode != nil {
		for _, ancestor := range tree.GetAncestors(parentNode) {
			if ancestor.Name == branchName {
				return fmt.Errorf("cannot adopt: would create a cycle")
			}
		}
	}

	// Set parent
	if err := cfg.SetParent(branchName, parent); err != nil {
		return err
	}

	fmt.Printf("Adopted branch %q with parent %q\n", branchName, parent)
	return nil
}
