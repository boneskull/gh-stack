package github

import (
	"fmt"
	"strings"
	"testing"

	"github.com/boneskull/gh-stack/internal/tree"
)

const testRepoURL = "https://github.com/test/repo"

// Helper to create PR info map for tests
func makePRInfo(prs ...struct {
	num   int
	title string
}) map[int]PRInfo {
	info := make(map[int]PRInfo)
	for _, pr := range prs {
		info[pr.num] = PRInfo{Number: pr.num, Title: pr.title}
	}
	return info
}

func TestGenerateStackComment(t *testing.T) {
	// Build a test tree: main -> auth (#1) -> tests (#2) -> integration (#3)
	root := &tree.Node{Name: "main"}
	auth := &tree.Node{Name: "feature-auth", PR: 1, Parent: root}
	tests := &tree.Node{Name: "feature-auth-tests", PR: 2, Parent: auth}
	integration := &tree.Node{Name: "feature-auth-integration", PR: 3, Parent: tests}

	root.Children = []*tree.Node{auth}
	auth.Children = []*tree.Node{tests}
	tests.Children = []*tree.Node{integration}

	prInfo := makePRInfo(
		struct {
			num   int
			title string
		}{1, "Add authentication"},
		struct {
			num   int
			title string
		}{2, "Add auth tests"},
		struct {
			num   int
			title string
		}{3, "Add integration tests"},
	)

	t.Run("middle of stack shows warning", func(t *testing.T) {
		comment := GenerateStackComment(root, "feature-auth-tests", "main", testRepoURL, prInfo)

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

		comment := GenerateStackComment(simpleRoot, "feature-auth", "main", testRepoURL, prInfo)

		if strings.Contains(comment, "[!WARNING]") {
			t.Error("top-of-stack PR should not have warning")
		}
	})

	t.Run("current PR is highlighted", func(t *testing.T) {
		comment := GenerateStackComment(root, "feature-auth-tests", "main", testRepoURL, prInfo)

		// The current PR should have "(this PR)" text
		if !strings.Contains(comment, "*(this PR)*") {
			t.Error("current PR should be highlighted with '(this PR)'")
		}
	})

	t.Run("PR links include title and number", func(t *testing.T) {
		comment := GenerateStackComment(root, "feature-auth-tests", "main", testRepoURL, prInfo)

		// Check that PR links include title
		if !strings.Contains(comment, "[Add authentication #1](https://github.com/test/repo/pull/1)") {
			t.Error("should contain link with title for PR #1")
		}
		if !strings.Contains(comment, "[Add auth tests #2](https://github.com/test/repo/pull/2)") {
			t.Error("should contain link with title for PR #2")
		}
		if !strings.Contains(comment, "[Add integration tests #3](https://github.com/test/repo/pull/3)") {
			t.Error("should contain link with title for PR #3")
		}
	})

	t.Run("branch names shown in backticks", func(t *testing.T) {
		comment := GenerateStackComment(root, "feature-auth-tests", "main", testRepoURL, prInfo)

		if !strings.Contains(comment, "branch: `feature-auth`") {
			t.Error("should show branch name in backticks")
		}
		if !strings.Contains(comment, "branch: `feature-auth-tests`") {
			t.Error("should show current branch name in backticks")
		}
	})

	t.Run("uses nested markdown lists", func(t *testing.T) {
		comment := GenerateStackComment(root, "feature-auth-tests", "main", testRepoURL, prInfo)

		// Should use markdown list format, not code blocks
		if strings.Contains(comment, "```") {
			t.Error("should not use code blocks")
		}
		// Trunk has no PR, shown with "branch:" prefix
		if !strings.Contains(comment, "- branch: `main`") {
			t.Error("should use markdown list format with 'branch:' prefix for trunk")
		}
	})

	t.Run("fallback when no title available", func(t *testing.T) {
		// No PR info provided
		comment := GenerateStackComment(root, "feature-auth-tests", "main", testRepoURL, nil)

		// Should fallback to just "#N" format (no title)
		if !strings.Contains(comment, "[#1](https://github.com/test/repo/pull/1)") {
			t.Error("should fallback to '#N' when title not available")
		}
	})

	t.Run("no-PR branches show branch prefix", func(t *testing.T) {
		// Branch without a PR in the middle of a stack
		noPRRoot := &tree.Node{Name: "main"}
		noPRMiddle := &tree.Node{Name: "wip-branch", PR: 0, Parent: noPRRoot}
		noPRChild := &tree.Node{Name: "child-branch", PR: 5, Parent: noPRMiddle}

		noPRRoot.Children = []*tree.Node{noPRMiddle}
		noPRMiddle.Children = []*tree.Node{noPRChild}

		childPRInfo := makePRInfo(struct {
			num   int
			title string
		}{5, "Child feature"})
		comment := GenerateStackComment(noPRRoot, "child-branch", "main", testRepoURL, childPRInfo)

		// Trunk and middle branch both have no PR; both should show "branch:" prefix
		if !strings.Contains(comment, "- branch: `main`") {
			t.Error("trunk without PR should show 'branch:' prefix")
		}
		if !strings.Contains(comment, "- branch: `wip-branch`") {
			t.Error("branch without PR should show 'branch:' prefix")
		}
	})
}

func TestGenerateStackComment_EdgeCases(t *testing.T) {
	t.Run("single PR targeting trunk", func(t *testing.T) {
		root := &tree.Node{Name: "main"}
		feature := &tree.Node{Name: "feature", PR: 1, Parent: root}
		root.Children = []*tree.Node{feature}

		prInfo := makePRInfo(struct {
			num   int
			title string
		}{1, "My feature"})
		comment := GenerateStackComment(root, "feature", "main", testRepoURL, prInfo)

		if strings.Contains(comment, "[!WARNING]") {
			t.Error("single PR targeting trunk should not have warning")
		}
		if !strings.Contains(comment, "**[My feature #1](https://github.com/test/repo/pull/1)**") {
			t.Error("should highlight current PR with bold link")
		}
		if !strings.Contains(comment, "*(this PR)*") {
			t.Error("should have '(this PR)' marker")
		}
	})

	t.Run("deeply nested stack", func(t *testing.T) {
		root := &tree.Node{Name: "main"}
		prev := root
		prInfo := make(map[int]PRInfo)
		for i := 1; i <= 5; i++ {
			node := &tree.Node{
				Name:   fmt.Sprintf("level-%d", i),
				PR:     i,
				Parent: prev,
			}
			prev.Children = []*tree.Node{node}
			prev = node
			prInfo[i] = PRInfo{Number: i, Title: fmt.Sprintf("Level %d feature", i)}
		}

		comment := GenerateStackComment(root, "level-3", "main", testRepoURL, prInfo)

		// Should show all levels with links
		for i := 1; i <= 5; i++ {
			expectedLink := fmt.Sprintf("[Level %d feature #%d](https://github.com/test/repo/pull/%d)", i, i, i)
			if !strings.Contains(comment, expectedLink) {
				t.Errorf("should contain link to PR #%d", i)
			}
		}
	})

	t.Run("branch with siblings only shows current stack", func(t *testing.T) {
		root := &tree.Node{Name: "main"}
		a := &tree.Node{Name: "feature-a", PR: 1, Parent: root}
		b := &tree.Node{Name: "feature-b", PR: 2, Parent: root}
		root.Children = []*tree.Node{a, b}

		prInfo := makePRInfo(
			struct {
				num   int
				title string
			}{1, "Feature A"},
			struct {
				num   int
				title string
			}{2, "Feature B"},
		)
		comment := GenerateStackComment(root, "feature-a", "main", testRepoURL, prInfo)

		if !strings.Contains(comment, "feature-a") {
			t.Error("should contain current branch")
		}
		if strings.Contains(comment, "feature-b") {
			t.Error("should NOT contain sibling branch from a different stack")
		}
	})

	t.Run("multiple stacks only shows current one", func(t *testing.T) {
		// Tree: main -> {stack1-base (#1) -> stack1-child (#2), unrelated (#3)}
		root := &tree.Node{Name: "main"}
		stack1Base := &tree.Node{Name: "stack1-base", PR: 1, Parent: root}
		stack1Child := &tree.Node{Name: "stack1-child", PR: 2, Parent: stack1Base}
		unrelated := &tree.Node{Name: "unrelated", PR: 3, Parent: root}

		root.Children = []*tree.Node{stack1Base, unrelated}
		stack1Base.Children = []*tree.Node{stack1Child}

		prInfo := makePRInfo(
			struct {
				num   int
				title string
			}{1, "Stack 1 base"},
			struct {
				num   int
				title string
			}{2, "Stack 1 child"},
			struct {
				num   int
				title string
			}{3, "Unrelated feature"},
		)

		// Viewing the child PR - should see stack1-base and stack1-child, but NOT unrelated
		comment := GenerateStackComment(root, "stack1-child", "main", testRepoURL, prInfo)

		if !strings.Contains(comment, "stack1-base") {
			t.Error("should contain ancestor branch in the current stack")
		}
		if !strings.Contains(comment, "stack1-child") {
			t.Error("should contain current branch")
		}
		if strings.Contains(comment, "unrelated") {
			t.Error("should NOT contain branch from a different stack")
		}
	})

	t.Run("branch not found returns empty", func(t *testing.T) {
		root := &tree.Node{Name: "main"}

		comment := GenerateStackComment(root, "nonexistent", "main", testRepoURL, nil)

		if comment != "" {
			t.Error("should return empty for nonexistent branch")
		}
	})

	t.Run("current PR link is bolded", func(t *testing.T) {
		root := &tree.Node{Name: "main"}
		feature := &tree.Node{Name: "feature", PR: 1, Parent: root}
		root.Children = []*tree.Node{feature}

		prInfo := makePRInfo(struct {
			num   int
			title string
		}{1, "My feature"})
		comment := GenerateStackComment(root, "feature", "main", testRepoURL, prInfo)

		// The full link should be bolded for current PR
		if !strings.Contains(comment, "**[My feature #1]") {
			t.Error("current PR link should be bolded")
		}
	})
}

func TestCollectPRNumbers(t *testing.T) {
	root := &tree.Node{Name: "main"}
	a := &tree.Node{Name: "a", PR: 1, Parent: root}
	b := &tree.Node{Name: "b", PR: 2, Parent: a}
	c := &tree.Node{Name: "c", PR: 0, Parent: b} // no PR
	d := &tree.Node{Name: "d", PR: 3, Parent: c}

	root.Children = []*tree.Node{a}
	a.Children = []*tree.Node{b}
	b.Children = []*tree.Node{c}
	c.Children = []*tree.Node{d}

	numbers := CollectPRNumbers(root)

	if len(numbers) != 3 {
		t.Errorf("expected 3 PR numbers, got %d", len(numbers))
	}

	// Check all expected numbers are present
	found := make(map[int]bool)
	for _, n := range numbers {
		found[n] = true
	}
	for _, expected := range []int{1, 2, 3} {
		if !found[expected] {
			t.Errorf("expected PR #%d to be collected", expected)
		}
	}
}
