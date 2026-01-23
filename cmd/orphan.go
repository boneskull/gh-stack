// cmd/orphan.go
package cmd

import (
	"fmt"
	"os"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/git"
	"github.com/boneskull/gh-stack/internal/tree"
	"github.com/spf13/cobra"
)

var orphanCmd = &cobra.Command{
	Use:   "orphan [branch]",
	Short: "Stop tracking a branch",
	Long:  `Stop tracking a branch by removing it from the stack tree.`,
	Args:  cobra.MaximumNArgs(1),
	RunE:  runOrphan,
}

var orphanForceFlag bool

func init() {
	orphanCmd.Flags().BoolVar(&orphanForceFlag, "force", false, "also orphan all descendants")
	rootCmd.AddCommand(orphanCmd)
}

func runOrphan(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	cfg, err := config.Load(cwd)
	if err != nil {
		return err
	}

	g := git.New(cwd)

	// Determine branch to orphan
	var branchName string
	if len(args) > 0 {
		branchName = args[0]
	} else {
		branchName, err = g.CurrentBranch()
		if err != nil {
			return err
		}
	}

	// Build tree to check for children
	root, err := tree.Build(cfg)
	if err != nil {
		return err
	}

	node := tree.FindNode(root, branchName)
	if node == nil {
		return fmt.Errorf("branch %q is not tracked", branchName)
	}

	// Check for children
	if len(node.Children) > 0 && !orphanForceFlag {
		return fmt.Errorf("branch %q has children; use --force to orphan descendants too", branchName)
	}

	// Orphan descendants if --force
	if orphanForceFlag {
		descendants := tree.GetDescendants(node)
		for _, desc := range descendants {
			cfg.RemoveParent(desc.Name)
			cfg.RemovePR(desc.Name)
			fmt.Printf("Orphaned %q\n", desc.Name)
		}
	}

	// Orphan the branch
	cfg.RemoveParent(branchName)
	cfg.RemovePR(branchName)
	fmt.Printf("Orphaned %q\n", branchName)

	return nil
}
