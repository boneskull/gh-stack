package github

import (
	"fmt"
	"strings"
	"testing"

	"github.com/boneskull/gh-stack/internal/tree"
)

func TestGenerateStackComment(t *testing.T) {
	// Build a test tree: main -> auth (#1) -> tests (#2) -> integration (#3)
	root := &tree.Node{Name: "main"}
	auth := &tree.Node{Name: "feature-auth", PR: 1, Parent: root}
	tests := &tree.Node{Name: "feature-auth-tests", PR: 2, Parent: auth}
	integration := &tree.Node{Name: "feature-auth-integration", PR: 3, Parent: tests}

	root.Children = []*tree.Node{auth}
	auth.Children = []*tree.Node{tests}
	tests.Children = []*tree.Node{integration}

	t.Run("middle of stack shows warning", func(t *testing.T) {
		comment := GenerateStackComment(root, "feature-auth-tests", "main")

		if !strings.Contains(comment, StackCommentMarker) {
			t.Error("missing stack comment marker")
		}
		if !strings.Contains(comment, "[!WARNING]") {
			t.Error("middle-of-stack PR should have warning")
		}
		if !strings.Contains(comment, "feature-auth-tests") {
			t.Error("should mention current branch")
		}
		if !strings.Contains(comment, "#1") {
			t.Error("should link to parent PR")
		}
		if !strings.Contains(comment, "#2") {
			t.Error("should link to current PR")
		}
		if !strings.Contains(comment, "#3") {
			t.Error("should link to child PR")
		}
	})

	t.Run("top of stack has no warning", func(t *testing.T) {
		// Create tree where auth targets main directly
		simpleRoot := &tree.Node{Name: "main"}
		simpleAuth := &tree.Node{Name: "feature-auth", PR: 1, Parent: simpleRoot}
		simpleRoot.Children = []*tree.Node{simpleAuth}

		comment := GenerateStackComment(simpleRoot, "feature-auth", "main")

		if strings.Contains(comment, "[!WARNING]") {
			t.Error("top-of-stack PR should not have warning")
		}
	})

	t.Run("current PR is highlighted", func(t *testing.T) {
		comment := GenerateStackComment(root, "feature-auth-tests", "main")

		// The current PR should have the ← indicator
		if !strings.Contains(comment, "←") {
			t.Error("current PR should be highlighted with ←")
		}
	})
}

func TestGenerateStackComment_EdgeCases(t *testing.T) {
	t.Run("single PR targeting trunk", func(t *testing.T) {
		root := &tree.Node{Name: "main"}
		feature := &tree.Node{Name: "feature", PR: 1, Parent: root}
		root.Children = []*tree.Node{feature}

		comment := GenerateStackComment(root, "feature", "main")

		if strings.Contains(comment, "[!WARNING]") {
			t.Error("single PR targeting trunk should not have warning")
		}
		if !strings.Contains(comment, "← #1 (this PR)") {
			t.Error("should highlight current PR")
		}
	})

	t.Run("deeply nested stack", func(t *testing.T) {
		root := &tree.Node{Name: "main"}
		prev := root
		for i := 1; i <= 5; i++ {
			node := &tree.Node{
				Name:   fmt.Sprintf("level-%d", i),
				PR:     i,
				Parent: prev,
			}
			prev.Children = []*tree.Node{node}
			prev = node
		}

		comment := GenerateStackComment(root, "level-3", "main")

		// Should show all levels
		for i := 1; i <= 5; i++ {
			if !strings.Contains(comment, fmt.Sprintf("#%d", i)) {
				t.Errorf("should contain PR #%d", i)
			}
		}
	})

	t.Run("branch with siblings", func(t *testing.T) {
		root := &tree.Node{Name: "main"}
		a := &tree.Node{Name: "feature-a", PR: 1, Parent: root}
		b := &tree.Node{Name: "feature-b", PR: 2, Parent: root}
		root.Children = []*tree.Node{a, b}

		comment := GenerateStackComment(root, "feature-a", "main")

		if !strings.Contains(comment, "feature-a") {
			t.Error("should contain current branch")
		}
		if !strings.Contains(comment, "feature-b") {
			t.Error("should contain sibling branch")
		}
	})

	t.Run("branch not found returns empty", func(t *testing.T) {
		root := &tree.Node{Name: "main"}

		comment := GenerateStackComment(root, "nonexistent", "main")

		if comment != "" {
			t.Error("should return empty for nonexistent branch")
		}
	})
}
