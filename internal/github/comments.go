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
func GenerateStackComment(root *tree.Node, currentBranch, trunk string) string {
	var sb strings.Builder

	// Find the current node
	currentNode := tree.FindNode(root, currentBranch)
	if currentNode == nil {
		return ""
	}

	// Start with marker
	sb.WriteString(StackCommentMarker)
	sb.WriteString("\n")

	// Add warning if not targeting trunk
	if currentNode.Parent != nil && currentNode.Parent.Name != trunk {
		sb.WriteString("> [!WARNING]\n")
		sb.WriteString(fmt.Sprintf("> This PR is part of a stack and targets **%s**, not **%s**.\n", currentNode.Parent.Name, trunk))
		sb.WriteString("> Do not merge until the parent PR is merged.\n\n")
	}

	// Stack header
	sb.WriteString("### Stack\n\n")
	sb.WriteString("```\n")

	// Render tree from root
	renderTree(&sb, root, currentBranch, 0)

	sb.WriteString("```\n\n")
	sb.WriteString("---\n")
	sb.WriteString("*Managed by [gh-stack](https://github.com/boneskull/gh-stack)*\n")

	return sb.String()
}

// renderTree recursively renders the tree structure.
func renderTree(sb *strings.Builder, node *tree.Node, currentBranch string, depth int) {
	// Build prefix based on depth
	prefix := ""
	if depth > 0 {
		prefix = strings.Repeat("    ", depth-1) + "└── "
	}

	// Build the line
	line := prefix + node.Name

	// Add PR reference if exists
	if node.PR > 0 {
		if node.Name == currentBranch {
			line += fmt.Sprintf(" ← #%d (this PR)", node.PR)
		} else {
			line += fmt.Sprintf(" → #%d", node.PR)
		}
	}

	sb.WriteString(line)
	sb.WriteString("\n")

	// Render children
	for _, child := range node.Children {
		renderTree(sb, child, currentBranch, depth+1)
	}
}

// FindStackComment searches for an existing gh-stack comment on a PR.
// Returns the comment ID if found, or 0 if not found.
func (c *Client) FindStackComment(prNumber int) (int, error) {
	comments, err := c.ListComments(prNumber)
	if err != nil {
		return 0, err
	}

	for _, comment := range comments {
		if strings.Contains(comment.Body, StackCommentMarker) {
			return comment.ID, nil
		}
	}

	return 0, nil
}

// CreateOrUpdateStackComment finds an existing stack comment by marker and updates it,
// or creates a new one if none exists.
func (c *Client) CreateOrUpdateStackComment(prNumber int, body string) error {
	existingID, err := c.FindStackComment(prNumber)
	if err != nil {
		return fmt.Errorf("find existing comment: %w", err)
	}

	if existingID > 0 {
		return c.UpdateComment(existingID, body)
	}

	_, err = c.CreateComment(prNumber, body)
	return err
}
