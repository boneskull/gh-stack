// Package github provides GitHub API integration for gh-stack.
package github

import (
	"fmt"
	"strings"

	"github.com/boneskull/gh-stack/internal/tree"
)

// StackCommentMarker identifies gh-stack managed comments.
const StackCommentMarker = "<!-- gh-stack:nav -->"

// GenerateStackComment builds markdown for a PR's stack position.
// It includes a warning if the PR targets a non-trunk branch.
// The repoURL should be the base repository URL (e.g., "https://github.com/owner/repo").
// The prInfo map provides titles for PRs (keyed by PR number).
//
// Only the current stack is rendered: the path from root to the current branch,
// plus all descendants of the current branch. Sibling stacks are excluded.
func GenerateStackComment(root *tree.Node, currentBranch, trunk, repoURL string, prInfo map[int]PRInfo) string {
	var sb strings.Builder

	// Find the current node
	currentNode := tree.FindNode(root, currentBranch)
	if currentNode == nil {
		return ""
	}

	// Build ancestor path: the set of branch names from root down to (and
	// including) the current branch. When rendering, only children on this
	// path are shown for ancestor nodes; descendants of the current branch
	// are always shown in full.
	ancestorPath := make(map[string]bool)
	for n := currentNode; n != nil; n = n.Parent {
		ancestorPath[n.Name] = true
	}

	// Start with marker
	sb.WriteString(StackCommentMarker)
	sb.WriteString("\n")

	// Add warning if not targeting trunk
	if currentNode.Parent != nil && currentNode.Parent.Name != trunk {
		sb.WriteString("> [!WARNING]\n")
		sb.WriteString(fmt.Sprintf("> This PR is part of a stack and targets branch `%s`, not `%s`.\n", currentNode.Parent.Name, trunk))

		// Build parent PR reference if available
		parentPR := currentNode.Parent.PR
		if parentPR > 0 {
			parentURL := fmt.Sprintf("%s/pull/%d", repoURL, parentPR)
			linkText := fmt.Sprintf("#%d", parentPR)
			if info, ok := prInfo[parentPR]; ok && info.Title != "" {
				linkText = fmt.Sprintf("%s #%d", info.Title, parentPR)
			}
			sb.WriteString(fmt.Sprintf("> **DO NOT MERGE** until [%s](%s) is merged into `%s`.\n\n", linkText, parentURL, trunk))
		} else {
			sb.WriteString(fmt.Sprintf("> **DO NOT MERGE** until the parent branch is merged into `%s`.\n\n", trunk))
		}
	}

	// Stack header
	sb.WriteString("### :books: Pull Request Stack\n\n")

	// Render tree from root as nested markdown list, filtered to current stack
	renderTree(&sb, root, currentBranch, repoURL, prInfo, 0, ancestorPath)

	sb.WriteString("\n---\n")
	sb.WriteString("*Managed by [gh-stack](https://github.com/boneskull/gh-stack)*\n")

	return sb.String()
}

// CollectPRNumbers walks a tree and returns all PR numbers found.
func CollectPRNumbers(root *tree.Node) []int {
	var numbers []int
	collectPRNumbersRecursive(root, &numbers)
	return numbers
}

func collectPRNumbersRecursive(node *tree.Node, numbers *[]int) {
	if node.PR > 0 {
		*numbers = append(*numbers, node.PR)
	}
	for _, child := range node.Children {
		collectPRNumbersRecursive(child, numbers)
	}
}

// renderTree recursively renders the tree structure as nested markdown lists.
//
// The ancestorPath set controls which children are rendered at each level:
//   - For nodes that are ancestors of the current branch (on the path but not
//     the current branch itself), only the child on the ancestor path is shown.
//   - For the current branch and all its descendants, all children are shown.
//
// This ensures only the current stack is rendered, not sibling stacks.
func renderTree(sb *strings.Builder, node *tree.Node, currentBranch, repoURL string, prInfo map[int]PRInfo, depth int, ancestorPath map[string]bool) {
	// Build prefix based on depth (2 spaces per level for markdown nested lists)
	prefix := strings.Repeat("  ", depth) + "- "

	isCurrent := node.Name == currentBranch

	// Format: "[Title #N](url) - branch: `name`" or "branch: `name`" if no PR
	if node.PR > 0 {
		prURL := fmt.Sprintf("%s/pull/%d", repoURL, node.PR)
		linkText := fmt.Sprintf("#%d", node.PR)
		if info, ok := prInfo[node.PR]; ok && info.Title != "" {
			linkText = fmt.Sprintf("%s #%d", info.Title, node.PR)
		}

		if isCurrent {
			// Bold the link for current PR
			fmt.Fprintf(sb, "%s**[%s](%s)** - branch: `%s` *(this PR)*", prefix, linkText, prURL, node.Name)
		} else {
			fmt.Fprintf(sb, "%s[%s](%s) - branch: `%s`", prefix, linkText, prURL, node.Name)
		}
	} else {
		// No PR - show "branch: `name`"
		if isCurrent {
			fmt.Fprintf(sb, "%s**branch: `%s`** *(this PR)*", prefix, node.Name)
		} else {
			fmt.Fprintf(sb, "%sbranch: `%s`", prefix, node.Name)
		}
	}

	sb.WriteString("\n")

	// For ancestor nodes (above the current branch), only render the child
	// that leads to the current branch. For the current branch and its
	// descendants, render all children.
	isAncestorAboveCurrent := ancestorPath[node.Name] && !isCurrent
	for _, child := range node.Children {
		if isAncestorAboveCurrent && !ancestorPath[child.Name] {
			continue
		}
		renderTree(sb, child, currentBranch, repoURL, prInfo, depth+1, ancestorPath)
	}
}

// FindStackComment searches for an existing gh-stack comment on a PR.
// Returns the comment ID and body if found, or (0, "") if not found.
func (c *Client) FindStackComment(prNumber int) (int, string, error) {
	comments, err := c.ListComments(prNumber)
	if err != nil {
		return 0, "", err
	}

	for _, comment := range comments {
		if strings.Contains(comment.Body, StackCommentMarker) {
			return comment.ID, comment.Body, nil
		}
	}

	return 0, "", nil
}

// CreateOrUpdateStackComment finds an existing stack comment by marker and updates it,
// or creates a new one if none exists. Skips the update if the body hasn't changed.
func (c *Client) CreateOrUpdateStackComment(prNumber int, body string) error {
	existingID, existingBody, err := c.FindStackComment(prNumber)
	if err != nil {
		return fmt.Errorf("find existing comment: %w", err)
	}

	if existingID > 0 {
		// Skip update if body hasn't changed
		if existingBody == body {
			return nil
		}
		return c.UpdateComment(existingID, body)
	}

	_, err = c.CreateComment(prNumber, body)
	return err
}

// GenerateAndPostStackComment generates and posts/updates a stack comment for a PR.
// It fetches PR titles via GraphQL and renders the full comment.
func (c *Client) GenerateAndPostStackComment(root *tree.Node, branch, trunk string, prNumber int) error {
	// Collect all PR numbers from the tree
	prNumbers := CollectPRNumbers(root)

	// Fetch PR titles in a single GraphQL request
	prInfo, err := c.GetPRTitles(prNumbers)
	if err != nil {
		// Non-fatal: we can still render without titles
		prInfo = make(map[int]PRInfo)
	}

	comment := GenerateStackComment(root, branch, trunk, c.RepoURL(), prInfo)
	if comment == "" {
		return nil
	}

	return c.CreateOrUpdateStackComment(prNumber, comment)
}
