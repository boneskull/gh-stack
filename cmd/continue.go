// cmd/continue.go
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

var continueCmd = &cobra.Command{
	Use:   "continue",
	Short: "Continue a cascade after resolving conflicts",
	Long:  `Continue a cascade operation after resolving rebase conflicts.`,
	RunE:  runContinue,
}

func init() {
	rootCmd.AddCommand(continueCmd)
}

func runContinue(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	g := git.New(cwd)

	// Check if cascade in progress
	st, err := state.Load(g.GetGitDir())
	if err != nil {
		return fmt.Errorf("no cascade in progress")
	}

	// Complete the in-progress rebase
	if g.IsRebaseInProgress() {
		fmt.Println("Continuing rebase...")
		if err := g.RebaseContinue(); err != nil {
			return fmt.Errorf("rebase --continue failed; resolve conflicts first")
		}
	}

	fmt.Printf("Completed %s\n", st.Current)

	// Continue with remaining branches
	if len(st.Pending) == 0 {
		state.Remove(g.GetGitDir())
		fmt.Println("Cascade complete!")
		return nil
	}

	cfg, err := config.Load(cwd)
	if err != nil {
		return err
	}

	// Build tree to get node objects
	root, err := tree.Build(cfg)
	if err != nil {
		return err
	}

	var branches []*tree.Node
	for _, name := range st.Pending {
		if node := tree.FindNode(root, name); node != nil {
			branches = append(branches, node)
		}
	}

	// Remove state file before continuing (will be recreated if conflict)
	state.Remove(g.GetGitDir())

	return doCascade(g, cfg, branches, false)
}
