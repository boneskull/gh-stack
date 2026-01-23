// cmd/create.go
package cmd

import (
	"fmt"
	"os"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/git"
	"github.com/spf13/cobra"
)

var createCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new branch stacked on the current branch",
	Long:  `Create a new branch stacked on the current branch and optionally commit staged changes.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runCreate,
}

var (
	createMessageFlag string
	createEmptyFlag   bool
)

func init() {
	createCmd.Flags().StringVarP(&createMessageFlag, "message", "m", "", "commit message for staged changes")
	createCmd.Flags().BoolVar(&createEmptyFlag, "empty", false, "create branch without committing staged changes")
	rootCmd.AddCommand(createCmd)
}

func runCreate(cmd *cobra.Command, args []string) error {
	branchName := args[0]

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	cfg, err := config.Load(cwd)
	if err != nil {
		return err
	}

	g := git.New(cwd)

	// Get current branch
	currentBranch, err := g.CurrentBranch()
	if err != nil {
		return err
	}

	// Validate current branch is trunk or tracked
	trunk, err := cfg.GetTrunk()
	if err != nil {
		return err
	}

	if currentBranch != trunk {
		if _, err := cfg.GetParent(currentBranch); err != nil {
			return fmt.Errorf("current branch %q is not tracked; use 'gh stack adopt' first", currentBranch)
		}
	}

	// Check if branch already exists
	if g.BranchExists(branchName) {
		return fmt.Errorf("branch %q already exists", branchName)
	}

	// Check for staged changes
	hasStaged, err := g.HasStagedChanges()
	if err != nil {
		return err
	}

	if hasStaged && !createEmptyFlag {
		if createMessageFlag == "" {
			return fmt.Errorf("staged changes found but no commit message provided; use -m or --empty")
		}
	}

	// Create and checkout the new branch
	if err := g.CreateAndCheckout(branchName); err != nil {
		return err
	}

	// Commit staged changes if any
	if hasStaged && !createEmptyFlag && createMessageFlag != "" {
		if err := g.Commit(createMessageFlag); err != nil {
			return err
		}
		fmt.Printf("Committed staged changes: %s\n", createMessageFlag)
	}

	// Set parent
	if err := cfg.SetParent(branchName, currentBranch); err != nil {
		return err
	}

	fmt.Printf("Created branch %q stacked on %q\n", branchName, currentBranch)
	return nil
}
