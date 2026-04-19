// cmd/submit_internal_test.go
//
// This file uses package cmd (not cmd_test) to unit-test unexported helpers
// like unwrapParagraphs, isBlockElement, etc. that are pure functions with no
// dependency on command wiring. The external test files (package cmd_test)
// cover command-level integration behavior.
package cmd

import (
	"errors"
	"fmt"
	"os/exec"
	"testing"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/git"
	"github.com/boneskull/gh-stack/internal/github"
	"github.com/boneskull/gh-stack/internal/style"
	"github.com/boneskull/gh-stack/internal/tree"
)

func TestUnwrapParagraphs(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "empty string",
			in:   "",
			want: "",
		},
		{
			name: "single line",
			in:   "Hello world",
			want: "Hello world",
		},
		{
			name: "hard-wrapped paragraph becomes single line",
			in:   "This is a paragraph that was\nhard-wrapped at around 72 columns\nfor the commit message.",
			want: "This is a paragraph that was hard-wrapped at around 72 columns for the commit message.",
		},
		{
			name: "blank lines preserved as paragraph breaks",
			in:   "First paragraph that is\nhard-wrapped.\n\nSecond paragraph also\nhard-wrapped.",
			want: "First paragraph that is hard-wrapped.\n\nSecond paragraph also hard-wrapped.",
		},
		{
			name: "fenced code block preserved verbatim",
			in:   "Before code:\n\n```go\nfunc main() {\n    fmt.Println(\"hello\")\n}\n```\n\nAfter code.",
			want: "Before code:\n\n```go\nfunc main() {\n    fmt.Println(\"hello\")\n}\n```\n\nAfter code.",
		},
		{
			name: "tilde fenced code block preserved",
			in:   "Text before.\n\n~~~\ncode here\n~~~\n\nText after.",
			want: "Text before.\n\n~~~\ncode here\n~~~\n\nText after.",
		},
		{
			name: "indented code block preserved",
			in:   "Some text.\n\n    indented code line 1\n    indented code line 2\n\nMore text.",
			want: "Some text.\n\n    indented code line 1\n    indented code line 2\n\nMore text.",
		},
		{
			name: "list continuation with indent is joined",
			in:   "Changes:\n\n- First item\n- Second item that is\n  also long\n- Third item",
			want: "Changes:\n\n- First item\n- Second item that is also long\n- Third item",
		},
		{
			name: "list continuation without indent is joined",
			in:   "Changes:\n\n- First item\n- Second item that is\nhard-wrapped here\n- Third item",
			want: "Changes:\n\n- First item\n- Second item that is hard-wrapped here\n- Third item",
		},
		{
			name: "ordered list items preserved",
			in:   "Steps:\n\n1. First step\n2. Second step\n3. Third step",
			want: "Steps:\n\n1. First step\n2. Second step\n3. Third step",
		},
		{
			name: "hard-wrapped ordered list item is joined",
			in:   "Steps:\n\n1. First step that is\nhard-wrapped here\n2. Second step",
			want: "Steps:\n\n1. First step that is hard-wrapped here\n2. Second step",
		},
		{
			name: "nested list items preserved",
			in:   "- Item 1\n  - Nested item\n  - Another nested\n- Item 2",
			want: "- Item 1\n  - Nested item\n  - Another nested\n- Item 2",
		},
		{
			name: "hard-wrapped nested list item is joined",
			in:   "- Item 1\n  - Nested item that is\n    also long\n- Item 2",
			want: "- Item 1\n  - Nested item that is also long\n- Item 2",
		},
		{
			name: "headers preserved",
			in:   "## Section\n\nParagraph that is\nhard-wrapped here.\n\n### Subsection\n\nAnother para.",
			want: "## Section\n\nParagraph that is hard-wrapped here.\n\n### Subsection\n\nAnother para.",
		},
		{
			name: "blockquotes preserved",
			in:   "> This is a quote\n> that spans lines\n\nRegular text.",
			want: "> This is a quote\n> that spans lines\n\nRegular text.",
		},
		{
			name: "horizontal rule preserved",
			in:   "Above\n\n---\n\nBelow",
			want: "Above\n\n---\n\nBelow",
		},
		{
			name: "realistic commit message body",
			in:   "This commit refactors the authentication middleware to\nuse JWT tokens instead of session cookies. The change\nimproves scalability by removing server-side session\nstorage requirements.\n\nKey changes:\n\n- Replace session middleware with JWT validation\n- Add token refresh endpoint\n- Update tests to use new auth flow\n\nBreaking change: clients must now send an\n`Authorization: Bearer <token>` header instead of\nrelying on cookies.",
			want: "This commit refactors the authentication middleware to use JWT tokens instead of session cookies. The change improves scalability by removing server-side session storage requirements.\n\nKey changes:\n\n- Replace session middleware with JWT validation\n- Add token refresh endpoint\n- Update tests to use new auth flow\n\nBreaking change: clients must now send an `Authorization: Bearer <token>` header instead of relying on cookies.",
		},
		{
			name: "pipe tables preserved",
			in:   "Results:\n\n| Name | Value |\n|------|-------|\n| foo  | 42    |",
			want: "Results:\n\n| Name | Value |\n|------|-------|\n| foo  | 42    |",
		},
		{
			name: "trailing whitespace on wrapped lines is trimmed",
			in:   "Line one with trailing space   \nline two.",
			want: "Line one with trailing space line two.",
		},
		{
			name: "HTML tags cause bail-out",
			in:   "Some text that is\nhard-wrapped.\n\n<details>\n<summary>Click me</summary>\nHidden content\n</details>",
			want: "Some text that is\nhard-wrapped.\n\n<details>\n<summary>Click me</summary>\nHidden content\n</details>",
		},
		{
			name: "inline HTML tag causes bail-out",
			in:   "This has a <br/> in it\nand wraps.",
			want: "This has a <br/> in it\nand wraps.",
		},
		{
			name: "angle bracket in non-HTML context still unwraps",
			in:   "The value x < y is\nalways true.",
			want: "The value x < y is always true.",
		},
		{
			name: "HTML inside fenced code block does not trigger bail-out",
			in:   "This adds a component.\n\n```html\n<div class=\"wrapper\">\n  <span>hello</span>\n</div>\n```\n\nThe paragraph is\nhard-wrapped here.",
			want: "This adds a component.\n\n```html\n<div class=\"wrapper\">\n  <span>hello</span>\n</div>\n```\n\nThe paragraph is hard-wrapped here.",
		},
		{
			name: "HTML inside indented code block does not trigger bail-out",
			in:   "Example:\n\n    <div>indented html</div>\n\nMore text that is\nhard-wrapped.",
			want: "Example:\n\n    <div>indented html</div>\n\nMore text that is hard-wrapped.",
		},
		{
			name: "HTML in prose with code block HTML still bails out",
			in:   "Use the <details> tag.\n\n```html\n<div>code</div>\n```",
			want: "Use the <details> tag.\n\n```html\n<div>code</div>\n```",
		},
		{
			name: "mismatched fence markers do not close each other",
			in:   "Text before.\n\n```\n~~~\nstill in code\n```\n\nParagraph that is\nhard-wrapped.",
			want: "Text before.\n\n```\n~~~\nstill in code\n```\n\nParagraph that is hard-wrapped.",
		},
		{
			name: "tilde fence with backticks inside",
			in:   "Text.\n\n~~~\n```\nnested marker\n~~~\n\nWrapped line\ncontinues here.",
			want: "Text.\n\n~~~\n```\nnested marker\n~~~\n\nWrapped line continues here.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := unwrapParagraphs(tt.in)
			if got != tt.want {
				t.Errorf("unwrapParagraphs() mismatch\n got: %q\nwant: %q", got, tt.want)
			}
		})
	}
}

func TestContainsHTMLOutsideCode(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"no HTML", "just plain text", false},
		{"HTML in prose", "Use <div> here", true},
		{"HTML in fenced code block", "```\n<div>hi</div>\n```", false},
		{"HTML in indented code block", "    <div>hi</div>", false},
		{"HTML in inline code", "Use `<div>` for this", false},
		{"HTML in prose AND code block", "<br/>\n\n```\n<div>x</div>\n```", true},
		{"angle bracket not HTML", "x < y", false},
		{"hyphenated custom element", "Use <my-component> here", true},
		{"namespaced XML tag", "The <xml:tag> element", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsHTMLOutsideCode(tt.in)
			if got != tt.want {
				t.Errorf("containsHTMLOutsideCode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsListItem(t *testing.T) {
	listLines := []string{
		"- item",
		"* item",
		"+ item",
		"-",
		"*",
		"+",
		"1. ordered",
		"12. multi-digit",
		"  - indented unordered",
		"  * indented star",
		"  1. indented ordered",
		"\t- tab indented",
	}
	for _, line := range listLines {
		if !isListItem(line) {
			t.Errorf("expected isListItem(%q) = true", line)
		}
	}

	nonListLines := []string{
		"just text",
		"# Header",
		"> blockquote",
		"| table",
		"2nd place finish",
		"",
	}
	for _, line := range nonListLines {
		if isListItem(line) {
			t.Errorf("expected isListItem(%q) = false", line)
		}
	}
}

func TestIsBlockElement(t *testing.T) {
	blockLines := []string{
		"# Header",
		"## Header 2",
		"- list item",
		"* list item",
		"+ list item",
		"1. ordered",
		"12. multi-digit ordered",
		"> blockquote",
		"| table row",
	}
	for _, line := range blockLines {
		if !isBlockElement(line) {
			t.Errorf("expected isBlockElement(%q) = true", line)
		}
	}

	nonBlockLines := []string{
		"just text",
		"This starts a sentence.",
		"2nd place finish",
	}
	for _, line := range nonBlockLines {
		if isBlockElement(line) {
			t.Errorf("expected isBlockElement(%q) = false", line)
		}
	}
}

func TestIsHorizontalRule(t *testing.T) {
	rules := []string{"---", "***", "___", "- - -", "* * *", "----", "****"}
	for _, r := range rules {
		if !isHorizontalRule(r) {
			t.Errorf("expected isHorizontalRule(%q) = true", r)
		}
	}

	nonRules := []string{"--", "**", "-", "abc", "---x"}
	for _, r := range nonRules {
		if isHorizontalRule(r) {
			t.Errorf("expected isHorizontalRule(%q) = false", r)
		}
	}
}

func TestIsBaseBranchInvalidError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "unrelated error",
			err:  errors.New("network timeout"),
			want: false,
		},
		{
			name: "exact GitHub 422 error",
			err:  errors.New("failed to create PR: HTTP 422: Validation Failed (https://api.github.com/repos/owner/repo/pulls)\nPullRequest.base is invalid"),
			want: true,
		},
		{
			name: "short form",
			err:  errors.New("base is invalid"),
			want: true,
		},
		{
			name: "wrapped error",
			err:  fmt.Errorf("something went wrong: %w", errors.New("PullRequest.base is invalid")),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isBaseBranchInvalidError(tt.err)
			if got != tt.want {
				t.Errorf("isBaseBranchInvalidError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func setupTestRepo(t *testing.T) *config.Config {
	t.Helper()
	dir := t.TempDir()
	if err := exec.Command("git", "init", dir).Run(); err != nil {
		t.Fatalf("git init failed: %v", err)
	}
	exec.Command("git", "-C", dir, "config", "user.email", "test@test.com").Run() //nolint:errcheck
	exec.Command("git", "-C", dir, "config", "user.name", "Test").Run()           //nolint:errcheck
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("config.Load failed: %v", err)
	}
	return cfg
}

// setupTestRepoWithDir is like setupTestRepo but also returns the directory path
// for callers that need to run git commands directly or construct a git.Git instance.
func setupTestRepoWithDir(t *testing.T) (*config.Config, string) {
	t.Helper()
	dir := t.TempDir()
	if err := exec.Command("git", "init", dir).Run(); err != nil {
		t.Fatalf("git init failed: %v", err)
	}
	exec.Command("git", "-C", dir, "config", "user.email", "test@test.com").Run() //nolint:errcheck
	exec.Command("git", "-C", dir, "config", "user.name", "Test").Run()           //nolint:errcheck
	// Create an initial commit so the repo has a HEAD and we can create branches.
	exec.Command("git", "-C", dir, "commit", "--allow-empty", "-m", "init").Run() //nolint:errcheck
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("config.Load failed: %v", err)
	}
	return cfg, dir
}

func TestIsTransitionToTrunk(t *testing.T) {
	trunk := "main"

	t.Run("no_stored_base_returns_true", func(t *testing.T) {
		cfg := setupTestRepo(t)
		// No stored base — first run after PR creation; should prompt.
		if !isTransitionToTrunk(cfg, "feat-a", trunk) {
			t.Error("expected true when no stored base exists")
		}
	})

	t.Run("stored_base_is_not_trunk_returns_true", func(t *testing.T) {
		cfg := setupTestRepo(t)
		// Branch previously targeted a non-trunk parent; now it targets trunk.
		if err := cfg.SetPRBase("feat-a", "feat-parent"); err != nil {
			t.Fatalf("SetPRBase failed: %v", err)
		}
		if !isTransitionToTrunk(cfg, "feat-a", trunk) {
			t.Error("expected true when stored base is not trunk")
		}
	})

	t.Run("stored_base_is_trunk_returns_false", func(t *testing.T) {
		cfg := setupTestRepo(t)
		// Branch was already targeting trunk on the previous run; don't re-prompt.
		if err := cfg.SetPRBase("feat-a", trunk); err != nil {
			t.Fatalf("SetPRBase failed: %v", err)
		}
		if isTransitionToTrunk(cfg, "feat-a", trunk) {
			t.Error("expected false when stored base is already trunk")
		}
	})

	t.Run("different_branches_are_independent", func(t *testing.T) {
		cfg := setupTestRepo(t)
		// feat-a already targeting trunk; feat-b has no stored base.
		if err := cfg.SetPRBase("feat-a", trunk); err != nil {
			t.Fatalf("SetPRBase failed: %v", err)
		}
		if isTransitionToTrunk(cfg, "feat-a", trunk) {
			t.Error("feat-a: expected false when stored base is trunk")
		}
		if !isTransitionToTrunk(cfg, "feat-b", trunk) {
			t.Error("feat-b: expected true when no stored base exists")
		}
	})

	t.Run("custom_trunk_name_works", func(t *testing.T) {
		cfg := setupTestRepo(t)
		customTrunk := "master"
		// Stored as "master"; should not prompt.
		if err := cfg.SetPRBase("feat-a", customTrunk); err != nil {
			t.Fatalf("SetPRBase failed: %v", err)
		}
		if isTransitionToTrunk(cfg, "feat-a", customTrunk) {
			t.Error("expected false with custom trunk name already stored")
		}
		// Stored as something else; should prompt.
		if err := cfg.SetPRBase("feat-b", "other-branch"); err != nil {
			t.Fatalf("SetPRBase failed: %v", err)
		}
		if !isTransitionToTrunk(cfg, "feat-b", customTrunk) {
			t.Error("expected true when stored base is not the custom trunk")
		}
	})
}

func TestApplyMustPushForSkippedAncestors(t *testing.T) {
	main := &tree.Node{Name: "main"}
	featA := &tree.Node{Name: "feat-a", Parent: main}
	featB := &tree.Node{Name: "feat-b", Parent: featA}

	t.Run("skipped_parent_pushed_when_child_gets_PR", func(t *testing.T) {
		decisions := []*prDecision{
			{node: featA, action: prActionSkip, skipReason: "user"},
			{node: featB, action: prActionCreate, title: "t", body: "b", draft: false},
		}
		applyMustPushForSkippedAncestors(decisions)
		if !decisions[0].pushAnyway {
			t.Error("feat-a should be marked pushAnyway when feat-b gets a PR")
		}
	})

	t.Run("skipped_branch_not_marked_when_no_descendant_PR", func(t *testing.T) {
		decisions := []*prDecision{
			{node: featA, action: prActionSkip, skipReason: "user"},
			{node: featB, action: prActionSkip, skipReason: "user"},
		}
		applyMustPushForSkippedAncestors(decisions)
		if decisions[0].pushAnyway || decisions[1].pushAnyway {
			t.Error("no pushAnyway when entire subtree is skipped")
		}
	})

	t.Run("skipped_parent_when_child_updates_PR", func(t *testing.T) {
		decisions := []*prDecision{
			{node: featA, action: prActionSkip, skipReason: "user"},
			{node: featB, action: prActionUpdate, prNum: 42},
		}
		applyMustPushForSkippedAncestors(decisions)
		if !decisions[0].pushAnyway {
			t.Error("feat-a should be pushAnyway when feat-b updates a PR")
		}
	})

	t.Run("skipped_parent_when_child_adopts_PR", func(t *testing.T) {
		decisions := []*prDecision{
			{node: featA, action: prActionSkip, skipReason: "user"},
			{node: featB, action: prActionAdopt, adoptPR: &github.PR{Number: 7}},
		}
		applyMustPushForSkippedAncestors(decisions)
		if !decisions[0].pushAnyway {
			t.Error("feat-a should be pushAnyway when feat-b adopts a PR")
		}
	})

	t.Run("three_level_chain_both_skipped_ancestors", func(t *testing.T) {
		featC := &tree.Node{Name: "feat-c", Parent: featB}
		decisions := []*prDecision{
			{node: featA, action: prActionSkip, skipReason: "user"},
			{node: featB, action: prActionSkip, skipReason: "user"},
			{node: featC, action: prActionCreate, title: "t", body: "b", draft: false},
		}
		applyMustPushForSkippedAncestors(decisions)
		if !decisions[0].pushAnyway || !decisions[1].pushAnyway {
			t.Errorf("both ancestors should be pushAnyway, got feat-a=%v feat-b=%v", decisions[0].pushAnyway, decisions[1].pushAnyway)
		}
	})

	t.Run("non_skipped_ancestor_not_given_pushAnyway_flag", func(t *testing.T) {
		decisions := []*prDecision{
			{node: featA, action: prActionUpdate, prNum: 1},
			{node: featB, action: prActionCreate, title: "t", body: "b", draft: false},
		}
		applyMustPushForSkippedAncestors(decisions)
		if decisions[0].pushAnyway {
			t.Error("feat-a is not skipped; pushAnyway should stay false")
		}
	})
}

func TestDeleteMergedBranchClearsPRBase(t *testing.T) {
	cfg, dir := setupTestRepoWithDir(t)
	g := git.New(dir)
	s := style.New()

	trunk, err := g.CurrentBranch()
	if err != nil {
		t.Fatalf("CurrentBranch failed: %v", err)
	}
	if err := cfg.SetTrunk(trunk); err != nil {
		t.Fatalf("SetTrunk failed: %v", err)
	}

	// Create feature-a with a commit so git can delete it later.
	if err := exec.Command("git", "-C", dir, "checkout", "-b", "feature-a").Run(); err != nil {
		t.Fatalf("create branch failed: %v", err)
	}
	if err := exec.Command("git", "-C", dir, "commit", "--allow-empty", "-m", "feat").Run(); err != nil {
		t.Fatalf("commit failed: %v", err)
	}
	if err := exec.Command("git", "-C", dir, "checkout", trunk).Run(); err != nil {
		t.Fatalf("checkout trunk failed: %v", err)
	}

	if err := cfg.SetParent("feature-a", trunk); err != nil {
		t.Fatalf("SetParent failed: %v", err)
	}
	if err := cfg.SetPR("feature-a", 10); err != nil {
		t.Fatalf("SetPR failed: %v", err)
	}
	if err := cfg.SetPRBase("feature-a", trunk); err != nil {
		t.Fatalf("SetPRBase failed: %v", err)
	}

	currentBranch := trunk
	deleteMergedBranch(g, cfg, "feature-a", trunk, &currentBranch, s)

	if v, err := cfg.GetPRBase("feature-a"); err == nil {
		t.Errorf("expected stackPRBase to be removed after deleteMergedBranch, got %q", v)
	}
	if v, err := cfg.GetPR("feature-a"); err == nil {
		t.Errorf("expected stackPR to be removed after deleteMergedBranch, got %d", v)
	}
}

func TestOrphanMergedBranchClearsPRBase(t *testing.T) {
	cfg, dir := setupTestRepoWithDir(t)
	g := git.New(dir)
	s := style.New()

	trunk, err := g.CurrentBranch()
	if err != nil {
		t.Fatalf("CurrentBranch failed: %v", err)
	}
	if err := cfg.SetTrunk(trunk); err != nil {
		t.Fatalf("SetTrunk failed: %v", err)
	}

	if err := cfg.SetParent("feature-a", trunk); err != nil {
		t.Fatalf("SetParent failed: %v", err)
	}
	if err := cfg.SetPR("feature-a", 11); err != nil {
		t.Fatalf("SetPR failed: %v", err)
	}
	if err := cfg.SetPRBase("feature-a", trunk); err != nil {
		t.Fatalf("SetPRBase failed: %v", err)
	}

	orphanMergedBranch(cfg, "feature-a", s)

	if v, err := cfg.GetPRBase("feature-a"); err == nil {
		t.Errorf("expected stackPRBase to be removed after orphanMergedBranch, got %q", v)
	}
	if v, err := cfg.GetPR("feature-a"); err == nil {
		t.Errorf("expected stackPR to be removed after orphanMergedBranch, got %d", v)
	}
	if v, err := cfg.GetParent("feature-a"); err == nil {
		t.Errorf("expected stackParent to be removed after orphanMergedBranch, got %q", v)
	}
}
