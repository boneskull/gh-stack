# gh-stack Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a GitHub CLI extension for managing stacked pull requests with parent-child branch relationships.

**Architecture:** Store stack metadata in `.git/config` under custom keys (`stack.trunk`, `branch.<name>.stackParent`, `branch.<name>.stackPR`). Commands are organized in `cmd/` with business logic in `internal/` packages (config, git, tree, github, state). Uses cobra for CLI, go-git for config parsing.

**Tech Stack:** Go 1.22+, Cobra CLI, go-git (for config), gh CLI (for GitHub API)

---

## Phase 1: Project Foundation

### Task 1.1: Initialize Go Module

**Files:**
- Create: `go.mod`
- Create: `.gitignore`

**Step 1: Initialize the Go module**

Run:
```bash
go mod init github.com/boneskull/gh-stack
```

**Step 2: Create .gitignore**

```gitignore
# Binaries
gh-stack
*.exe

# Test artifacts
coverage.out

# IDE
.idea/
.vscode/
*.swp

# OS
.DS_Store
```

**Step 3: Commit**

```bash
git add go.mod .gitignore
git commit -m "chore: initialize go module"
```

---

### Task 1.2: Set Up Directory Structure

**Files:**
- Create: `main.go`
- Create: `cmd/root.go`
- Create: `internal/config/config.go`
- Create: `internal/git/git.go`
- Create: `internal/tree/tree.go`
- Create: `internal/github/github.go`
- Create: `internal/state/state.go`

**Step 1: Create minimal main.go**

```go
// main.go
package main

import (
	"os"

	"github.com/boneskull/gh-stack/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
```

**Step 2: Create root command**

```go
// cmd/root.go
package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "gh-stack",
	Short: "Manage stacked pull requests",
	Long:  `gh-stack tracks parent-child relationships between branches, enabling workflows where PRs target other PRs.`,
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}
```

**Step 3: Create placeholder internal packages**

```go
// internal/config/config.go
package config

// Package config handles reading/writing stack metadata from .git/config.
```

```go
// internal/git/git.go
package git

// Package git provides git operations (rebase, fetch, branch management).
```

```go
// internal/tree/tree.go
package tree

// Package tree handles stack tree traversal and validation.
```

```go
// internal/github/github.go
package github

// Package github provides PR queries via gh CLI.
```

```go
// internal/state/state.go
package state

// Package state handles cascade state persistence.
```

**Step 4: Add cobra dependency**

Run:
```bash
go get github.com/spf13/cobra@latest
go mod tidy
```

**Step 5: Verify it builds**

Run:
```bash
go build -o gh-stack .
./gh-stack --help
```

Expected: Shows help text with "Manage stacked pull requests"

**Step 6: Commit**

```bash
git add .
git commit -m "chore: scaffold project structure with cobra CLI"
```

---

## Phase 2: Config Package (Core Data Model)

### Task 2.1: Config Interface and Types

**Files:**
- Modify: `internal/config/config.go`
- Create: `internal/config/config_test.go`

**Step 1: Write failing test for GetTrunk**

```go
// internal/config/config_test.go
package config_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/boneskull/gh-stack/internal/config"
)

func setupTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Initialize a git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init failed: %v", err)
	}

	// Configure git user for commits
	exec.Command("git", "-C", dir, "config", "user.email", "test@test.com").Run()
	exec.Command("git", "-C", dir, "config", "user.name", "Test").Run()

	return dir
}

func TestGetTrunk_NotInitialized(t *testing.T) {
	dir := setupTestRepo(t)

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	_, err = cfg.GetTrunk()
	if err == nil {
		t.Error("expected error when trunk not set, got nil")
	}
}
```

**Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/config/... -v
```

Expected: FAIL - config.Load doesn't exist

**Step 3: Implement Config type with GetTrunk**

```go
// internal/config/config.go
package config

import (
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrNotInitialized is returned when stack tracking is not initialized.
var ErrNotInitialized = errors.New("stack not initialized: run 'gh stack init' first")

// ErrBranchNotTracked is returned when a branch is not tracked.
var ErrBranchNotTracked = errors.New("branch not tracked")

// Config provides access to stack metadata stored in .git/config.
type Config struct {
	repoPath string
}

// Load creates a Config for the repository at the given path.
func Load(repoPath string) (*Config, error) {
	gitDir := filepath.Join(repoPath, ".git")
	if _, err := exec.Command("git", "-C", repoPath, "rev-parse", "--git-dir").Output(); err != nil {
		return nil, errors.New("not a git repository")
	}
	_ = gitDir // validate .git exists
	return &Config{repoPath: repoPath}, nil
}

// GetTrunk returns the configured trunk branch name.
func (c *Config) GetTrunk() (string, error) {
	out, err := exec.Command("git", "-C", c.repoPath, "config", "--get", "stack.trunk").Output()
	if err != nil {
		return "", ErrNotInitialized
	}
	return strings.TrimSpace(string(out)), nil
}
```

**Step 4: Run test to verify it passes**

Run:
```bash
go test ./internal/config/... -v
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat(config): add Config type with GetTrunk"
```

---

### Task 2.2: SetTrunk Implementation

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

**Step 1: Write failing test for SetTrunk**

Add to `config_test.go`:

```go
func TestSetTrunk(t *testing.T) {
	dir := setupTestRepo(t)

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if err := cfg.SetTrunk("main"); err != nil {
		t.Fatalf("SetTrunk failed: %v", err)
	}

	trunk, err := cfg.GetTrunk()
	if err != nil {
		t.Fatalf("GetTrunk failed: %v", err)
	}
	if trunk != "main" {
		t.Errorf("expected trunk 'main', got %q", trunk)
	}
}
```

**Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/config/... -v -run TestSetTrunk
```

Expected: FAIL - SetTrunk doesn't exist

**Step 3: Implement SetTrunk**

Add to `config.go`:

```go
// SetTrunk sets the trunk branch name.
func (c *Config) SetTrunk(branch string) error {
	cmd := exec.Command("git", "-C", c.repoPath, "config", "stack.trunk", branch)
	return cmd.Run()
}
```

**Step 4: Run test to verify it passes**

Run:
```bash
go test ./internal/config/... -v -run TestSetTrunk
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat(config): add SetTrunk"
```

---

### Task 2.3: Branch Parent Operations

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

**Step 1: Write failing tests for GetParent and SetParent**

Add to `config_test.go`:

```go
func TestGetParent_NotTracked(t *testing.T) {
	dir := setupTestRepo(t)

	cfg, _ := config.Load(dir)

	_, err := cfg.GetParent("feature-a")
	if !errors.Is(err, config.ErrBranchNotTracked) {
		t.Errorf("expected ErrBranchNotTracked, got %v", err)
	}
}

func TestSetAndGetParent(t *testing.T) {
	dir := setupTestRepo(t)

	cfg, _ := config.Load(dir)
	cfg.SetTrunk("main")

	if err := cfg.SetParent("feature-a", "main"); err != nil {
		t.Fatalf("SetParent failed: %v", err)
	}

	parent, err := cfg.GetParent("feature-a")
	if err != nil {
		t.Fatalf("GetParent failed: %v", err)
	}
	if parent != "main" {
		t.Errorf("expected parent 'main', got %q", parent)
	}
}
```

**Step 2: Run tests to verify they fail**

Run:
```bash
go test ./internal/config/... -v -run "TestGetParent|TestSetAndGetParent"
```

Expected: FAIL

**Step 3: Implement GetParent and SetParent**

Add to `config.go`:

```go
// GetParent returns the parent branch for the given branch.
func (c *Config) GetParent(branch string) (string, error) {
	key := "branch." + branch + ".stackParent"
	out, err := exec.Command("git", "-C", c.repoPath, "config", "--get", key).Output()
	if err != nil {
		return "", ErrBranchNotTracked
	}
	return strings.TrimSpace(string(out)), nil
}

// SetParent sets the parent branch for the given branch.
func (c *Config) SetParent(branch, parent string) error {
	key := "branch." + branch + ".stackParent"
	return exec.Command("git", "-C", c.repoPath, "config", key, parent).Run()
}

// RemoveParent removes the parent tracking for a branch.
func (c *Config) RemoveParent(branch string) error {
	key := "branch." + branch + ".stackParent"
	// --unset returns error if key doesn't exist, which is fine
	exec.Command("git", "-C", c.repoPath, "config", "--unset", key).Run()
	return nil
}
```

**Step 4: Run tests to verify they pass**

Run:
```bash
go test ./internal/config/... -v
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat(config): add parent branch operations"
```

---

### Task 2.4: PR Number Operations

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

**Step 1: Write failing test for PR operations**

Add to `config_test.go`:

```go
func TestPRNumber(t *testing.T) {
	dir := setupTestRepo(t)

	cfg, _ := config.Load(dir)

	// No PR set initially
	_, err := cfg.GetPR("feature-a")
	if err == nil {
		t.Error("expected error for unset PR, got nil")
	}

	// Set PR
	if err := cfg.SetPR("feature-a", 1234); err != nil {
		t.Fatalf("SetPR failed: %v", err)
	}

	// Get PR
	pr, err := cfg.GetPR("feature-a")
	if err != nil {
		t.Fatalf("GetPR failed: %v", err)
	}
	if pr != 1234 {
		t.Errorf("expected PR 1234, got %d", pr)
	}

	// Remove PR
	if err := cfg.RemovePR("feature-a"); err != nil {
		t.Fatalf("RemovePR failed: %v", err)
	}

	_, err = cfg.GetPR("feature-a")
	if err == nil {
		t.Error("expected error after removing PR, got nil")
	}
}
```

**Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/config/... -v -run TestPRNumber
```

Expected: FAIL

**Step 3: Implement PR operations**

Add to `config.go`:

```go
import (
	"strconv"
	// ... existing imports
)

// ErrNoPR is returned when a branch has no associated PR.
var ErrNoPR = errors.New("no PR associated with branch")

// GetPR returns the PR number for the given branch.
func (c *Config) GetPR(branch string) (int, error) {
	key := "branch." + branch + ".stackPR"
	out, err := exec.Command("git", "-C", c.repoPath, "config", "--get", key).Output()
	if err != nil {
		return 0, ErrNoPR
	}
	return strconv.Atoi(strings.TrimSpace(string(out)))
}

// SetPR sets the PR number for the given branch.
func (c *Config) SetPR(branch string, pr int) error {
	key := "branch." + branch + ".stackPR"
	return exec.Command("git", "-C", c.repoPath, "config", key, strconv.Itoa(pr)).Run()
}

// RemovePR removes the PR association for a branch.
func (c *Config) RemovePR(branch string) error {
	key := "branch." + branch + ".stackPR"
	exec.Command("git", "-C", c.repoPath, "config", "--unset", key).Run()
	return nil
}
```

**Step 4: Run test to verify it passes**

Run:
```bash
go test ./internal/config/... -v -run TestPRNumber
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat(config): add PR number operations"
```

---

### Task 2.5: List Tracked Branches

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

**Step 1: Write failing test**

Add to `config_test.go`:

```go
func TestListTrackedBranches(t *testing.T) {
	dir := setupTestRepo(t)

	cfg, _ := config.Load(dir)
	cfg.SetTrunk("main")
	cfg.SetParent("feature-a", "main")
	cfg.SetParent("feature-b", "feature-a")

	branches, err := cfg.ListTrackedBranches()
	if err != nil {
		t.Fatalf("ListTrackedBranches failed: %v", err)
	}

	if len(branches) != 2 {
		t.Errorf("expected 2 branches, got %d", len(branches))
	}

	// Check both branches are present
	found := make(map[string]bool)
	for _, b := range branches {
		found[b] = true
	}
	if !found["feature-a"] || !found["feature-b"] {
		t.Errorf("missing expected branches, got %v", branches)
	}
}
```

**Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/config/... -v -run TestListTrackedBranches
```

Expected: FAIL

**Step 3: Implement ListTrackedBranches**

Add to `config.go`:

```go
import (
	"bufio"
	"bytes"
	"regexp"
	// ... existing imports
)

// ListTrackedBranches returns all branches that have a stackParent set.
func (c *Config) ListTrackedBranches() ([]string, error) {
	out, err := exec.Command("git", "-C", c.repoPath, "config", "--get-regexp", "^branch\\..*\\.stackParent$").Output()
	if err != nil {
		// No matches is not an error
		return []string{}, nil
	}

	var branches []string
	re := regexp.MustCompile(`^branch\.(.+)\.stackParent\s+`)
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		if matches := re.FindStringSubmatch(line); len(matches) > 1 {
			branches = append(branches, matches[1])
		}
	}
	return branches, nil
}
```

**Step 4: Run test to verify it passes**

Run:
```bash
go test ./internal/config/... -v -run TestListTrackedBranches
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat(config): add ListTrackedBranches"
```

---

## Phase 3: Tree Package (Stack Traversal)

### Task 3.1: Build Tree Structure

**Files:**
- Modify: `internal/tree/tree.go`
- Create: `internal/tree/tree_test.go`

**Step 1: Write failing test for BuildTree**

```go
// internal/tree/tree_test.go
package tree_test

import (
	"os/exec"
	"testing"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/tree"
)

func setupTestRepo(t *testing.T) (*config.Config, string) {
	t.Helper()
	dir := t.TempDir()

	exec.Command("git", "init", dir).Run()
	exec.Command("git", "-C", dir, "config", "user.email", "test@test.com").Run()
	exec.Command("git", "-C", dir, "config", "user.name", "Test").Run()

	cfg, _ := config.Load(dir)
	return cfg, dir
}

func TestBuildTree(t *testing.T) {
	cfg, _ := setupTestRepo(t)

	cfg.SetTrunk("main")
	cfg.SetParent("feature-a", "main")
	cfg.SetParent("feature-b", "feature-a")
	cfg.SetParent("feature-c", "feature-a")

	root, err := tree.Build(cfg)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if root.Name != "main" {
		t.Errorf("expected root 'main', got %q", root.Name)
	}

	if len(root.Children) != 1 {
		t.Fatalf("expected 1 child of main, got %d", len(root.Children))
	}

	featureA := root.Children[0]
	if featureA.Name != "feature-a" {
		t.Errorf("expected 'feature-a', got %q", featureA.Name)
	}

	if len(featureA.Children) != 2 {
		t.Errorf("expected 2 children of feature-a, got %d", len(featureA.Children))
	}
}
```

**Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/tree/... -v
```

Expected: FAIL

**Step 3: Implement tree types and Build**

```go
// internal/tree/tree.go
package tree

import (
	"sort"

	"github.com/boneskull/gh-stack/internal/config"
)

// Node represents a branch in the stack tree.
type Node struct {
	Name     string
	PR       int  // 0 if no PR
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
```

**Step 4: Run test to verify it passes**

Run:
```bash
go test ./internal/tree/... -v
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/tree/
git commit -m "feat(tree): add tree building from config"
```

---

### Task 3.2: Tree Traversal Helpers

**Files:**
- Modify: `internal/tree/tree.go`
- Modify: `internal/tree/tree_test.go`

**Step 1: Write failing tests for traversal**

Add to `tree_test.go`:

```go
func TestFindNode(t *testing.T) {
	cfg, _ := setupTestRepo(t)
	cfg.SetTrunk("main")
	cfg.SetParent("feature-a", "main")

	root, _ := tree.Build(cfg)

	node := tree.FindNode(root, "feature-a")
	if node == nil {
		t.Fatal("FindNode returned nil")
	}
	if node.Name != "feature-a" {
		t.Errorf("expected 'feature-a', got %q", node.Name)
	}

	notFound := tree.FindNode(root, "nonexistent")
	if notFound != nil {
		t.Error("expected nil for nonexistent branch")
	}
}

func TestGetAncestors(t *testing.T) {
	cfg, _ := setupTestRepo(t)
	cfg.SetTrunk("main")
	cfg.SetParent("feature-a", "main")
	cfg.SetParent("feature-b", "feature-a")

	root, _ := tree.Build(cfg)
	node := tree.FindNode(root, "feature-b")

	ancestors := tree.GetAncestors(node)
	if len(ancestors) != 2 {
		t.Fatalf("expected 2 ancestors, got %d", len(ancestors))
	}
	if ancestors[0].Name != "feature-a" || ancestors[1].Name != "main" {
		t.Errorf("unexpected ancestors: %v", ancestors)
	}
}
```

**Step 2: Run tests to verify they fail**

Run:
```bash
go test ./internal/tree/... -v -run "TestFindNode|TestGetAncestors"
```

Expected: FAIL

**Step 3: Implement traversal helpers**

Add to `tree.go`:

```go
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
```

**Step 4: Run tests to verify they pass**

Run:
```bash
go test ./internal/tree/... -v
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/tree/
git commit -m "feat(tree): add traversal helpers"
```

---

## Phase 4: Git Package (Git Operations)

### Task 4.1: Basic Git Operations

**Files:**
- Modify: `internal/git/git.go`
- Create: `internal/git/git_test.go`

**Step 1: Write failing test for CurrentBranch**

```go
// internal/git/git_test.go
package git_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/boneskull/gh-stack/internal/git"
)

func setupTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if err := cmd.Run(); err != nil {
			t.Fatalf("git %v failed: %v", args, err)
		}
	}

	run("init")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "Test")

	// Create initial commit so we have a branch
	f := filepath.Join(dir, "README.md")
	os.WriteFile(f, []byte("# Test"), 0644)
	run("add", ".")
	run("commit", "-m", "initial")

	return dir
}

func TestCurrentBranch(t *testing.T) {
	dir := setupTestRepo(t)

	g := git.New(dir)
	branch, err := g.CurrentBranch()
	if err != nil {
		t.Fatalf("CurrentBranch failed: %v", err)
	}

	// Default branch after init is usually main or master
	if branch != "main" && branch != "master" {
		t.Errorf("expected 'main' or 'master', got %q", branch)
	}
}
```

**Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/git/... -v
```

Expected: FAIL

**Step 3: Implement Git type with CurrentBranch**

```go
// internal/git/git.go
package git

import (
	"errors"
	"os/exec"
	"strings"
)

// ErrDirtyWorkTree is returned when the working tree has uncommitted changes.
var ErrDirtyWorkTree = errors.New("working tree has uncommitted changes")

// Git provides git operations for a repository.
type Git struct {
	repoPath string
}

// New creates a Git instance for the repository at the given path.
func New(repoPath string) *Git {
	return &Git{repoPath: repoPath}
}

// CurrentBranch returns the name of the current branch.
func (g *Git) CurrentBranch() (string, error) {
	out, err := exec.Command("git", "-C", g.repoPath, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
```

**Step 4: Run test to verify it passes**

Run:
```bash
go test ./internal/git/... -v
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/git/
git commit -m "feat(git): add Git type with CurrentBranch"
```

---

### Task 4.2: Branch Existence and Creation

**Files:**
- Modify: `internal/git/git.go`
- Modify: `internal/git/git_test.go`

**Step 1: Write failing tests**

Add to `git_test.go`:

```go
func TestBranchExists(t *testing.T) {
	dir := setupTestRepo(t)
	g := git.New(dir)

	// Current branch should exist
	current, _ := g.CurrentBranch()
	if !g.BranchExists(current) {
		t.Errorf("expected current branch %q to exist", current)
	}

	// Nonexistent branch
	if g.BranchExists("nonexistent-branch-xyz") {
		t.Error("expected nonexistent branch to not exist")
	}
}

func TestCreateBranch(t *testing.T) {
	dir := setupTestRepo(t)
	g := git.New(dir)

	err := g.CreateBranch("new-feature")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	if !g.BranchExists("new-feature") {
		t.Error("new branch should exist after creation")
	}
}

func TestCheckout(t *testing.T) {
	dir := setupTestRepo(t)
	g := git.New(dir)

	g.CreateBranch("feature")
	err := g.Checkout("feature")
	if err != nil {
		t.Fatalf("Checkout failed: %v", err)
	}

	current, _ := g.CurrentBranch()
	if current != "feature" {
		t.Errorf("expected current branch 'feature', got %q", current)
	}
}
```

**Step 2: Run tests to verify they fail**

Run:
```bash
go test ./internal/git/... -v -run "TestBranchExists|TestCreateBranch|TestCheckout"
```

Expected: FAIL

**Step 3: Implement branch operations**

Add to `git.go`:

```go
// BranchExists checks if a branch exists.
func (g *Git) BranchExists(branch string) bool {
	err := exec.Command("git", "-C", g.repoPath, "rev-parse", "--verify", "refs/heads/"+branch).Run()
	return err == nil
}

// CreateBranch creates a new branch at the current HEAD.
func (g *Git) CreateBranch(name string) error {
	return exec.Command("git", "-C", g.repoPath, "branch", name).Run()
}

// Checkout switches to the specified branch.
func (g *Git) Checkout(branch string) error {
	return exec.Command("git", "-C", g.repoPath, "checkout", branch).Run()
}

// CreateAndCheckout creates a new branch and switches to it.
func (g *Git) CreateAndCheckout(name string) error {
	return exec.Command("git", "-C", g.repoPath, "checkout", "-b", name).Run()
}
```

**Step 4: Run tests to verify they pass**

Run:
```bash
go test ./internal/git/... -v
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/git/
git commit -m "feat(git): add branch existence and creation"
```

---

### Task 4.3: Working Tree Status

**Files:**
- Modify: `internal/git/git.go`
- Modify: `internal/git/git_test.go`

**Step 1: Write failing test for IsDirty**

Add to `git_test.go`:

```go
func TestIsDirty(t *testing.T) {
	dir := setupTestRepo(t)
	g := git.New(dir)

	// Clean repo
	dirty, err := g.IsDirty()
	if err != nil {
		t.Fatalf("IsDirty failed: %v", err)
	}
	if dirty {
		t.Error("expected clean repo to not be dirty")
	}

	// Make it dirty
	os.WriteFile(filepath.Join(dir, "newfile.txt"), []byte("content"), 0644)

	dirty, err = g.IsDirty()
	if err != nil {
		t.Fatalf("IsDirty failed: %v", err)
	}
	if !dirty {
		t.Error("expected repo with untracked file to be dirty")
	}
}

func TestHasStagedChanges(t *testing.T) {
	dir := setupTestRepo(t)
	g := git.New(dir)

	// No staged changes initially
	staged, err := g.HasStagedChanges()
	if err != nil {
		t.Fatalf("HasStagedChanges failed: %v", err)
	}
	if staged {
		t.Error("expected no staged changes")
	}

	// Stage a change
	f := filepath.Join(dir, "newfile.txt")
	os.WriteFile(f, []byte("content"), 0644)
	exec.Command("git", "-C", dir, "add", f).Run()

	staged, err = g.HasStagedChanges()
	if err != nil {
		t.Fatalf("HasStagedChanges failed: %v", err)
	}
	if !staged {
		t.Error("expected staged changes after git add")
	}
}
```

**Step 2: Run tests to verify they fail**

Run:
```bash
go test ./internal/git/... -v -run "TestIsDirty|TestHasStagedChanges"
```

Expected: FAIL

**Step 3: Implement status checks**

Add to `git.go`:

```go
// IsDirty returns true if there are uncommitted changes (staged or unstaged).
func (g *Git) IsDirty() (bool, error) {
	out, err := exec.Command("git", "-C", g.repoPath, "status", "--porcelain").Output()
	if err != nil {
		return false, err
	}
	return len(strings.TrimSpace(string(out))) > 0, nil
}

// HasStagedChanges returns true if there are staged changes.
func (g *Git) HasStagedChanges() (bool, error) {
	err := exec.Command("git", "-C", g.repoPath, "diff", "--cached", "--quiet").Run()
	if err != nil {
		// Exit code 1 means there are differences
		return true, nil
	}
	return false, nil
}
```

**Step 4: Run tests to verify they pass**

Run:
```bash
go test ./internal/git/... -v
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/git/
git commit -m "feat(git): add working tree status checks"
```

---

## Phase 5: First Commands (init, log)

### Task 5.1: Init Command

**Files:**
- Create: `cmd/init.go`
- Create: `cmd/init_test.go`

**Step 1: Write failing test for init command**

```go
// cmd/init_test.go
package cmd_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/boneskull/gh-stack/internal/config"
)

func setupTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Run()
	}

	run("init")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "Test")

	// Create main branch with initial commit
	f := filepath.Join(dir, "README.md")
	os.WriteFile(f, []byte("# Test"), 0644)
	run("add", ".")
	run("commit", "-m", "initial")

	return dir
}

func TestInitCommand(t *testing.T) {
	dir := setupTestRepo(t)

	// Run init via the binary (or we can test the function directly)
	cmd := exec.Command("go", "run", ".", "init", "--trunk", "main")
	cmd.Dir = filepath.Join(dir, "..") // Need to be in module root
	// Actually, let's test the internals instead

	// For now, just verify the config package works
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	err = cfg.SetTrunk("main")
	if err != nil {
		t.Fatalf("SetTrunk failed: %v", err)
	}

	trunk, err := cfg.GetTrunk()
	if err != nil {
		t.Fatalf("GetTrunk failed: %v", err)
	}
	if trunk != "main" {
		t.Errorf("expected 'main', got %q", trunk)
	}
}
```

**Step 2: Implement init command**

```go
// cmd/init.go
package cmd

import (
	"fmt"
	"os"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/git"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize stack tracking in the repository",
	Long:  `Initialize stack tracking by setting the trunk branch.`,
	RunE:  runInit,
}

var trunkFlag string

func init() {
	initCmd.Flags().StringVar(&trunkFlag, "trunk", "", "trunk branch name (default: main or master)")
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	cfg, err := config.Load(cwd)
	if err != nil {
		return err
	}

	g := git.New(cwd)

	// Determine trunk branch
	trunk := trunkFlag
	if trunk == "" {
		// Try main, then master
		if g.BranchExists("main") {
			trunk = "main"
		} else if g.BranchExists("master") {
			trunk = "master"
		} else {
			return fmt.Errorf("could not determine trunk branch; use --trunk to specify")
		}
	}

	// Validate trunk exists
	if !g.BranchExists(trunk) {
		return fmt.Errorf("branch %q does not exist", trunk)
	}

	// Check if already initialized
	if existing, err := cfg.GetTrunk(); err == nil {
		fmt.Printf("Already initialized with trunk %q\n", existing)
		return nil
	}

	if err := cfg.SetTrunk(trunk); err != nil {
		return err
	}

	fmt.Printf("Initialized stack tracking with trunk %q\n", trunk)
	return nil
}
```

**Step 3: Run tests and verify build**

Run:
```bash
go build -o gh-stack .
go test ./cmd/... -v
```

Expected: PASS

**Step 4: Commit**

```bash
git add cmd/
git commit -m "feat(cmd): add init command"
```

---

### Task 5.2: Log Command

**Files:**
- Create: `cmd/log.go`

**Step 1: Implement log command with tree visualization**

```go
// cmd/log.go
package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/git"
	"github.com/boneskull/gh-stack/internal/tree"
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
)

func init() {
	logCmd.Flags().BoolVar(&logAllFlag, "all", false, "show all branches")
	logCmd.Flags().BoolVar(&logPorcelainFlag, "porcelain", false, "machine-readable output")
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
	currentBranch, _ := g.CurrentBranch()

	if logPorcelainFlag {
		printPorcelain(root, currentBranch)
	} else {
		printTree(root, "", true, currentBranch)
	}

	return nil
}

func printTree(node *tree.Node, prefix string, isLast bool, current string) {
	// Determine the branch indicator
	connector := "├── "
	if isLast {
		connector = "└── "
	}
	if prefix == "" {
		connector = ""
	}

	// Build the line
	marker := ""
	if node.Name == current {
		marker = "* "
	}

	prInfo := ""
	if node.PR > 0 {
		prInfo = fmt.Sprintf(" (#%d)", node.PR)
	}

	fmt.Printf("%s%s%s%s%s\n", prefix, connector, marker, node.Name, prInfo)

	// Prepare prefix for children
	childPrefix := prefix
	if prefix != "" {
		if isLast {
			childPrefix += "    "
		} else {
			childPrefix += "│   "
		}
	}

	for i, child := range node.Children {
		isLastChild := i == len(node.Children)-1
		printTree(child, childPrefix, isLastChild, current)
	}
}

func printPorcelain(node *tree.Node, current string) {
	var printNode func(*tree.Node, int)
	printNode = func(n *tree.Node, depth int) {
		isCurrent := "0"
		if n.Name == current {
			isCurrent = "1"
		}
		parent := ""
		if n.Parent != nil {
			parent = n.Parent.Name
		}
		fmt.Printf("%s\t%s\t%d\t%s\n", n.Name, parent, n.PR, isCurrent)
		for _, child := range n.Children {
			printNode(child, depth+1)
		}
	}
	printNode(node, 0)
}
```

**Step 2: Build and test manually**

Run:
```bash
go build -o gh-stack .
```

**Step 3: Commit**

```bash
git add cmd/log.go
git commit -m "feat(cmd): add log command with tree visualization"
```

---

## Phase 6: Branch Management Commands (create, adopt, orphan)

### Task 6.1: Create Command

**Files:**
- Create: `cmd/create.go`

**Step 1: Implement create command**

```go
// cmd/create.go
package cmd

import (
	"fmt"
	"os"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/git"
	"github.com/boneskull/gh-stack/internal/tree"
	"github.com/spf13/cobra"
)

var createCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new branch stacked on the current branch",
	Long:  `Create a new branch stacked on the current branch and optionally commit staged changes.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runCreate,
}

var (
	createMessageFlag string
	createEmptyFlag   bool
)

func init() {
	createCmd.Flags().StringVarP(&createMessageFlag, "message", "m", "", "commit message for staged changes")
	createCmd.Flags().BoolVar(&createEmptyFlag, "empty", false, "create branch without committing staged changes")
	rootCmd.AddCommand(createCmd)
}

func runCreate(cmd *cobra.Command, args []string) error {
	branchName := args[0]

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	cfg, err := config.Load(cwd)
	if err != nil {
		return err
	}

	g := git.New(cwd)

	// Get current branch
	currentBranch, err := g.CurrentBranch()
	if err != nil {
		return err
	}

	// Validate current branch is trunk or tracked
	trunk, err := cfg.GetTrunk()
	if err != nil {
		return err
	}

	if currentBranch != trunk {
		if _, err := cfg.GetParent(currentBranch); err != nil {
			return fmt.Errorf("current branch %q is not tracked; use 'gh stack adopt' first", currentBranch)
		}
	}

	// Check if branch already exists
	if g.BranchExists(branchName) {
		return fmt.Errorf("branch %q already exists", branchName)
	}

	// Check for staged changes
	hasStaged, err := g.HasStagedChanges()
	if err != nil {
		return err
	}

	if hasStaged && !createEmptyFlag {
		if createMessageFlag == "" {
			return fmt.Errorf("staged changes found but no commit message provided; use -m or --empty")
		}
	}

	// Create and checkout the new branch
	if err := g.CreateAndCheckout(branchName); err != nil {
		return err
	}

	// Commit staged changes if any
	if hasStaged && !createEmptyFlag && createMessageFlag != "" {
		if err := g.Commit(createMessageFlag); err != nil {
			return err
		}
		fmt.Printf("Committed staged changes: %s\n", createMessageFlag)
	}

	// Set parent
	if err := cfg.SetParent(branchName, currentBranch); err != nil {
		return err
	}

	fmt.Printf("Created branch %q stacked on %q\n", branchName, currentBranch)
	return nil
}
```

**Step 2: Add Commit method to git package**

Add to `internal/git/git.go`:

```go
// Commit creates a commit with the given message.
func (g *Git) Commit(message string) error {
	return exec.Command("git", "-C", g.repoPath, "commit", "-m", message).Run()
}
```

**Step 3: Build and verify**

Run:
```bash
go build -o gh-stack .
```

**Step 4: Commit**

```bash
git add cmd/create.go internal/git/git.go
git commit -m "feat(cmd): add create command"
```

---

### Task 6.2: Adopt Command

**Files:**
- Create: `cmd/adopt.go`

**Step 1: Implement adopt command**

```go
// cmd/adopt.go
package cmd

import (
	"fmt"
	"os"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/git"
	"github.com/boneskull/gh-stack/internal/tree"
	"github.com/spf13/cobra"
)

var adoptCmd = &cobra.Command{
	Use:   "adopt [branch]",
	Short: "Start tracking an existing branch",
	Long:  `Start tracking an existing branch by setting its parent.`,
	Args:  cobra.MaximumNArgs(1),
	RunE:  runAdopt,
}

var adoptParentFlag string

func init() {
	adoptCmd.Flags().StringVar(&adoptParentFlag, "parent", "", "parent branch")
	rootCmd.AddCommand(adoptCmd)
}

func runAdopt(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	cfg, err := config.Load(cwd)
	if err != nil {
		return err
	}

	g := git.New(cwd)

	// Determine branch to adopt
	var branchName string
	if len(args) > 0 {
		branchName = args[0]
	} else {
		branchName, err = g.CurrentBranch()
		if err != nil {
			return err
		}
	}

	// Validate branch exists
	if !g.BranchExists(branchName) {
		return fmt.Errorf("branch %q does not exist", branchName)
	}

	// Check if already tracked
	if _, err := cfg.GetParent(branchName); err == nil {
		return fmt.Errorf("branch %q is already tracked", branchName)
	}

	// Determine parent
	parent := adoptParentFlag
	if parent == "" {
		return fmt.Errorf("--parent is required (interactive picker not yet implemented)")
	}

	// Validate parent is trunk or tracked
	trunk, err := cfg.GetTrunk()
	if err != nil {
		return err
	}

	if parent != trunk {
		if _, err := cfg.GetParent(parent); err != nil {
			return fmt.Errorf("parent %q is not tracked", parent)
		}
	}

	// Check for cycles (branch can't be ancestor of parent)
	root, err := tree.Build(cfg)
	if err != nil {
		return err
	}

	parentNode := tree.FindNode(root, parent)
	if parentNode != nil {
		for _, ancestor := range tree.GetAncestors(parentNode) {
			if ancestor.Name == branchName {
				return fmt.Errorf("cannot adopt: would create a cycle")
			}
		}
	}

	// Set parent
	if err := cfg.SetParent(branchName, parent); err != nil {
		return err
	}

	fmt.Printf("Adopted branch %q with parent %q\n", branchName, parent)
	return nil
}
```

**Step 2: Build and verify**

Run:
```bash
go build -o gh-stack .
```

**Step 3: Commit**

```bash
git add cmd/adopt.go
git commit -m "feat(cmd): add adopt command"
```

---

### Task 6.3: Orphan Command

**Files:**
- Create: `cmd/orphan.go`

**Step 1: Implement orphan command**

```go
// cmd/orphan.go
package cmd

import (
	"fmt"
	"os"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/git"
	"github.com/boneskull/gh-stack/internal/tree"
	"github.com/spf13/cobra"
)

var orphanCmd = &cobra.Command{
	Use:   "orphan [branch]",
	Short: "Stop tracking a branch",
	Long:  `Stop tracking a branch by removing it from the stack tree.`,
	Args:  cobra.MaximumNArgs(1),
	RunE:  runOrphan,
}

var orphanForceFlag bool

func init() {
	orphanCmd.Flags().BoolVar(&orphanForceFlag, "force", false, "also orphan all descendants")
	rootCmd.AddCommand(orphanCmd)
}

func runOrphan(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	cfg, err := config.Load(cwd)
	if err != nil {
		return err
	}

	g := git.New(cwd)

	// Determine branch to orphan
	var branchName string
	if len(args) > 0 {
		branchName = args[0]
	} else {
		branchName, err = g.CurrentBranch()
		if err != nil {
			return err
		}
	}

	// Build tree to check for children
	root, err := tree.Build(cfg)
	if err != nil {
		return err
	}

	node := tree.FindNode(root, branchName)
	if node == nil {
		return fmt.Errorf("branch %q is not tracked", branchName)
	}

	// Check for children
	if len(node.Children) > 0 && !orphanForceFlag {
		return fmt.Errorf("branch %q has children; use --force to orphan descendants too", branchName)
	}

	// Orphan descendants if --force
	if orphanForceFlag {
		descendants := tree.GetDescendants(node)
		for _, desc := range descendants {
			cfg.RemoveParent(desc.Name)
			cfg.RemovePR(desc.Name)
			fmt.Printf("Orphaned %q\n", desc.Name)
		}
	}

	// Orphan the branch
	cfg.RemoveParent(branchName)
	cfg.RemovePR(branchName)
	fmt.Printf("Orphaned %q\n", branchName)

	return nil
}
```

**Step 2: Build and verify**

Run:
```bash
go build -o gh-stack .
```

**Step 3: Commit**

```bash
git add cmd/orphan.go
git commit -m "feat(cmd): add orphan command"
```

---

## Phase 7: PR Commands (link, unlink, pr)

### Task 7.1: Link Command

**Files:**
- Create: `cmd/link.go`

**Step 1: Implement link command**

```go
// cmd/link.go
package cmd

import (
	"fmt"
	"os"
	"strconv"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/git"
	"github.com/spf13/cobra"
)

var linkCmd = &cobra.Command{
	Use:   "link <pr-number>",
	Short: "Associate a PR with the current branch",
	Long:  `Associate an existing GitHub PR number with the current branch.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runLink,
}

func init() {
	rootCmd.AddCommand(linkCmd)
}

func runLink(cmd *cobra.Command, args []string) error {
	prNumber, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid PR number: %s", args[0])
	}

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	cfg, err := config.Load(cwd)
	if err != nil {
		return err
	}

	g := git.New(cwd)
	branch, err := g.CurrentBranch()
	if err != nil {
		return err
	}

	// Verify branch is tracked
	if _, err := cfg.GetParent(branch); err != nil {
		return fmt.Errorf("branch %q is not tracked", branch)
	}

	if err := cfg.SetPR(branch, prNumber); err != nil {
		return err
	}

	fmt.Printf("Linked PR #%d to branch %q\n", prNumber, branch)
	return nil
}
```

**Step 2: Build and verify**

Run:
```bash
go build -o gh-stack .
```

**Step 3: Commit**

```bash
git add cmd/link.go
git commit -m "feat(cmd): add link command"
```

---

### Task 7.2: Unlink Command

**Files:**
- Create: `cmd/unlink.go`

**Step 1: Implement unlink command**

```go
// cmd/unlink.go
package cmd

import (
	"fmt"
	"os"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/git"
	"github.com/spf13/cobra"
)

var unlinkCmd = &cobra.Command{
	Use:   "unlink",
	Short: "Remove PR association from current branch",
	Long:  `Remove the GitHub PR number association from the current branch.`,
	RunE:  runUnlink,
}

func init() {
	rootCmd.AddCommand(unlinkCmd)
}

func runUnlink(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	cfg, err := config.Load(cwd)
	if err != nil {
		return err
	}

	g := git.New(cwd)
	branch, err := g.CurrentBranch()
	if err != nil {
		return err
	}

	if err := cfg.RemovePR(branch); err != nil {
		return err
	}

	fmt.Printf("Unlinked PR from branch %q\n", branch)
	return nil
}
```

**Step 2: Build and verify**

Run:
```bash
go build -o gh-stack .
```

**Step 3: Commit**

```bash
git add cmd/unlink.go
git commit -m "feat(cmd): add unlink command"
```

---

### Task 7.3: PR Command

**Files:**
- Create: `cmd/pr.go`
- Modify: `internal/github/github.go`

**Step 1: Implement github package for PR creation**

```go
// internal/github/github.go
package github

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// PR represents a GitHub pull request.
type PR struct {
	Number int    `json:"number"`
	State  string `json:"state"`
	Merged bool   `json:"merged"`
}

// CreatePR creates a new pull request and returns the PR number.
func CreatePR(base, title, body string) (int, error) {
	args := []string{"pr", "create", "--base", base, "--title", title, "--body", body}
	out, err := exec.Command("gh", args...).Output()
	if err != nil {
		return 0, fmt.Errorf("gh pr create failed: %w", err)
	}

	// Output is the PR URL, extract the number
	url := strings.TrimSpace(string(out))
	parts := strings.Split(url, "/")
	if len(parts) == 0 {
		return 0, fmt.Errorf("unexpected output: %s", url)
	}
	return strconv.Atoi(parts[len(parts)-1])
}

// GetPR fetches PR details by number.
func GetPR(number int) (*PR, error) {
	out, err := exec.Command("gh", "pr", "view", strconv.Itoa(number), "--json", "number,state,merged").Output()
	if err != nil {
		return nil, err
	}

	var pr PR
	if err := json.Unmarshal(out, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

// UpdatePRBase updates the base branch of a PR.
func UpdatePRBase(number int, base string) error {
	return exec.Command("gh", "pr", "edit", strconv.Itoa(number), "--base", base).Run()
}
```

**Step 2: Implement pr command**

```go
// cmd/pr.go
package cmd

import (
	"fmt"
	"os"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/git"
	"github.com/boneskull/gh-stack/internal/github"
	"github.com/spf13/cobra"
)

var prCmd = &cobra.Command{
	Use:   "pr",
	Short: "Create or update a PR for the current branch",
	Long:  `Create a new PR targeting the parent branch, or update an existing PR's base.`,
	RunE:  runPR,
}

var prBaseFlag string

func init() {
	prCmd.Flags().StringVar(&prBaseFlag, "base", "", "override base branch")
	rootCmd.AddCommand(prCmd)
}

func runPR(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	cfg, err := config.Load(cwd)
	if err != nil {
		return err
	}

	g := git.New(cwd)
	branch, err := g.CurrentBranch()
	if err != nil {
		return err
	}

	// Get parent (base branch)
	parent, err := cfg.GetParent(branch)
	if err != nil {
		return fmt.Errorf("branch %q is not tracked", branch)
	}

	base := prBaseFlag
	if base == "" {
		base = parent
	}

	// Check if PR already exists
	existingPR, _ := cfg.GetPR(branch)
	if existingPR > 0 {
		// Update existing PR's base if needed
		fmt.Printf("PR #%d already exists, updating base to %q\n", existingPR, base)
		if err := github.UpdatePRBase(existingPR, base); err != nil {
			return fmt.Errorf("failed to update PR base: %w", err)
		}
		return nil
	}

	// Create new PR
	fmt.Printf("Creating PR for %q targeting %q...\n", branch, base)
	prNumber, err := github.CreatePR(base, branch, "")
	if err != nil {
		return err
	}

	// Store PR number
	if err := cfg.SetPR(branch, prNumber); err != nil {
		return err
	}

	fmt.Printf("Created PR #%d\n", prNumber)
	return nil
}
```

**Step 3: Build and verify**

Run:
```bash
go build -o gh-stack .
```

**Step 4: Commit**

```bash
git add cmd/pr.go internal/github/github.go
git commit -m "feat(cmd): add pr command with GitHub integration"
```

---

## Phase 8: Push Command

### Task 8.1: Push Command Implementation

**Files:**
- Create: `cmd/push.go`
- Modify: `internal/git/git.go`

**Step 1: Add push methods to git package**

Add to `internal/git/git.go`:

```go
// Push force-pushes a branch to origin with lease.
func (g *Git) Push(branch string, force bool) error {
	args := []string{"-C", g.repoPath, "push", "origin", branch}
	if force {
		args = append(args, "--force-with-lease")
	}
	cmd := exec.Command("git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
```

Add import for `os` at the top of git.go.

**Step 2: Implement push command**

```go
// cmd/push.go
package cmd

import (
	"fmt"
	"os"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/git"
	"github.com/boneskull/gh-stack/internal/github"
	"github.com/boneskull/gh-stack/internal/tree"
	"github.com/spf13/cobra"
)

var pushCmd = &cobra.Command{
	Use:   "push",
	Short: "Force-push branches from trunk to current branch",
	Long:  `Force-push all branches in the stack from trunk to current branch, updating PR base branches as needed.`,
	RunE:  runPush,
}

var pushDryRunFlag bool

func init() {
	pushCmd.Flags().BoolVar(&pushDryRunFlag, "dry-run", false, "show what would be pushed without pushing")
	rootCmd.AddCommand(pushCmd)
}

func runPush(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	cfg, err := config.Load(cwd)
	if err != nil {
		return err
	}

	g := git.New(cwd)

	currentBranch, err := g.CurrentBranch()
	if err != nil {
		return err
	}

	// Build tree
	root, err := tree.Build(cfg)
	if err != nil {
		return err
	}

	// Find current branch in tree
	node := tree.FindNode(root, currentBranch)
	if node == nil {
		return fmt.Errorf("branch %q is not tracked", currentBranch)
	}

	// Get downstack (ancestors from current to trunk, reversed)
	ancestors := tree.GetAncestors(node)
	trunk, _ := cfg.GetTrunk()

	// Build list: current + ancestors (excluding trunk)
	var branches []*tree.Node
	branches = append(branches, node)
	for _, a := range ancestors {
		if a.Name != trunk {
			branches = append(branches, a)
		}
	}

	// Reverse to go from trunk-adjacent to current
	for i, j := 0, len(branches)-1; i < j; i, j = i+1, j-1 {
		branches[i], branches[j] = branches[j], branches[i]
	}

	// Update PR bases and push
	for _, b := range branches {
		parent, _ := cfg.GetParent(b.Name)

		// Update PR base if needed
		if b.PR > 0 {
			if pushDryRunFlag {
				fmt.Printf("Would update PR #%d base to %q\n", b.PR, parent)
			} else {
				fmt.Printf("Updating PR #%d base to %q\n", b.PR, parent)
				if err := github.UpdatePRBase(b.PR, parent); err != nil {
					fmt.Printf("Warning: failed to update PR base: %v\n", err)
				}
			}
		}

		// Push
		if pushDryRunFlag {
			fmt.Printf("Would push %s -> origin/%s (forced)\n", b.Name, b.Name)
		} else {
			fmt.Printf("Pushing %s -> origin/%s (forced)\n", b.Name, b.Name)
			if err := g.Push(b.Name, true); err != nil {
				return fmt.Errorf("failed to push %s: %w", b.Name, err)
			}
		}
	}

	return nil
}
```

**Step 3: Build and verify**

Run:
```bash
go build -o gh-stack .
```

**Step 4: Commit**

```bash
git add cmd/push.go internal/git/git.go
git commit -m "feat(cmd): add push command"
```

---

## Phase 9: Cascade System (cascade, continue, abort)

### Task 9.1: State Package for Cascade Persistence

**Files:**
- Modify: `internal/state/state.go`
- Create: `internal/state/state_test.go`

**Step 1: Write failing test for state persistence**

```go
// internal/state/state_test.go
package state_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/boneskull/gh-stack/internal/state"
)

func TestCascadeState(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	os.Mkdir(gitDir, 0755)

	s := &state.CascadeState{
		Current:      "feature-b",
		Pending:      []string{"feature-c", "feature-d"},
		OriginalHead: "abc123",
	}

	err := state.Save(gitDir, s)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := state.Load(gitDir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.Current != s.Current {
		t.Errorf("Current mismatch: %q != %q", loaded.Current, s.Current)
	}
	if len(loaded.Pending) != len(s.Pending) {
		t.Errorf("Pending length mismatch")
	}
}

func TestCascadeStateNotExists(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	os.Mkdir(gitDir, 0755)

	_, err := state.Load(gitDir)
	if err == nil {
		t.Error("expected error when state doesn't exist")
	}
}
```

**Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/state/... -v
```

Expected: FAIL

**Step 3: Implement state package**

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

// ErrNoState is returned when no cascade state exists.
var ErrNoState = errors.New("no cascade in progress")

// CascadeState represents the state of an in-progress cascade operation.
type CascadeState struct {
	Current      string   `json:"current"`
	Pending      []string `json:"pending"`
	OriginalHead string   `json:"original_head"`
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

Run:
```bash
go test ./internal/state/... -v
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/state/
git commit -m "feat(state): add cascade state persistence"
```

---

### Task 9.2: Add Rebase Methods to Git Package

**Files:**
- Modify: `internal/git/git.go`

**Step 1: Add rebase methods**

Add to `internal/git/git.go`:

```go
// GetMergeBase returns the merge base of two branches.
func (g *Git) GetMergeBase(a, b string) (string, error) {
	out, err := exec.Command("git", "-C", g.repoPath, "merge-base", a, b).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// GetTip returns the commit SHA at the tip of a branch.
func (g *Git) GetTip(branch string) (string, error) {
	out, err := exec.Command("git", "-C", g.repoPath, "rev-parse", branch).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// NeedsRebase returns true if branch needs to be rebased onto parent.
func (g *Git) NeedsRebase(branch, parent string) (bool, error) {
	mergeBase, err := g.GetMergeBase(branch, parent)
	if err != nil {
		return false, err
	}
	parentTip, err := g.GetTip(parent)
	if err != nil {
		return false, err
	}
	return mergeBase != parentTip, nil
}

// Rebase rebases the current branch onto target.
func (g *Git) Rebase(onto string) error {
	cmd := exec.Command("git", "-C", g.repoPath, "rebase", onto)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// RebaseContinue continues an in-progress rebase.
func (g *Git) RebaseContinue() error {
	cmd := exec.Command("git", "-C", g.repoPath, "rebase", "--continue")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// RebaseAbort aborts an in-progress rebase.
func (g *Git) RebaseAbort() error {
	return exec.Command("git", "-C", g.repoPath, "rebase", "--abort").Run()
}

// IsRebaseInProgress checks if a rebase is in progress.
func (g *Git) IsRebaseInProgress() bool {
	rebaseMerge := filepath.Join(g.repoPath, ".git", "rebase-merge")
	rebaseApply := filepath.Join(g.repoPath, ".git", "rebase-apply")
	_, err1 := os.Stat(rebaseMerge)
	_, err2 := os.Stat(rebaseApply)
	return err1 == nil || err2 == nil
}

// GetGitDir returns the .git directory path.
func (g *Git) GetGitDir() string {
	return filepath.Join(g.repoPath, ".git")
}
```

Add `path/filepath` to imports.

**Step 2: Build and verify**

Run:
```bash
go build -o gh-stack .
```

**Step 3: Commit**

```bash
git add internal/git/git.go
git commit -m "feat(git): add rebase operations"
```

---

### Task 9.3: Cascade Command

**Files:**
- Create: `cmd/cascade.go`

**Step 1: Implement cascade command**

```go
// cmd/cascade.go
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

var cascadeCmd = &cobra.Command{
	Use:   "cascade",
	Short: "Rebase current branch and descendants onto their parents",
	Long:  `Rebase the current branch onto its parent, then recursively cascade to descendants.`,
	RunE:  runCascade,
}

var (
	cascadeOnlyFlag   bool
	cascadeDryRunFlag bool
)

func init() {
	cascadeCmd.Flags().BoolVar(&cascadeOnlyFlag, "only", false, "only cascade current branch, not descendants")
	cascadeCmd.Flags().BoolVar(&cascadeDryRunFlag, "dry-run", false, "show what would be done")
	rootCmd.AddCommand(cascadeCmd)
}

func runCascade(cmd *cobra.Command, args []string) error {
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

	// Check if cascade already in progress
	if state.Exists(g.GetGitDir()) {
		return fmt.Errorf("cascade already in progress; use 'gh stack continue' or 'gh stack abort'")
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

	// Collect branches to cascade
	var branches []*tree.Node
	branches = append(branches, node)
	if !cascadeOnlyFlag {
		branches = append(branches, tree.GetDescendants(node)...)
	}

	return doCascade(g, cfg, branches, cascadeDryRunFlag)
}

func doCascade(g *git.Git, cfg *config.Config, branches []*tree.Node, dryRun bool) error {
	originalBranch, _ := g.CurrentBranch()
	originalHead, _ := g.GetTip(originalBranch)

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
			}
			state.Save(g.GetGitDir(), st)

			fmt.Printf("\nCONFLICT: Resolve conflicts and run 'gh stack continue', or 'gh stack abort' to cancel.\n")
			fmt.Printf("Remaining branches: %v\n", remaining)
			return nil
		}

		fmt.Printf("Cascading %s... ok\n", b.Name)
	}

	// Return to original branch
	if !dryRun {
		g.Checkout(originalBranch)
	}

	return nil
}
```

**Step 2: Build and verify**

Run:
```bash
go build -o gh-stack .
```

**Step 3: Commit**

```bash
git add cmd/cascade.go
git commit -m "feat(cmd): add cascade command"
```

---

### Task 9.4: Continue Command

**Files:**
- Create: `cmd/continue.go`

**Step 1: Implement continue command**

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
	Short: "Continue a cascade after resolving conflicts",
	Long:  `Continue a cascade operation after resolving rebase conflicts.`,
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

	// Check if cascade in progress
	st, err := state.Load(g.GetGitDir())
	if err != nil {
		return fmt.Errorf("no cascade in progress")
	}

	// Complete the in-progress rebase
	if g.IsRebaseInProgress() {
		fmt.Println("Continuing rebase...")
		if err := g.RebaseContinue(); err != nil {
			return fmt.Errorf("rebase --continue failed; resolve conflicts first")
		}
	}

	fmt.Printf("Completed %s\n", st.Current)

	// Continue with remaining branches
	if len(st.Pending) == 0 {
		state.Remove(g.GetGitDir())
		fmt.Println("Cascade complete!")
		return nil
	}

	cfg, err := config.Load(cwd)
	if err != nil {
		return err
	}

	// Build tree to get node objects
	root, err := tree.Build(cfg)
	if err != nil {
		return err
	}

	var branches []*tree.Node
	for _, name := range st.Pending {
		if node := tree.FindNode(root, name); node != nil {
			branches = append(branches, node)
		}
	}

	// Remove state file before continuing (will be recreated if conflict)
	state.Remove(g.GetGitDir())

	return doCascade(g, cfg, branches, false)
}
```

**Step 2: Build and verify**

Run:
```bash
go build -o gh-stack .
```

**Step 3: Commit**

```bash
git add cmd/continue.go
git commit -m "feat(cmd): add continue command"
```

---

### Task 9.5: Abort Command

**Files:**
- Create: `cmd/abort.go`

**Step 1: Implement abort command**

```go
// cmd/abort.go
package cmd

import (
	"fmt"
	"os"

	"github.com/boneskull/gh-stack/internal/git"
	"github.com/boneskull/gh-stack/internal/state"
	"github.com/spf13/cobra"
)

var abortCmd = &cobra.Command{
	Use:   "abort",
	Short: "Abort a cascade in progress",
	Long:  `Abort a cascade operation and restore the original state.`,
	RunE:  runAbort,
}

func init() {
	rootCmd.AddCommand(abortCmd)
}

func runAbort(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	g := git.New(cwd)

	// Check if cascade in progress
	st, err := state.Load(g.GetGitDir())
	if err != nil {
		return fmt.Errorf("no cascade in progress")
	}

	// Abort rebase if in progress
	if g.IsRebaseInProgress() {
		fmt.Println("Aborting rebase...")
		if err := g.RebaseAbort(); err != nil {
			return fmt.Errorf("failed to abort rebase: %w", err)
		}
	}

	// Clean up state
	state.Remove(g.GetGitDir())

	fmt.Printf("Cascade aborted. Original HEAD was %s\n", st.OriginalHead)
	return nil
}
```

**Step 2: Build and verify**

Run:
```bash
go build -o gh-stack .
```

**Step 3: Commit**

```bash
git add cmd/abort.go
git commit -m "feat(cmd): add abort command"
```

---

## Phase 10: Sync Command

### Task 10.1: Sync Command Implementation

**Files:**
- Create: `cmd/sync.go`
- Modify: `internal/git/git.go`

**Step 1: Add fetch and fast-forward methods**

Add to `internal/git/git.go`:

```go
// Fetch fetches from origin.
func (g *Git) Fetch() error {
	cmd := exec.Command("git", "-C", g.repoPath, "fetch", "origin")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// FastForward fast-forwards a branch to its remote tracking branch.
func (g *Git) FastForward(branch string) error {
	// First checkout the branch
	if err := g.Checkout(branch); err != nil {
		return err
	}
	// Then merge with fast-forward only
	return exec.Command("git", "-C", g.repoPath, "merge", "--ff-only", "origin/"+branch).Run()
}

// DeleteBranch deletes a local branch.
func (g *Git) DeleteBranch(branch string) error {
	return exec.Command("git", "-C", g.repoPath, "branch", "-D", branch).Run()
}
```

**Step 2: Implement sync command**

```go
// cmd/sync.go
package cmd

import (
	"fmt"
	"os"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/git"
	"github.com/boneskull/gh-stack/internal/github"
	"github.com/boneskull/gh-stack/internal/tree"
	"github.com/spf13/cobra"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Fetch, detect merged PRs, retarget orphaned branches, cascade all",
	Long:  `Fetch from origin, detect merged PRs, retarget orphaned branches to trunk, and cascade all branches.`,
	RunE:  runSync,
}

var (
	syncNoCascadeFlag bool
	syncDryRunFlag    bool
)

func init() {
	syncCmd.Flags().BoolVar(&syncNoCascadeFlag, "no-cascade", false, "skip cascading branches")
	syncCmd.Flags().BoolVar(&syncDryRunFlag, "dry-run", false, "show what would be done")
	rootCmd.AddCommand(syncCmd)
}

func runSync(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	cfg, err := config.Load(cwd)
	if err != nil {
		return err
	}

	g := git.New(cwd)

	trunk, err := cfg.GetTrunk()
	if err != nil {
		return err
	}

	// Fetch
	fmt.Println("Fetching from origin...")
	if !syncDryRunFlag {
		if err := g.Fetch(); err != nil {
			return fmt.Errorf("fetch failed: %w", err)
		}
	}

	// Fast-forward trunk
	currentBranch, _ := g.CurrentBranch()
	fmt.Printf("Fast-forwarding %s...\n", trunk)
	if !syncDryRunFlag {
		if err := g.FastForward(trunk); err != nil {
			fmt.Printf("Warning: could not fast-forward %s: %v\n", trunk, err)
		}
		// Return to original branch
		g.Checkout(currentBranch)
	}

	// Check for merged PRs
	branches, err := cfg.ListTrackedBranches()
	if err != nil {
		return err
	}

	var merged []string
	for _, branch := range branches {
		prNum, err := cfg.GetPR(branch)
		if err != nil || prNum == 0 {
			continue
		}

		pr, err := github.GetPR(prNum)
		if err != nil {
			fmt.Printf("Warning: could not fetch PR #%d: %v\n", prNum, err)
			continue
		}

		if pr.Merged {
			merged = append(merged, branch)
		}
	}

	// Handle merged branches
	root, _ := tree.Build(cfg)
	for _, branch := range merged {
		node := tree.FindNode(root, branch)
		if node == nil {
			continue
		}

		// Retarget children to trunk
		for _, child := range node.Children {
			if syncDryRunFlag {
				fmt.Printf("Would retarget %s from %s to %s\n", child.Name, branch, trunk)
			} else {
				fmt.Printf("Retargeting %s from %s to %s\n", child.Name, branch, trunk)
				cfg.SetParent(child.Name, trunk)
			}
		}

		// Prompt to delete merged branch
		if syncDryRunFlag {
			fmt.Printf("Would delete merged branch %s\n", branch)
		} else {
			fmt.Printf("Deleting merged branch %s (PR was merged)\n", branch)
			cfg.RemoveParent(branch)
			cfg.RemovePR(branch)
			g.DeleteBranch(branch)
		}
	}

	// Cascade all (if not disabled)
	if !syncNoCascadeFlag {
		fmt.Println("\nCascading all branches...")
		// Rebuild tree after modifications
		root, err = tree.Build(cfg)
		if err != nil {
			return err
		}

		// Cascade from trunk's children
		for _, child := range root.Children {
			allBranches := []*tree.Node{child}
			allBranches = append(allBranches, tree.GetDescendants(child)...)
			if err := doCascade(g, cfg, allBranches, syncDryRunFlag); err != nil {
				return err
			}
		}
	}

	fmt.Println("\nSync complete!")
	return nil
}
```

**Step 3: Build and verify**

Run:
```bash
go build -o gh-stack .
```

**Step 4: Commit**

```bash
git add cmd/sync.go internal/git/git.go
git commit -m "feat(cmd): add sync command"
```

---

## Phase 11: Final Polish

### Task 11.1: Add Version Command

**Files:**
- Modify: `cmd/root.go`

**Step 1: Add version flag**

Modify `cmd/root.go`:

```go
// At the top, add version variable
var version = "dev"

// In init(), add:
func init() {
	rootCmd.Version = version
}
```

**Step 2: Commit**

```bash
git add cmd/root.go
git commit -m "feat(cmd): add version flag"
```

---

### Task 11.2: Add Makefile

**Files:**
- Create: `Makefile`

**Step 1: Create Makefile**

```makefile
.PHONY: build test install clean

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X github.com/boneskull/gh-stack/cmd.version=$(VERSION)"

build:
	go build $(LDFLAGS) -o gh-stack .

test:
	go test ./... -v

install:
	go install $(LDFLAGS) .

clean:
	rm -f gh-stack

# Install as gh extension
gh-install: build
	mkdir -p ~/.local/share/gh/extensions/gh-stack
	cp gh-stack ~/.local/share/gh/extensions/gh-stack/
```

**Step 2: Commit**

```bash
git add Makefile
git commit -m "chore: add Makefile"
```

---

### Task 11.3: Run All Tests

**Step 1: Run full test suite**

Run:
```bash
go test ./... -v
```

Expected: All tests pass

**Step 2: Build and test manually**

Run:
```bash
make build
./gh-stack --help
./gh-stack --version
```

---

### Task 11.4: Final Commit

**Step 1: Commit any remaining changes**

```bash
git status
git add .
git commit -m "chore: finalize initial implementation"
```

---

## Summary

This plan implements the complete gh-stack CLI with:

1. **Phase 1-2**: Project foundation and config package (data model)
2. **Phase 3**: Tree package (stack traversal)
3. **Phase 4**: Git package (git operations)
4. **Phase 5**: Basic commands (init, log)
5. **Phase 6**: Branch management (create, adopt, orphan)
6. **Phase 7**: PR commands (link, unlink, pr)
7. **Phase 8**: Push command
8. **Phase 9**: Cascade system (cascade, continue, abort)
9. **Phase 10**: Sync command
10. **Phase 11**: Polish (version, Makefile, tests)

Each task follows TDD: write failing test, implement minimal code, verify pass, commit.
