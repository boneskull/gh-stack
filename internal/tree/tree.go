// internal/tree/tree.go
package tree

import (
	"sort"

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
		pr, _ := cfg.GetPR(branch) // ignore error, 0 is fine
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
