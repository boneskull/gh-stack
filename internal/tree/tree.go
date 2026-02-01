// internal/tree/tree.go
package tree

import (
	"fmt"
	"sort"
	"strings"

	"github.com/boneskull/gh-stack/internal/config"
)

// Node represents a branch in the stack tree.
type Node struct {
	Name     string
	PR       int // 0 if no PR
	Parent   *Node
	Children []*Node
}

// Build constructs the stack tree from config.
func Build(cfg *config.Config) (*Node, error) {
	trunk, err := cfg.GetTrunk()
	if err != nil {
		return nil, err
	}

	// Create root node for trunk
	root := &Node{Name: trunk}
	nodes := map[string]*Node{trunk: root}

	// Get all tracked branches
	branches, err := cfg.ListTrackedBranches()
	if err != nil {
		return nil, err
	}

	// Create nodes for all branches
	for _, branch := range branches {
		pr, _ := cfg.GetPR(branch) //nolint:errcheck // 0 is fine if no PR
		nodes[branch] = &Node{Name: branch, PR: pr}
	}

	// Wire up parent-child relationships
	for _, branch := range branches {
		parent, err := cfg.GetParent(branch)
		if err != nil {
			continue
		}
		parentNode, ok := nodes[parent]
		if !ok {
			// Broken parent link - parent doesn't exist
			continue
		}
		childNode := nodes[branch]
		childNode.Parent = parentNode
		parentNode.Children = append(parentNode.Children, childNode)
	}

	// Sort children alphabetically for consistent output
	var sortChildren func(*Node)
	sortChildren = func(n *Node) {
		sort.Slice(n.Children, func(i, j int) bool {
			return n.Children[i].Name < n.Children[j].Name
		})
		for _, child := range n.Children {
			sortChildren(child)
		}
	}
	sortChildren(root)

	return root, nil
}

// FindNode finds a node by name in the tree.
func FindNode(root *Node, name string) *Node {
	if root.Name == name {
		return root
	}
	for _, child := range root.Children {
		if found := FindNode(child, name); found != nil {
			return found
		}
	}
	return nil
}

// GetAncestors returns all ancestors from node to root (excluding the node itself).
func GetAncestors(node *Node) []*Node {
	var ancestors []*Node
	current := node.Parent
	for current != nil {
		ancestors = append(ancestors, current)
		current = current.Parent
	}
	return ancestors
}

// GetDescendants returns all descendants of a node (excluding the node itself).
func GetDescendants(node *Node) []*Node {
	var descendants []*Node
	for _, child := range node.Children {
		descendants = append(descendants, child)
		descendants = append(descendants, GetDescendants(child)...)
	}
	return descendants
}

// FormatOptions configures tree formatting.
type FormatOptions struct {
	// CurrentBranch is marked with "* " prefix
	CurrentBranch string
	// PRURLFunc returns the URL for a PR number (optional)
	PRURLFunc func(pr int) string
}

// FormatTree returns a string representation of the tree with box-drawing characters.
func FormatTree(root *Node, opts FormatOptions) string {
	var sb strings.Builder
	formatNode(&sb, root, "", true, opts)
	return sb.String()
}

func formatNode(sb *strings.Builder, node *Node, prefix string, isLast bool, opts FormatOptions) {
	// Determine the branch connector
	connector := "├── "
	if isLast {
		connector = "└── "
	}
	// Root node (no parent) gets no connector
	if node.Parent == nil {
		connector = ""
	}

	// Current branch marker
	marker := ""
	if node.Name == opts.CurrentBranch {
		marker = "* "
	}

	// PR info
	prInfo := ""
	if node.PR > 0 {
		if opts.PRURLFunc != nil {
			prInfo = fmt.Sprintf(" (#%d) %s", node.PR, opts.PRURLFunc(node.PR))
		} else {
			prInfo = fmt.Sprintf(" (#%d)", node.PR)
		}
	}

	sb.WriteString(prefix)
	sb.WriteString(connector)
	sb.WriteString(marker)
	sb.WriteString(node.Name)
	sb.WriteString(prInfo)
	sb.WriteString("\n")

	// Prepare prefix for children
	childPrefix := prefix
	// Only add indentation if we're not at the root
	if node.Parent != nil {
		if isLast {
			childPrefix += "    "
		} else {
			childPrefix += "│   "
		}
	}

	for i, child := range node.Children {
		isLastChild := i == len(node.Children)-1
		formatNode(sb, child, childPrefix, isLastChild, opts)
	}
}
