# GitHub Stack Integration Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add GitHub integration features that display stack navigation comments on PRs and manage draft status based on stack position.

**Architecture:** Extend `internal/github` with comment and draft management methods. Add a new `internal/github/comments.go` for stack comment generation. Modify `cmd/pr.go` to create drafts when targeting non-trunk and post stack comments. Modify `cmd/sync.go` to update comments across the stack and prompt to undraft when PRs become top-of-stack.

**Tech Stack:** Go, go-gh REST API, Cobra CLI

---

## Task 1: Add Comment Types and Methods to GitHub Client

**Files:**
- Modify: `internal/github/github.go`

**Step 1: Write the failing test**

Create test file first:

```go
// internal/github/github_test.go
package github

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_CreateComment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/repos/owner/repo/issues/123/comments" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["body"] != "test comment" {
			t.Errorf("expected body 'test comment', got %s", body["body"])
		}

		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id": 456}`))
	}))
	defer server.Close()

	client := &Client{
		owner: "owner",
		repo:  "repo",
	}
	// Note: We can't easily test with real REST client, so this test verifies the contract
	// Real integration testing would require mocking go-gh
	t.Skip("requires REST client mocking - contract test only")
}
```

**Step 2: Run test to verify it skips**

Run: `go test -v -run TestClient_CreateComment ./internal/github/`
Expected: SKIP (test documents the contract)

**Step 3: Add Comment type and CreateComment method**

Add to `internal/github/github.go`:

```go
// Comment represents a GitHub issue/PR comment.
type Comment struct {
	ID   int    `json:"id"`
	Body string `json:"body"`
}

// CreateComment adds a comment to a PR (PRs are issues in GitHub's API).
func (c *Client) CreateComment(prNumber int, body string) (int, error) {
	path := fmt.Sprintf("repos/%s/%s/issues/%d/comments", c.owner, c.repo, prNumber)

	reqBody := map[string]string{"body": body}
	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		return 0, fmt.Errorf("marshal comment body: %w", err)
	}

	resp := &Comment{}
	if err := c.rest.Post(path, bytes.NewReader(reqBytes), resp); err != nil {
		return 0, fmt.Errorf("create comment on PR #%d: %w", prNumber, err)
	}

	return resp.ID, nil
}
```

**Step 4: Run existing tests to verify no regression**

Run: `go test -v ./internal/github/...`
Expected: PASS (or skip for mock-dependent tests)

**Step 5: Commit**

```
git add internal/github/github.go internal/github/github_test.go
git commit -m "feat(github): add CreateComment method

Adds Comment type and CreateComment method to post comments on PRs.
Uses the issues API endpoint since PRs are issues in GitHub.
"
```

---

## Task 2: Add ListComments and UpdateComment Methods

**Files:**
- Modify: `internal/github/github.go`

**Step 1: Add ListComments method**

Add to `internal/github/github.go`:

```go
// ListComments retrieves all comments on a PR.
func (c *Client) ListComments(prNumber int) ([]Comment, error) {
	path := fmt.Sprintf("repos/%s/%s/issues/%d/comments", c.owner, c.repo, prNumber)

	var comments []Comment
	if err := c.rest.Get(path, &comments); err != nil {
		return nil, fmt.Errorf("list comments on PR #%d: %w", prNumber, err)
	}

	return comments, nil
}
```

**Step 2: Add UpdateComment method**

Add to `internal/github/github.go`:

```go
// UpdateComment updates an existing comment by ID.
func (c *Client) UpdateComment(commentID int, body string) error {
	path := fmt.Sprintf("repos/%s/%s/issues/comments/%d", c.owner, c.repo, commentID)

	reqBody := map[string]string{"body": body}
	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal comment body: %w", err)
	}

	if err := c.rest.Patch(path, bytes.NewReader(reqBytes), nil); err != nil {
		return fmt.Errorf("update comment %d: %w", commentID, err)
	}

	return nil
}
```

**Step 3: Run tests**

Run: `go test -v ./internal/github/...`
Expected: PASS

**Step 4: Commit**

```
git add internal/github/github.go
git commit -m "feat(github): add ListComments and UpdateComment methods

ListComments retrieves all comments on a PR.
UpdateComment modifies an existing comment by ID.
Both methods needed for idempotent stack comment management.
"
```

---

## Task 3: Add Draft PR Support

**Files:**
- Modify: `internal/github/github.go`

**Step 1: Update PR struct to include Draft field**

Modify the `PR` struct in `internal/github/github.go`:

```go
// PR represents a GitHub pull request.
type PR struct {
	Number int    `json:"number"`
	State  string `json:"state"`
	Merged bool   `json:"merged"`
	Draft  bool   `json:"draft"`
	Base   struct {
		Ref string `json:"ref"`
	} `json:"base"`
}
```

**Step 2: Add CreateDraftPR method**

Add to `internal/github/github.go`:

```go
// CreateDraftPR creates a new pull request as a draft.
func (c *Client) CreateDraftPR(head, base, title, body string) (int, error) {
	path := fmt.Sprintf("repos/%s/%s/pulls", c.owner, c.repo)

	reqBody := map[string]any{
		"head":  head,
		"base":  base,
		"title": title,
		"body":  body,
		"draft": true,
	}
	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		return 0, fmt.Errorf("marshal PR body: %w", err)
	}

	resp := &PR{}
	if err := c.rest.Post(path, bytes.NewReader(reqBytes), resp); err != nil {
		return 0, fmt.Errorf("create draft PR: %w", err)
	}

	return resp.Number, nil
}
```

**Step 3: Add MarkPRReady method**

Add to `internal/github/github.go`:

```go
// MarkPRReady converts a draft PR to ready for review.
// Uses the GraphQL API since REST doesn't support this operation.
func (c *Client) MarkPRReady(prNumber int) error {
	// First get the PR's node_id for GraphQL
	path := fmt.Sprintf("repos/%s/%s/pulls/%d", c.owner, c.repo, prNumber)

	var prData struct {
		NodeID string `json:"node_id"`
	}
	if err := c.rest.Get(path, &prData); err != nil {
		return fmt.Errorf("get PR #%d: %w", prNumber, err)
	}

	// Use GraphQL mutation to mark ready
	query := `mutation($id: ID!) {
		markPullRequestReadyForReview(input: {pullRequestId: $id}) {
			pullRequest { number }
		}
	}`

	variables := map[string]any{"id": prData.NodeID}
	reqBody := map[string]any{
		"query":     query,
		"variables": variables,
	}
	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal GraphQL request: %w", err)
	}

	if err := c.rest.Post("graphql", bytes.NewReader(reqBytes), nil); err != nil {
		return fmt.Errorf("mark PR #%d ready: %w", prNumber, err)
	}

	return nil
}
```

**Step 4: Run tests and lint**

Run: `make lint && go test -v ./internal/github/...`
Expected: PASS

**Step 5: Commit**

```
git add internal/github/github.go
git commit -m "feat(github): add draft PR support

- Add Draft and Base fields to PR struct
- Add CreateDraftPR to create PRs as drafts
- Add MarkPRReady to convert draft to ready for review

MarkPRReady uses GraphQL since REST API doesn't support this.
"
```

---

## Task 4: Create Stack Comment Generator

**Files:**
- Create: `internal/github/comments.go`
- Create: `internal/github/comments_test.go`

**Step 1: Write the failing test for comment generation**

Create `internal/github/comments_test.go`:

```go
package github

import (
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
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run TestGenerateStackComment ./internal/github/`
Expected: FAIL with "undefined: GenerateStackComment"

**Step 3: Create comments.go with implementation**

Create `internal/github/comments.go`:

```go
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
```

**Step 4: Run tests**

Run: `go test -v -run TestGenerateStackComment ./internal/github/`
Expected: PASS

**Step 5: Commit**

```
git add internal/github/comments.go internal/github/comments_test.go
git commit -m "feat(github): add stack comment generator

- Add StackCommentMarker constant for identifying managed comments
- Add GenerateStackComment to build markdown with tree visualization
- Add FindStackComment to locate existing managed comments
- Add CreateOrUpdateStackComment for idempotent comment management

Comments show warning when PR targets non-trunk branch.
Current PR is highlighted with ← indicator.
"
```

---

## Task 5: Integrate Stack Comments into PR Creation

**Files:**
- Modify: `cmd/pr.go`

**Step 1: Read current pr.go to understand structure**

Run: Read `cmd/pr.go` to see current implementation

**Step 2: Modify runPR to create draft and post comment**

The key changes to `cmd/pr.go`:

1. After creating PR, build tree and generate comment
2. If base != trunk, create as draft instead of regular PR

```go
// In runPR function, replace the CreatePR call with logic like:

// Determine if this should be a draft (targets non-trunk)
trunk, err := cfg.GetTrunk()
if err != nil {
	return err
}

var prNum int
if base != trunk {
	// Create as draft since it's part of a stack
	prNum, err = gh.CreateDraftPR(branch, base, title, body)
	if err != nil {
		return err
	}
	fmt.Printf("Created draft PR #%d for %s -> %s\n", prNum, branch, base)
} else {
	prNum, err = gh.CreatePR(branch, base, title, body)
	if err != nil {
		return err
	}
	fmt.Printf("Created PR #%d for %s -> %s\n", prNum, branch, base)
}

// Store PR number in config
if err := cfg.SetPR(branch, prNum); err != nil {
	return err
}

// Post stack navigation comment
root, err := tree.Build(cfg)
if err != nil {
	return fmt.Errorf("build tree: %w", err)
}

comment := github.GenerateStackComment(root, branch, trunk)
if comment != "" {
	if err := gh.CreateOrUpdateStackComment(prNum, comment); err != nil {
		fmt.Printf("Warning: failed to add stack comment: %v\n", err)
		// Don't fail the command for comment issues
	}
}
```

**Step 3: Run tests and lint**

Run: `make lint && go test -v ./cmd/...`
Expected: PASS

**Step 4: Commit**

```
git add cmd/pr.go
git commit -m "feat(pr): create drafts for stacked PRs and post comments

- PRs targeting non-trunk branches are created as drafts
- Stack navigation comment posted after PR creation
- Comment shows tree structure with links to related PRs
- Warning displayed for PRs that shouldn't be merged yet
"
```

---

## Task 6: Update Stack Comments in Sync Command

**Files:**
- Modify: `cmd/sync.go`

**Step 1: Read current sync.go**

Run: Read `cmd/sync.go` to understand the sync flow

**Step 2: Add function to update all stack comments**

Add helper function and integrate into sync:

```go
// updateStackComments updates the navigation comment on all PRs in the stack.
func updateStackComments(cfg *config.Config, gh *github.Client) error {
	trunk, err := cfg.GetTrunk()
	if err != nil {
		return err
	}

	root, err := tree.Build(cfg)
	if err != nil {
		return err
	}

	// Walk tree and update each PR's comment
	return walkTreeAndUpdateComments(root, root, trunk, gh)
}

func walkTreeAndUpdateComments(node, root *tree.Node, trunk string, gh *github.Client) error {
	if node.PR > 0 {
		comment := github.GenerateStackComment(root, node.Name, trunk)
		if comment != "" {
			if err := gh.CreateOrUpdateStackComment(node.PR, comment); err != nil {
				fmt.Printf("Warning: failed to update comment on PR #%d: %v\n", node.PR, err)
				// Continue with other PRs
			}
		}
	}

	for _, child := range node.Children {
		if err := walkTreeAndUpdateComments(child, root, trunk, gh); err != nil {
			return err
		}
	}

	return nil
}
```

**Step 3: Add prompt to undraft PRs that now target trunk**

After retargeting orphaned PRs in sync, check if they now target trunk and prompt:

```go
// After retargeting a PR to trunk:
if newBase == trunk {
	pr, err := gh.GetPR(prNum)
	if err == nil && pr.Draft {
		fmt.Printf("PR #%d (%s) now targets %s.\n", prNum, branch, trunk)
		fmt.Print("Mark as ready for review? [y/N]: ")

		var response string
		fmt.Scanln(&response)

		if strings.ToLower(strings.TrimSpace(response)) == "y" {
			if err := gh.MarkPRReady(prNum); err != nil {
				fmt.Printf("Warning: failed to mark PR ready: %v\n", err)
			} else {
				fmt.Printf("PR #%d marked as ready for review.\n", prNum)
			}
		}
	}
}
```

**Step 4: Call updateStackComments at end of sync**

Add at end of runSync:

```go
// Update stack comments on all PRs
fmt.Println("Updating stack comments...")
if err := updateStackComments(cfg, gh); err != nil {
	fmt.Printf("Warning: failed to update some comments: %v\n", err)
}
```

**Step 5: Run tests and lint**

Run: `make lint && go test -v ./cmd/...`
Expected: PASS

**Step 6: Commit**

```
git add cmd/sync.go
git commit -m "feat(sync): update stack comments and manage draft status

- Update navigation comments on all PRs after sync
- Prompt to mark PRs ready when they become top-of-stack
- Comments reflect new tree structure after parent merges
- Warning removed from PRs that now target trunk
"
```

---

## Task 7: Add Tests for Comment Generation Edge Cases

**Files:**
- Modify: `internal/github/comments_test.go`

**Step 1: Add edge case tests**

Add to `internal/github/comments_test.go`:

```go
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
```

**Step 2: Run tests**

Run: `go test -v -run TestGenerateStackComment ./internal/github/`
Expected: PASS

**Step 3: Commit**

```
git add internal/github/comments_test.go
git commit -m "test(github): add edge case tests for comment generation

Tests cover:
- Single PR targeting trunk (no warning)
- Deeply nested stacks (5 levels)
- Branches with siblings
- Nonexistent branch handling
"
```

---

## Task 8: Manual Integration Testing

**Files:** None (testing only)

**Step 1: Build the binary**

Run: `make build`
Expected: Binary built at `./gh-stack`

**Step 2: Create a test stack**

```bash
# In a test repo with gh-stack initialized
git checkout -b feature-auth
# make changes
git commit -m "feat: add auth"
./gh-stack track feature-auth

git checkout -b feature-auth-tests
# make changes
git commit -m "test: add auth tests"
./gh-stack track feature-auth-tests

git checkout -b feature-auth-integration
# make changes
git commit -m "feat: add auth integration"
./gh-stack track feature-auth-integration
```

**Step 3: Create PRs and verify comments**

```bash
git checkout feature-auth
./gh-stack pr -t "Add authentication"
# Verify: PR created (not draft since targets main)
# Verify: Stack comment appears on PR

git checkout feature-auth-tests
./gh-stack pr -t "Add auth tests"
# Verify: PR created as DRAFT
# Verify: Stack comment has WARNING
# Verify: Tree shows all 3 branches

git checkout feature-auth-integration
./gh-stack pr -t "Add auth integration"
# Verify: PR created as DRAFT
# Verify: Stack comment has WARNING
```

**Step 4: Merge bottom PR and sync**

```bash
# Merge feature-auth PR on GitHub (or simulate by merging locally)
git checkout main
git merge feature-auth
git push

./gh-stack sync
# Verify: feature-auth-tests PR retargeted to main
# Verify: Prompted to mark as ready for review
# Verify: Stack comments updated on all PRs
# Verify: WARNING removed from feature-auth-tests
```

**Step 5: Document any issues found**

Create issues for any bugs discovered during manual testing.

---

## Summary

| Task | Description | Files |
|------|-------------|-------|
| 1 | Add CreateComment method | `internal/github/github.go` |
| 2 | Add ListComments and UpdateComment | `internal/github/github.go` |
| 3 | Add draft PR support | `internal/github/github.go` |
| 4 | Create stack comment generator | `internal/github/comments.go`, `internal/github/comments_test.go` |
| 5 | Integrate into PR creation | `cmd/pr.go` |
| 6 | Update comments in sync | `cmd/sync.go` |
| 7 | Add edge case tests | `internal/github/comments_test.go` |
| 8 | Manual integration testing | N/A |

Total: 8 tasks, estimated 7-8 commits
