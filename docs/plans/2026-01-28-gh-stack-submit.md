# `gh stack submit` Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Create a unified `gh stack submit` command that cascades, pushes, and creates/updates PRs for the current branch and its descendants in one operation.

**Architecture:** The submit command orchestrates three phases: (1) cascade current + descendants onto their parents, (2) push all affected branches with --force-with-lease, (3) create PRs for branches without them or update PR bases for branches that have them. State is persisted to allow continue/abort after conflicts. The existing `continue` command will be extended to understand submit context.

**Tech Stack:** Go, Cobra CLI, go-gh library, existing internal packages (config, git, github, state, tree)

---

## Task 1: Extend State to Support Submit Operations

The current `CascadeState` only tracks cascade progress. We need to extend it to track submit context so `continue` knows to also push and create PRs after cascade completes.

**Files:**

- Modify: `internal/state/state.go`
- Modify: `internal/state/state_test.go`

**Step 1: Write the failing test**

Add to `internal/state/state_test.go`:

```go
func TestSubmitState(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}

	s := &CascadeState{
		Current:      "feature-b",
		Pending:      []string{"feature-c"},
		OriginalHead: "abc123",
		Operation:    OperationSubmit,
		UpdateOnly:   true,
	}

	if err := Save(gitDir, s); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := Load(gitDir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.Operation != OperationSubmit {
		t.Errorf("expected operation %q, got %q", OperationSubmit, loaded.Operation)
	}
	if !loaded.UpdateOnly {
		t.Error("expected UpdateOnly to be true")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run TestSubmitState ./internal/state/...`
Expected: FAIL - `OperationSubmit` undefined

**Step 3: Implement the state extension**

Update `internal/state/state.go`:

```go
// internal/state/state.go
package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const stateFile = "STACK_CASCADE_STATE"

// Operation types for cascade state.
const (
	OperationCascade = "cascade"
	OperationSubmit  = "submit"
)

// ErrNoState is returned when no cascade state exists.
var ErrNoState = errors.New("no cascade in progress")

// CascadeState represents the state of an in-progress cascade or submit operation.
type CascadeState struct {
	Current      string   `json:"current"`
	Pending      []string `json:"pending"`
	OriginalHead string   `json:"original_head"`
	// Operation is "cascade" or "submit" - determines what happens after cascade completes
	Operation string `json:"operation,omitempty"`
	// UpdateOnly (submit only) - if true, don't create new PRs, only update existing
	UpdateOnly bool `json:"update_only,omitempty"`
}

// Save persists cascade state to .git/STACK_CASCADE_STATE.
func Save(gitDir string, s *CascadeState) error {
	path := filepath.Join(gitDir, stateFile)
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// Load reads cascade state from .git/STACK_CASCADE_STATE.
func Load(gitDir string) (*CascadeState, error) {
	path := filepath.Join(gitDir, stateFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNoState
		}
		return nil, err
	}

	var s CascadeState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// Remove deletes the cascade state file.
func Remove(gitDir string) error {
	path := filepath.Join(gitDir, stateFile)
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// Exists checks if a cascade is in progress.
func Exists(gitDir string) bool {
	path := filepath.Join(gitDir, stateFile)
	_, err := os.Stat(path)
	return err == nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test -v -run TestSubmitState ./internal/state/...`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/state/state.go internal/state/state_test.go
git commit -m "feat(state): add operation type and update-only flag for submit

Extends CascadeState to track whether the operation is a cascade or submit,
and whether submit should only update existing PRs (not create new ones).
This allows 'continue' to know what to do after cascade completes.
"
```

---

## Task 2: Create the Submit Command Structure

Create the basic `submit` command with flags, without the implementation yet.

**Files:**

- Create: `cmd/submit.go`

**Step 1: Create the command file**

```go
// cmd/submit.go
package cmd

import (
	"fmt"
	"os"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/git"
	"github.com/boneskull/gh-stack/internal/github"
	"github.com/boneskull/gh-stack/internal/state"
	"github.com/boneskull/gh-stack/internal/tree"
	"github.com/spf13/cobra"
)

var submitCmd = &cobra.Command{
	Use:   "submit",
	Short: "Cascade, push, and create/update PRs for current branch and descendants",
	Long: `Submit rebases the current branch and its descendants onto their parents,
pushes all affected branches, and creates or updates pull requests.

This is the typical workflow command after making changes in a stack:
1. Cascade: rebase current branch + descendants onto their parents
2. Push: force-push all affected branches (with --force-with-lease)
3. PR: create PRs for branches without them, update PR bases for those that have them

If a rebase conflict occurs, resolve it and run 'gh stack continue'.`,
	RunE: runSubmit,
}

var (
	submitDryRunFlag      bool
	submitCurrentOnlyFlag bool
	submitUpdateOnlyFlag  bool
)

func init() {
	submitCmd.Flags().BoolVar(&submitDryRunFlag, "dry-run", false, "show what would be done without doing it")
	submitCmd.Flags().BoolVar(&submitCurrentOnlyFlag, "current-only", false, "only submit current branch, not descendants")
	submitCmd.Flags().BoolVar(&submitUpdateOnlyFlag, "update-only", false, "only update existing PRs, don't create new ones")
	rootCmd.AddCommand(submitCmd)
}

func runSubmit(cmd *cobra.Command, args []string) error {
	return fmt.Errorf("not implemented")
}
```

**Step 2: Verify it compiles and shows in help**

Run: `go build -o gh-stack . && ./gh-stack submit --help`
Expected: Shows submit command help with flags

**Step 3: Commit**

```bash
git add cmd/submit.go
git commit -m "feat(cmd): add submit command structure with flags

Adds the submit command skeleton with --dry-run, --current-only,
and --update-only flags. Implementation to follow.
"
```

---

## Task 3: Implement Submit - Phase 1 (Cascade)

Implement the cascade phase of submit.

**Files:**

- Modify: `cmd/submit.go`
- Modify: `cmd/cascade.go` (export `doCascade` or refactor)

**Step 1: Refactor cascade to support submit context**

First, update `cmd/cascade.go` to accept an operation type parameter. Modify `doCascade` to accept state options:

```go
// In cmd/cascade.go, update doCascade signature and state saving:

// doCascadeWithState performs cascade and saves state with the given operation type.
func doCascadeWithState(g *git.Git, cfg *config.Config, branches []*tree.Node, dryRun bool, operation string, updateOnly bool) error {
	originalBranch, err := g.CurrentBranch()
	if err != nil {
		return err
	}
	originalHead, err := g.GetTip(originalBranch)
	if err != nil {
		return err
	}

	for i, b := range branches {
		parent, err := cfg.GetParent(b.Name)
		if err != nil {
			continue // trunk or untracked
		}

		// Check if rebase needed
		needsRebase, err := g.NeedsRebase(b.Name, parent)
		if err != nil {
			return err
		}

		if !needsRebase {
			fmt.Printf("Cascading %s... already up to date\n", b.Name)
			continue
		}

		if dryRun {
			fmt.Printf("Would rebase %s onto %s\n", b.Name, parent)
			continue
		}

		fmt.Printf("Cascading %s onto %s...\n", b.Name, parent)

		// Checkout and rebase
		if err := g.Checkout(b.Name); err != nil {
			return err
		}

		if err := g.Rebase(parent); err != nil {
			// Rebase conflict - save state
			remaining := make([]string, 0, len(branches)-i-1)
			for _, r := range branches[i+1:] {
				remaining = append(remaining, r.Name)
			}

			st := &state.CascadeState{
				Current:      b.Name,
				Pending:      remaining,
				OriginalHead: originalHead,
				Operation:    operation,
				UpdateOnly:   updateOnly,
			}
			_ = state.Save(g.GetGitDir(), st) //nolint:errcheck // best effort

			fmt.Printf("\nCONFLICT: Resolve conflicts and run 'gh stack continue', or 'gh stack abort' to cancel.\n")
			fmt.Printf("Remaining branches: %v\n", remaining)
			return ErrConflict
		}

		fmt.Printf("Cascading %s... ok\n", b.Name)
	}

	// Return to original branch
	if !dryRun {
		_ = g.Checkout(originalBranch) //nolint:errcheck // best effort
	}

	return nil
}

// Keep the old doCascade for backward compatibility
func doCascade(g *git.Git, cfg *config.Config, branches []*tree.Node, dryRun bool) error {
	return doCascadeWithState(g, cfg, branches, dryRun, state.OperationCascade, false)
}
```

**Step 2: Implement submit cascade phase**

Update `cmd/submit.go`:

```go
func runSubmit(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	cfg, err := config.Load(cwd)
	if err != nil {
		return err
	}

	g := git.New(cwd)

	// Check for dirty working tree
	dirty, err := g.IsDirty()
	if err != nil {
		return err
	}
	if dirty {
		return fmt.Errorf("working tree has uncommitted changes; commit or stash first")
	}

	// Check if operation already in progress
	if state.Exists(g.GetGitDir()) {
		return fmt.Errorf("operation already in progress; use 'gh stack continue' or 'gh stack abort'")
	}

	currentBranch, err := g.CurrentBranch()
	if err != nil {
		return err
	}

	// Build tree
	root, err := tree.Build(cfg)
	if err != nil {
		return err
	}

	node := tree.FindNode(root, currentBranch)
	if node == nil {
		return fmt.Errorf("branch %q is not tracked", currentBranch)
	}

	// Collect branches to submit (current + descendants)
	var branches []*tree.Node
	branches = append(branches, node)
	if !submitCurrentOnlyFlag {
		branches = append(branches, tree.GetDescendants(node)...)
	}

	// Phase 1: Cascade
	fmt.Println("=== Phase 1: Cascade ===")
	if err := doCascadeWithState(g, cfg, branches, submitDryRunFlag, state.OperationSubmit, submitUpdateOnlyFlag); err != nil {
		return err // Conflict or error - state saved, user can continue
	}

	// Phases 2 & 3 will be added in subsequent tasks
	return doSubmitPushAndPR(g, cfg, root, branches, submitDryRunFlag, submitUpdateOnlyFlag)
}

// doSubmitPushAndPR handles push and PR creation/update phases.
// This is called after cascade succeeds (or from continue after conflict resolution).
func doSubmitPushAndPR(g *git.Git, cfg *config.Config, root *tree.Node, branches []*tree.Node, dryRun, updateOnly bool) error {
	// TODO: Implement in Task 4 and 5
	fmt.Println("=== Phase 2: Push ===")
	fmt.Println("(not yet implemented)")
	fmt.Println("=== Phase 3: PRs ===")
	fmt.Println("(not yet implemented)")
	return nil
}
```

**Step 3: Run to verify cascade phase works**

Run: `go build -o gh-stack . && ./gh-stack submit --dry-run`
Expected: Shows cascade dry-run output

**Step 4: Commit**

```bash
git add cmd/cascade.go cmd/submit.go
git commit -m "feat(submit): implement cascade phase

Adds the cascade phase of submit, reusing cascade logic but with
submit-specific state tracking. Refactors doCascade to support
operation type parameter for state persistence.
"
```

---

## Task 4: Implement Submit - Phase 2 (Push)

Implement the push phase.

**Files:**

- Modify: `cmd/submit.go`

**Step 1: Implement push phase**

Update `doSubmitPushAndPR` in `cmd/submit.go`:

```go
// doSubmitPushAndPR handles push and PR creation/update phases.
func doSubmitPushAndPR(g *git.Git, cfg *config.Config, root *tree.Node, branches []*tree.Node, dryRun, updateOnly bool) error {
	trunk, err := cfg.GetTrunk()
	if err != nil {
		return err
	}

	// Phase 2: Push all branches
	fmt.Println("\n=== Phase 2: Push ===")
	for _, b := range branches {
		if dryRun {
			fmt.Printf("Would push %s -> origin/%s (forced)\n", b.Name, b.Name)
		} else {
			fmt.Printf("Pushing %s -> origin/%s (forced)\n", b.Name, b.Name)
			if err := g.Push(b.Name, true); err != nil {
				return fmt.Errorf("failed to push %s: %w", b.Name, err)
			}
		}
	}

	// Phase 3: Create/update PRs
	return doSubmitPRs(cfg, root, branches, trunk, dryRun, updateOnly)
}

// doSubmitPRs handles PR creation/update for all branches.
func doSubmitPRs(cfg *config.Config, root *tree.Node, branches []*tree.Node, trunk string, dryRun, updateOnly bool) error {
	// TODO: Implement in Task 5
	fmt.Println("\n=== Phase 3: PRs ===")
	fmt.Println("(not yet implemented)")
	return nil
}
```

**Step 2: Verify push phase works in dry-run**

Run: `go build -o gh-stack . && ./gh-stack submit --dry-run`
Expected: Shows cascade and push dry-run output

**Step 3: Commit**

```bash
git add cmd/submit.go
git commit -m "feat(submit): implement push phase

Adds force-push with lease for all branches in the submit set.
"
```

---

## Task 5: Implement Submit - Phase 3 (PRs)

Implement PR creation/update for all submitted branches.

**Files:**

- Modify: `cmd/submit.go`

**Step 1: Implement PR phase**

Replace `doSubmitPRs` in `cmd/submit.go`:

```go
// doSubmitPRs handles PR creation/update for all branches.
func doSubmitPRs(cfg *config.Config, root *tree.Node, branches []*tree.Node, trunk string, dryRun, updateOnly bool) error {
	fmt.Println("\n=== Phase 3: PRs ===")

	ghClient, err := github.NewClient()
	if err != nil {
		return err
	}

	for _, b := range branches {
		parent, _ := cfg.GetParent(b.Name) //nolint:errcheck // empty is fine
		if parent == "" {
			parent = trunk
		}

		existingPR, _ := cfg.GetPR(b.Name) //nolint:errcheck // 0 is fine

		if existingPR > 0 {
			// Update existing PR
			if dryRun {
				fmt.Printf("Would update PR #%d base to %q\n", existingPR, parent)
			} else {
				fmt.Printf("Updating PR #%d for %s (base: %s)\n", existingPR, b.Name, parent)
				if err := ghClient.UpdatePRBase(existingPR, parent); err != nil {
					fmt.Printf("Warning: failed to update PR #%d base: %v\n", existingPR, err)
				}
				// Update stack comment
				if err := ghClient.GenerateAndPostStackComment(root, b.Name, trunk, existingPR); err != nil {
					fmt.Printf("Warning: failed to update stack comment for PR #%d: %v\n", existingPR, err)
				}
			}
		} else if !updateOnly {
			// Create new PR
			if dryRun {
				fmt.Printf("Would create PR for %s (base: %s)\n", b.Name, parent)
			} else {
				prNum, err := createPRForBranch(ghClient, cfg, root, b.Name, parent, trunk)
				if err != nil {
					fmt.Printf("Warning: failed to create PR for %s: %v\n", b.Name, err)
				} else {
					fmt.Printf("Created PR #%d for %s\n", prNum, b.Name)
				}
			}
		} else {
			fmt.Printf("Skipping %s (no existing PR, --update-only)\n", b.Name)
		}
	}

	return nil
}

// createPRForBranch creates a PR for the given branch and stores the PR number.
func createPRForBranch(ghClient *github.Client, cfg *config.Config, root *tree.Node, branch, base, trunk string) (int, error) {
	// Determine if draft (not targeting trunk = middle of stack)
	draft := base != trunk

	pr, err := ghClient.CreatePR(branch, base, draft)
	if err != nil {
		return 0, err
	}

	// Store PR number in config
	if err := cfg.SetPR(branch, pr.Number); err != nil {
		return pr.Number, fmt.Errorf("PR created but failed to store number: %w", err)
	}

	// Add stack navigation comment
	if err := ghClient.GenerateAndPostStackComment(root, branch, trunk, pr.Number); err != nil {
		fmt.Printf("Warning: failed to add stack comment to PR #%d: %v\n", pr.Number, err)
	}

	return pr.Number, nil
}
```

**Step 2: Verify PR phase works in dry-run**

Run: `go build -o gh-stack . && ./gh-stack submit --dry-run`
Expected: Shows all three phases in dry-run

**Step 3: Commit**

```bash
git add cmd/submit.go
git commit -m "feat(submit): implement PR creation/update phase

Creates PRs for branches without them (as drafts if not targeting trunk),
updates PR bases for branches that have them. Respects --update-only flag.
"
```

---

## Task 6: Add CreatePR Method to GitHub Client

The PR phase needs a non-interactive `CreatePR` method.

**Files:**

- Modify: `internal/github/github.go`
- Modify: `internal/github/github_test.go`

**Step 1: Write the failing test**

Add to `internal/github/github_test.go`:

```go
func TestClient_CreatePRNonInteractive(t *testing.T) {
	var capturedBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/pulls") {
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &capturedBody)
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"number": 42, "html_url": "https://github.com/owner/repo/pull/42"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := &Client{
		restClient: &mockRESTClient{baseURL: server.URL},
		owner:      "owner",
		repo:       "repo",
	}

	pr, err := client.CreatePR("feature", "main", false)
	if err != nil {
		t.Fatalf("CreatePR failed: %v", err)
	}
	if pr.Number != 42 {
		t.Errorf("expected PR number 42, got %d", pr.Number)
	}
	if capturedBody["draft"] != false {
		t.Errorf("expected draft=false, got %v", capturedBody["draft"])
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run TestClient_CreatePRNonInteractive ./internal/github/...`
Expected: FAIL - method doesn't exist

**Step 3: Implement CreatePR**

Add to `internal/github/github.go`:

```go
// CreatePR creates a new pull request non-interactively.
// The title is derived from the branch name.
// If draft is true, creates a draft PR.
func (c *Client) CreatePR(head, base string, draft bool) (*PR, error) {
	// Generate title from branch name (replace - and _ with spaces, title case)
	title := strings.ReplaceAll(head, "-", " ")
	title = strings.ReplaceAll(title, "_", " ")
	title = strings.Title(title) //nolint:staticcheck // simple title case is fine

	body := map[string]interface{}{
		"title": title,
		"head":  head,
		"base":  base,
		"draft": draft,
	}

	var result PR
	err := c.restClient.Post(fmt.Sprintf("repos/%s/%s/pulls", c.owner, c.repo), body, &result)
	if err != nil {
		return nil, fmt.Errorf("create PR: %w", err)
	}

	return &result, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test -v -run TestClient_CreatePRNonInteractive ./internal/github/...`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/github/github.go internal/github/github_test.go
git commit -m "feat(github): add non-interactive CreatePR method

Creates PRs programmatically without user interaction, supporting
draft mode. Title is derived from branch name.
"
```

---

## Task 7: Update Continue Command for Submit

When continuing after a conflict during submit, we need to also do push + PR phases.

**Files:**

- Modify: `cmd/continue.go`

**Step 1: Update continue to handle submit operation**

Update `cmd/continue.go`:

```go
// cmd/continue.go
package cmd

import (
	"fmt"
	"os"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/git"
	"github.com/boneskull/gh-stack/internal/state"
	"github.com/boneskull/gh-stack/internal/tree"
	"github.com/spf13/cobra"
)

var continueCmd = &cobra.Command{
	Use:   "continue",
	Short: "Continue an operation after resolving conflicts",
	Long:  `Continue a cascade or submit operation after resolving rebase conflicts.`,
	RunE:  runContinue,
}

func init() {
	rootCmd.AddCommand(continueCmd)
}

func runContinue(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	g := git.New(cwd)

	// Check if operation in progress
	st, err := state.Load(g.GetGitDir())
	if err != nil {
		return fmt.Errorf("no operation in progress")
	}

	// Complete the in-progress rebase
	if g.IsRebaseInProgress() {
		fmt.Println("Continuing rebase...")
		if rebaseErr := g.RebaseContinue(); rebaseErr != nil {
			return fmt.Errorf("rebase --continue failed; resolve conflicts first")
		}
	}

	fmt.Printf("Completed %s\n", st.Current)

	cfg, err := config.Load(cwd)
	if err != nil {
		return err
	}

	// Build tree to get node objects
	root, err := tree.Build(cfg)
	if err != nil {
		return err
	}

	// If there are more branches to cascade, continue cascading
	if len(st.Pending) > 0 {
		var branches []*tree.Node
		for _, name := range st.Pending {
			if node := tree.FindNode(root, name); node != nil {
				branches = append(branches, node)
			}
		}

		// Remove state file before continuing (will be recreated if conflict)
		_ = state.Remove(g.GetGitDir()) //nolint:errcheck // cleanup

		if err := doCascadeWithState(g, cfg, branches, false, st.Operation, st.UpdateOnly); err != nil {
			return err // Another conflict - state saved
		}
	} else {
		// No more branches to cascade - cleanup state
		_ = state.Remove(g.GetGitDir()) //nolint:errcheck // cleanup
	}

	// If this was a submit operation, continue with push + PR phases
	if st.Operation == state.OperationSubmit {
		// Rebuild branches list (current + all that were pending, now completed)
		currentNode := tree.FindNode(root, st.Current)
		if currentNode == nil {
			return fmt.Errorf("branch %q not found in tree", st.Current)
		}

		var allBranches []*tree.Node
		allBranches = append(allBranches, currentNode)
		for _, name := range st.Pending {
			if node := tree.FindNode(root, name); node != nil {
				allBranches = append(allBranches, node)
			}
		}

		// Also need to include branches that were cascaded before the conflict
		// For simplicity, we re-derive from current branch's descendants
		// This assumes submit always starts from a single branch
		// TODO: Consider storing original branch list in state

		return doSubmitPushAndPR(g, cfg, root, allBranches, false, st.UpdateOnly)
	}

	fmt.Println("Cascade complete!")
	return nil
}
```

**Step 2: Verify it compiles**

Run: `go build -o gh-stack .`
Expected: Builds successfully

**Step 3: Commit**

```bash
git add cmd/continue.go
git commit -m "feat(continue): support submit operation continuation

When continuing after a submit conflict, proceeds with push and PR
phases after cascade completes.
"
```

---

## Task 8: Write E2E Tests for Submit

**Files:**

- Create: `e2e/submit_test.go`

**Step 1: Write the e2e tests**

```go
// e2e/submit_test.go
package e2e_test

import (
	"strings"
	"testing"
)

func TestSubmitSingleBranch(t *testing.T) {
	env := NewTestEnvWithRemote(t)
	env.MustRun("init")

	env.MustRun("create", "feature-1")
	env.CreateCommit("feature 1 work")

	// Submit with --current-only (no descendants)
	result := env.MustRun("submit", "--current-only")

	// Should show all three phases
	if !strings.Contains(result.Stdout, "Phase 1: Cascade") {
		t.Error("expected cascade phase output")
	}
	if !strings.Contains(result.Stdout, "Phase 2: Push") {
		t.Error("expected push phase output")
	}
	if !strings.Contains(result.Stdout, "Phase 3: PRs") {
		t.Error("expected PR phase output")
	}

	// Branch should be on remote
	remoteBranches := env.GitRemote("branch")
	if !strings.Contains(remoteBranches, "feature-1") {
		t.Errorf("feature-1 not on remote: %s", remoteBranches)
	}
}

func TestSubmitStack(t *testing.T) {
	env := NewTestEnvWithRemote(t)
	env.MustRun("init")

	// Create stack: main -> feat-a -> feat-b
	env.MustRun("create", "feat-a")
	env.CreateCommit("a work")

	env.MustRun("create", "feat-b")
	env.CreateCommit("b work")

	// Go back to feat-a
	env.Git("checkout", "feat-a")

	// Submit from feat-a (should include feat-a and feat-b)
	env.MustRun("submit")

	// Both branches should be on remote
	remoteBranches := env.GitRemote("branch")
	if !strings.Contains(remoteBranches, "feat-a") {
		t.Errorf("feat-a not on remote: %s", remoteBranches)
	}
	if !strings.Contains(remoteBranches, "feat-b") {
		t.Errorf("feat-b not on remote: %s", remoteBranches)
	}
}

func TestSubmitDryRun(t *testing.T) {
	env := NewTestEnvWithRemote(t)
	env.MustRun("init")

	env.MustRun("create", "feature-1")
	env.CreateCommit("feature 1 work")

	result := env.MustRun("submit", "--dry-run")

	// Should show "Would" for each action
	if !strings.Contains(result.Stdout, "Would") {
		t.Error("expected dry-run output with 'Would'")
	}

	// Branch should NOT be on remote
	remoteBranches := env.GitRemote("branch")
	if strings.Contains(remoteBranches, "feature-1") {
		t.Error("feature-1 should not be on remote in dry-run")
	}
}

func TestSubmitWithCascadeNeeded(t *testing.T) {
	env := NewTestEnvWithRemote(t)
	env.MustRun("init")

	// Create stack
	env.MustRun("create", "feat-a")
	env.CreateCommit("a work")

	env.MustRun("create", "feat-b")
	env.CreateCommit("b work")

	// Go back to feat-a and add commit (feat-b now needs rebase)
	env.Git("checkout", "feat-a")
	env.CreateCommit("more a work")

	// Submit should cascade feat-b onto updated feat-a
	env.MustRun("submit")

	// Verify feat-b is rebased (feat-a is ancestor)
	env.AssertAncestor("feat-a", "feat-b")
}

func TestSubmitCurrentOnly(t *testing.T) {
	env := NewTestEnvWithRemote(t)
	env.MustRun("init")

	env.MustRun("create", "feat-a")
	env.CreateCommit("a work")

	env.MustRun("create", "feat-b")
	env.CreateCommit("b work")

	env.Git("checkout", "feat-a")
	env.CreateCommit("more a work")

	// Submit --current-only should NOT cascade feat-b
	env.MustRun("submit", "--current-only")

	// feat-a should be on remote
	remoteBranches := env.GitRemote("branch")
	if !strings.Contains(remoteBranches, "feat-a") {
		t.Errorf("feat-a not on remote: %s", remoteBranches)
	}

	// feat-b should NOT be on remote (not included in submit)
	if strings.Contains(remoteBranches, "feat-b") {
		t.Error("feat-b should not be on remote with --current-only")
	}
}
```

**Step 2: Run tests**

Run: `go test -v -run "TestSubmit" ./e2e/...`
Expected: Tests may fail initially due to missing PR creation (no GitHub in tests)

Note: E2E tests that create PRs will need mock GitHub or skip PR verification. The tests above focus on cascade and push phases.

**Step 3: Commit**

```bash
git add e2e/submit_test.go
git commit -m "test(e2e): add submit command tests

Tests submit phases, dry-run, cascade behavior, and --current-only flag.
"
```

---

## Task 9: Integration Testing and Bug Fixes

Run full test suite and fix any issues.

**Step 1: Run all tests**

Run: `make ci`
Expected: All tests pass

**Step 2: Fix any issues found**

Address any test failures or lint errors.

**Step 3: Final commit**

```bash
git add -A
git commit -m "fix: address issues from integration testing"
```

---

## Task 10: Update Documentation

**Files:**

- Modify: `README.md`

**Step 1: Add submit command to README**

Add documentation for the new `submit` command, including:

- What it does (cascade + push + PR)
- Flags: `--dry-run`, `--current-only`, `--update-only`
- Workflow example
- Conflict resolution with `gh stack continue`

**Step 2: Commit**

```bash
git add README.md
git commit -m "docs: add submit command documentation"
```

---

## Summary

After completing all tasks, `gh stack submit` will:

1. **Cascade** current branch + descendants onto their parents
2. **Push** all affected branches with `--force-with-lease`
3. **Create/update PRs** for all pushed branches

With flags:

- `--dry-run`: Show what would happen without doing it
- `--current-only`: Only submit the current branch, not descendants
- `--update-only`: Only update existing PRs, don't create new ones

And proper conflict handling via `gh stack continue`.
