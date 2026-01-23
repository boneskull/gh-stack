// cmd/link.go
package cmd

import (
	"fmt"
	"os"
	"strconv"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/git"
	"github.com/spf13/cobra"
)

var linkCmd = &cobra.Command{
	Use:   "link <pr-number>",
	Short: "Associate a PR with the current branch",
	Long:  `Associate an existing GitHub PR number with the current branch.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runLink,
}

func init() {
	rootCmd.AddCommand(linkCmd)
}

func runLink(cmd *cobra.Command, args []string) error {
	prNumber, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid PR number: %s", args[0])
	}

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

	// Verify branch is tracked
	if _, err := cfg.GetParent(branch); err != nil {
		return fmt.Errorf("branch %q is not tracked", branch)
	}

	if err := cfg.SetPR(branch, prNumber); err != nil {
		return err
	}

	fmt.Printf("Linked PR #%d to branch %q\n", prNumber, branch)
	return nil
}
