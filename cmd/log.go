// cmd/log.go
package cmd

import (
	"fmt"
	"os"
	"strconv"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/detect"
	"github.com/boneskull/gh-stack/internal/git"
	"github.com/boneskull/gh-stack/internal/github"
	"github.com/boneskull/gh-stack/internal/style"
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
	logNoDetectFlag  bool
)

func init() {
	logCmd.Flags().BoolVar(&logAllFlag, "all", false, "show all branches")
	logCmd.Flags().BoolVar(&logPorcelainFlag, "porcelain", false, "machine-readable output")
	logCmd.Flags().BoolVar(&logNoDetectFlag, "no-detect", false, "skip auto-detection of untracked branches")
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

	// Auto-detect untracked branches (read-only — injects virtual nodes)
	if !logNoDetectFlag {
		injectDetectedNodes(root, cfg, g)
	}

	currentBranch, _ := g.CurrentBranch() //nolint:errcheck // empty string is fine for display

	// Try to get GitHub client for PR URLs (optional - may fail if not in a GitHub repo)
	gh, _ := github.NewClient() //nolint:errcheck // nil is fine, URLs won't be shown

	s := style.New()

	if logPorcelainFlag {
		printPorcelain(root, currentBranch, gh, s)
	} else {
		opts := tree.FormatOptions{
			CurrentBranch: currentBranch,
			Style:         s,
		}
		if gh != nil {
			opts.PRURLFunc = gh.PRURL
		}
		fmt.Print(tree.FormatTree(root, opts))
	}

	return nil
}

// injectDetectedNodes discovers untracked branches via merge-base analysis
// and injects them as virtual (Detected) nodes in the tree. This is strictly
// read-only — no git config is modified.
func injectDetectedNodes(root *tree.Node, cfg *config.Config, g *git.Git) {
	trunk := root.Name
	tracked, err := cfg.ListTrackedBranches()
	if err != nil {
		return // silent failure for read-only preview
	}

	candidates, err := detect.FindUntrackedCandidates(g, tracked, trunk)
	if err != nil {
		return
	}

	for _, branch := range candidates {
		result, detectErr := detect.DetectParentLocal(branch, tracked, trunk, g)
		if detectErr != nil || result.Confidence == detect.Ambiguous {
			continue
		}

		parentNode := tree.FindNode(root, result.Parent)
		if parentNode == nil {
			continue
		}

		var cl tree.ConfidenceLevel
		switch result.Confidence {
		case detect.Medium:
			cl = tree.ConfidenceMedium
		case detect.High:
			cl = tree.ConfidenceHigh
		}

		node := &tree.Node{
			Name:       branch,
			Parent:     parentNode,
			Detected:   true,
			Confidence: cl,
		}
		parentNode.Children = append(parentNode.Children, node)
	}
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
func printPorcelain(node *tree.Node, current string, gh *github.Client, s *style.Style) {
	t := term.FromEnv()
	isTTY := t.IsTerminalOutput()

	var width int
	if isTTY {
		w, _, err := t.Size()
		if err != nil || w <= 0 {
			width = 80 // reasonable default width for TTY when detection fails
		} else {
			width = w
		}
	} else {
		// In non-TTY mode, tableprinter outputs TSV; use a large width to avoid truncation.
		width = 4096
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
			branchName = s.Bold("* " + n.Name)
		} else if isTTY {
			branchName = s.Branch(n.Name)
		}

		parent := ""
		if n.Parent != nil {
			if isTTY {
				parent = s.Branch(n.Parent.Name)
			} else {
				parent = n.Parent.Name
			}
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
		fmt.Fprintf(os.Stderr, "%s failed to render table: %v\n", s.WarningIcon(), err)
	}
}
