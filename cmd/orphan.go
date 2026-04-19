// cmd/orphan.go
package cmd

import (
	"fmt"
	"os"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/git"
	"github.com/boneskull/gh-stack/internal/style"
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
	orphanCmd.Flags().BoolVarP(&orphanForceFlag, "force", "f", false, "also orphan all descendants")
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

	s := style.New()

	// Orphan descendants if --force
	if orphanForceFlag {
		descendants := tree.GetDescendants(node)
		for _, desc := range descendants {
			_ = cfg.RemoveParent(desc.Name)    //nolint:errcheck // best effort cleanup
			_ = cfg.RemovePR(desc.Name)        //nolint:errcheck // best effort cleanup
			_ = cfg.RemoveForkPoint(desc.Name) //nolint:errcheck // best effort cleanup
			_ = cfg.RemovePRBase(desc.Name)    //nolint:errcheck // best effort cleanup
			fmt.Printf("%s Orphaned %s\n", s.SuccessIcon(), s.Branch(desc.Name))
		}
	}

	// Orphan the branch
	_ = cfg.RemoveParent(branchName)    //nolint:errcheck // best effort cleanup
	_ = cfg.RemovePR(branchName)        //nolint:errcheck // best effort cleanup
	_ = cfg.RemoveForkPoint(branchName) //nolint:errcheck // best effort cleanup
	_ = cfg.RemovePRBase(branchName)    //nolint:errcheck // best effort cleanup
	fmt.Printf("%s Orphaned %s\n", s.SuccessIcon(), s.Branch(branchName))

	return nil
}
