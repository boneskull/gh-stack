// cmd/init.go
package cmd

import (
	"fmt"
	"os"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/git"
	"github.com/boneskull/gh-stack/internal/style"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize stack tracking in the repository",
	Long:  `Initialize stack tracking by setting the trunk branch.`,
	RunE:  runInit,
}

var trunkFlag string

func init() {
	initCmd.Flags().StringVar(&trunkFlag, "trunk", "", "trunk branch name (default: main or master)")
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	cfg, err := config.Load(cwd)
	if err != nil {
		return err
	}

	g := git.New(cwd)

	// Determine trunk branch
	trunk := trunkFlag
	if trunk == "" {
		// Try main, then master
		if g.BranchExists("main") {
			trunk = "main"
		} else if g.BranchExists("master") {
			trunk = "master"
		} else {
			return fmt.Errorf("could not determine trunk branch; use --trunk to specify")
		}
	}

	// Validate trunk exists
	if !g.BranchExists(trunk) {
		return fmt.Errorf("branch %q does not exist", trunk)
	}

	s := style.New()

	// Check if already initialized
	if existing, err := cfg.GetTrunk(); err == nil {
		fmt.Printf("Already initialized with trunk %s\n", s.Branch(existing))
		return nil
	}

	if err := cfg.SetTrunk(trunk); err != nil {
		return err
	}

	fmt.Printf("%s Initialized stack tracking with trunk %s\n", s.SuccessIcon(), s.Branch(trunk))
	return nil
}
