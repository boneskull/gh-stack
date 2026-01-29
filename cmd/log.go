// cmd/log.go
package cmd

import (
	"fmt"
	"os"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/git"
	"github.com/boneskull/gh-stack/internal/github"
	"github.com/boneskull/gh-stack/internal/tree"
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
		printTree(root, "", true, currentBranch, gh)
	}

	return nil
}

func printTree(node *tree.Node, prefix string, isLast bool, current string, gh *github.Client) {
	// Determine the branch indicator
	connector := "├── "
	if isLast {
		connector = "└── "
	}
	if prefix == "" {
		connector = ""
	}

	// Build the line
	marker := ""
	if node.Name == current {
		marker = "* "
	}

	prInfo := ""
	if node.PR > 0 {
		if gh != nil {
			prInfo = fmt.Sprintf(" (#%d) %s", node.PR, gh.PRURL(node.PR))
		} else {
			prInfo = fmt.Sprintf(" (#%d)", node.PR)
		}
	}

	fmt.Printf("%s%s%s%s%s\n", prefix, connector, marker, node.Name, prInfo)

	// Prepare prefix for children
	childPrefix := prefix
	if prefix != "" {
		if isLast {
			childPrefix += "    "
		} else {
			childPrefix += "│   "
		}
	}

	for i, child := range node.Children {
		isLastChild := i == len(node.Children)-1
		printTree(child, childPrefix, isLastChild, current, gh)
	}
}

// printPorcelain outputs machine-readable tab-separated format:
// BRANCH<tab>PARENT<tab>PR_NUMBER<tab>IS_CURRENT<tab>PR_URL
//
// Fields:
//   - BRANCH: branch name
//   - PARENT: parent branch name (empty for trunk)
//   - PR_NUMBER: associated PR number (0 if none)
//   - IS_CURRENT: "1" if current branch, "0" otherwise
//   - PR_URL: full PR URL (empty if no PR or GitHub client unavailable)
func printPorcelain(node *tree.Node, current string, gh *github.Client) {
	var printNode func(*tree.Node, int)
	printNode = func(n *tree.Node, depth int) {
		isCurrent := "0"
		if n.Name == current {
			isCurrent = "1"
		}
		parent := ""
		if n.Parent != nil {
			parent = n.Parent.Name
		}
		prURL := ""
		if n.PR > 0 && gh != nil {
			prURL = gh.PRURL(n.PR)
		}
		fmt.Printf("%s\t%s\t%d\t%s\t%s\n", n.Name, parent, n.PR, isCurrent, prURL)
		for _, child := range n.Children {
			printNode(child, depth+1)
		}
	}
	printNode(node, 0)
}
