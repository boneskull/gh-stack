// cmd/log.go
package cmd

import (
	"fmt"
	"os"
	"strconv"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/git"
	"github.com/boneskull/gh-stack/internal/github"
	"github.com/boneskull/gh-stack/internal/tree"
	"github.com/cli/go-gh/v2/pkg/tableprinter"
	"github.com/cli/go-gh/v2/pkg/term"
	"github.com/spf13/cobra"
)

var logCmd = &cobra.Command{
	Use:   "log",
	Short: "Display the branch tree",
	Long:  `Display the stack tree structure with branch names and PR numbers.`,
	RunE:  runLog,
}

var (
	logAllFlag       bool
	logPorcelainFlag bool
)

func init() {
	logCmd.Flags().BoolVar(&logAllFlag, "all", false, "show all branches")
	logCmd.Flags().BoolVar(&logPorcelainFlag, "porcelain", false, "machine-readable output")
	rootCmd.AddCommand(logCmd)
}

func runLog(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	cfg, err := config.Load(cwd)
	if err != nil {
		return err
	}

	root, err := tree.Build(cfg)
	if err != nil {
		return err
	}

	g := git.New(cwd)
	currentBranch, _ := g.CurrentBranch() //nolint:errcheck // empty string is fine for display

	// Try to get GitHub client for PR URLs (optional - may fail if not in a GitHub repo)
	gh, _ := github.NewClient() //nolint:errcheck // nil is fine, URLs won't be shown

	if logPorcelainFlag {
		printPorcelain(root, currentBranch, gh)
	} else {
		opts := tree.FormatOptions{
			CurrentBranch: currentBranch,
		}
		if gh != nil {
			opts.PRURLFunc = gh.PRURL
		}
		fmt.Print(tree.FormatTree(root, opts))
	}

	return nil
}

// printPorcelain outputs stack information in table format.
// In TTY mode, outputs nicely formatted columns.
// In non-TTY mode (piped/scripted), outputs tab-separated values.
//
// Columns:
//   - BRANCH: branch name (* prefix for current branch in TTY mode)
//   - PARENT: parent branch name (empty for trunk)
//   - PR: associated PR number (empty if none)
//   - URL: full PR URL (empty if no PR or GitHub client unavailable)
func printPorcelain(node *tree.Node, current string, gh *github.Client) {
	t := term.FromEnv()
	isTTY := t.IsTerminalOutput()
	width, _, err := t.Size()
	if err != nil || width <= 0 {
		width = 80 // default width for non-TTY or error
	}

	tp := tableprinter.New(os.Stdout, isTTY, width)

	// Add headers in TTY mode
	if isTTY {
		tp.AddHeader([]string{"BRANCH", "PARENT", "PR", "URL"})
	}

	// Collect all nodes in tree order
	var addNode func(*tree.Node)
	addNode = func(n *tree.Node) {
		branchName := n.Name
		if isTTY && n.Name == current {
			branchName = "* " + n.Name
		}

		parent := ""
		if n.Parent != nil {
			parent = n.Parent.Name
		}

		prNum := ""
		if n.PR > 0 {
			prNum = strconv.Itoa(n.PR)
		}

		prURL := ""
		if n.PR > 0 && gh != nil {
			prURL = gh.PRURL(n.PR)
		}

		tp.AddField(branchName)
		tp.AddField(parent)
		tp.AddField(prNum)
		tp.AddField(prURL)
		tp.EndRow()

		for _, child := range n.Children {
			addNode(child)
		}
	}
	addNode(node)

	if err := tp.Render(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to render table: %v\n", err)
	}
}
