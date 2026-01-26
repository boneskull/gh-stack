// cmd/log.go
package cmd

import (
	"fmt"
	"os"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/git"
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

	if logPorcelainFlag {
		printPorcelain(root, currentBranch)
	} else {
		printTree(root, "", true, currentBranch)
	}

	return nil
}

func printTree(node *tree.Node, prefix string, isLast bool, current string) {
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
		prInfo = fmt.Sprintf(" (#%d)", node.PR)
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
		printTree(child, childPrefix, isLastChild, current)
	}
}

func printPorcelain(node *tree.Node, current string) {
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
		fmt.Printf("%s\t%s\t%d\t%s\n", n.Name, parent, n.PR, isCurrent)
		for _, child := range n.Children {
			printNode(child, depth+1)
		}
	}
	printNode(node, 0)
}
