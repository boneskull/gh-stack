# Squash-Merge Handling Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Handle squash-merged parent PRs gracefully by using `git rebase --onto` instead of simple rebase, preventing false conflicts when syncing stacked branches.

**Architecture:** Store the fork point SHA (where child branched from parent) in git config. Use this SHA with `git rebase --onto <new-base> <fork-point> <child>` when retargeting branches after a parent is merged. Update the fork point after every successful cascade.

**Tech Stack:** Go, git CLI, git config storage

---

## Background

When a parent PR is squash-merged into trunk:

1. `sync` detects the merge and retargets child branches to trunk
2. `cascade` runs `git rebase trunk` on child branches
3. Git sees the squashed commit as unrelated to the original commits
4. Every commit that was in the parent causes a conflict

The fix: use `git rebase --onto trunk <fork-point> <child>` where `<fork-point>` is the commit where child originally branched from parent. This tells git to only replay commits unique to child.

---

## Phase 1: Fix Sync Flow (Calculate Fork Point On-Demand)

This phase fixes the immediate problem by calculating fork points before deleting merged branches.

### Task 1: Add RebaseOnto Method to Git Package

**Files:**

- Modify: `internal/git/git.go`
- Test: `internal/git/git_test.go`

**Step 1: Write the failing test**

Add to `internal/git/git_test.go`:

```go
func TestRebaseOnto(t *testing.T) {
	repo := setupTestRepo(t)
	g := New(repo)

	// Create initial commit on main
	writeFile(t, repo, "file.txt", "initial")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "initial")

	// Create parent branch with a commit
	runGit(t, repo, "checkout", "-b", "parent")
	writeFile(t, repo, "parent.txt", "parent content")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "parent commit")
	parentTip, _ := g.GetTip("parent")

	// Create child branch with a commit
	runGit(t, repo, "checkout", "-b", "child")
	writeFile(t, repo, "child.txt", "child content")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "child commit")

	// Go back to main and add a new commit (simulating trunk moving forward)
	runGit(t, repo, "checkout", "main")
	writeFile(t, repo, "main2.txt", "main moved forward")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "main moved forward")

	// Now rebase child onto main, using parent tip as the fork point
	// This should only replay "child commit", not "parent commit"
	err := g.RebaseOnto("main", parentTip, "child")
	if err != nil {
		t.Fatalf("RebaseOnto failed: %v", err)
	}

	// Verify child is now based on main
	runGit(t, repo, "checkout", "child")
	mergeBase, _ := g.GetMergeBase("child", "main")
	mainTip, _ := g.GetTip("main")
	if mergeBase != mainTip {
		t.Errorf("child should be based on main tip, got merge-base %s, main tip %s", mergeBase, mainTip)
	}

	// Verify child.txt exists (child's commit was replayed)
	if _, err := os.Stat(filepath.Join(repo, "child.txt")); err != nil {
		t.Error("child.txt should exist after rebase")
	}

	// Verify parent.txt does NOT exist (parent's commit was not replayed)
	if _, err := os.Stat(filepath.Join(repo, "parent.txt")); os.IsNotExist(err) {
		// This is expected - parent.txt should not be on child after --onto rebase
	} else {
		t.Error("parent.txt should NOT exist - only child's commits should be replayed")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run TestRebaseOnto ./internal/git`
Expected: FAIL with "g.RebaseOnto undefined"

**Step 3: Write minimal implementation**

Add to `internal/git/git.go` after the `Rebase` method:

```go
// RebaseOnto rebases a branch onto a new base, replaying only commits after oldBase.
// This is equivalent to: git rebase --onto <newBase> <oldBase> <branch>
// Useful when a parent branch was squash-merged and we need to replay only
// the commits unique to the child branch.
func (g *Git) RebaseOnto(newBase, oldBase, branch string) error {
	// First checkout the branch to rebase
	if err := g.Checkout(branch); err != nil {
		return err
	}
	return g.runInteractive("rebase", "--onto", newBase, oldBase)
}
```

**Step 4: Run test to verify it passes**

Run: `go test -v -run TestRebaseOnto ./internal/git`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/git/git.go internal/git/git_test.go
git commit -m "feat(git): add RebaseOnto method for --onto rebases

Enables rebasing a branch onto a new base while specifying the old
fork point. This is needed to handle squash-merged parent branches
without generating false conflicts.
"
```

---

### Task 2: Add CommitExists Method for Validation

**Files:**

- Modify: `internal/git/git.go`
- Test: `internal/git/git_test.go`

**Step 1: Write the failing test**

Add to `internal/git/git_test.go`:

```go
func TestCommitExists(t *testing.T) {
	repo := setupTestRepo(t)
	g := New(repo)

	// Create a commit
	writeFile(t, repo, "file.txt", "content")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "test commit")
	sha, _ := g.GetTip("HEAD")

	// Valid SHA should exist
	if !g.CommitExists(sha) {
		t.Errorf("CommitExists(%s) = false, want true", sha)
	}

	// Invalid SHA should not exist
	if g.CommitExists("0000000000000000000000000000000000000000") {
		t.Error("CommitExists(invalid) = true, want false")
	}

	// Garbage input should not exist
	if g.CommitExists("not-a-sha") {
		t.Error("CommitExists(garbage) = true, want false")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run TestCommitExists ./internal/git`
Expected: FAIL with "g.CommitExists undefined"

**Step 3: Write minimal implementation**

Add to `internal/git/git.go`:

```go
// CommitExists checks if a commit SHA exists in the repository.
func (g *Git) CommitExists(sha string) bool {
	err := g.runSilent("cat-file", "-e", sha+"^{commit}")
	return err == nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test -v -run TestCommitExists ./internal/git`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/git/git.go internal/git/git_test.go
git commit -m "feat(git): add CommitExists for SHA validation

Used to verify a stored fork point SHA is still valid before
attempting a rebase --onto operation.
"
```

---

### Task 3: Modify Sync to Use --onto When Retargeting

**Files:**

- Modify: `cmd/sync.go`

**Step 1: Review current sync flow**

The current flow in `runSync` (lines 144-197):

1. Detects merged branches
2. For each merged branch's children:
   - Retargets child to trunk via `cfg.SetParent`
   - Updates PR base on GitHub
3. Deletes merged branch
4. Runs cascade

The problem: cascade runs `git rebase trunk` which causes conflicts.

**Step 2: Write the failing e2e test**

Add to `e2e/sync_test.go` (create if doesn't exist):

```go
package e2e

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSyncSquashMerge(t *testing.T) {
	env := setupE2EEnv(t)
	defer env.Cleanup()

	// Initialize stack
	env.Run("gh", "stack", "init")

	// Create parent branch with multiple commits
	env.Run("git", "checkout", "-b", "parent")
	env.WriteFile("parent1.txt", "content1")
	env.Run("git", "add", ".")
	env.Run("git", "commit", "-m", "parent commit 1")
	env.WriteFile("parent2.txt", "content2")
	env.Run("git", "add", ".")
	env.Run("git", "commit", "-m", "parent commit 2")
	env.Run("gh", "stack", "create", "parent")

	// Create child branch
	env.Run("git", "checkout", "-b", "child")
	env.WriteFile("child.txt", "child content")
	env.Run("git", "add", ".")
	env.Run("git", "commit", "-m", "child commit")
	env.Run("gh", "stack", "create", "child")

	// Simulate squash-merge of parent into main:
	// 1. Checkout main
	// 2. Create a single "squashed" commit with parent's changes
	// 3. Mark parent's PR as merged (mock)
	env.Run("git", "checkout", "main")
	env.WriteFile("parent1.txt", "content1")
	env.WriteFile("parent2.txt", "content2")
	env.Run("git", "add", ".")
	env.Run("git", "commit", "-m", "Squashed: parent (#1)")

	// Mock the PR as merged (would normally come from GitHub API)
	// For e2e test, we simulate by setting up mock responses

	// Run sync - this should:
	// 1. Detect parent PR merged
	// 2. Retarget child to main using --onto rebase
	// 3. NOT produce conflicts
	err := env.RunMayFail("gh", "stack", "sync")
	if err != nil {
		t.Fatalf("sync failed (likely due to rebase conflicts): %v", err)
	}

	// Verify child is now based on main
	env.Run("git", "checkout", "child")

	// child.txt should exist
	if _, err := os.Stat(filepath.Join(env.RepoPath, "child.txt")); err != nil {
		t.Error("child.txt should exist")
	}

	// parent files should NOT exist on child branch (they're on main now)
	// Actually wait - after rebasing onto main, parent files WILL exist
	// because main has them. Let me reconsider...

	// The key test: no conflicts occurred and child has its unique changes
	// The parent files existing is fine - they're in main's history now
}
```

Note: This test needs the e2e framework. Check existing e2e tests for patterns.

**Step 3: Run test to verify it fails**

Run: `go test -v -run TestSyncSquashMerge ./e2e`
Expected: FAIL (conflicts during rebase)

**Step 4: Implement the fix in sync.go**

Modify `runSync` in `cmd/sync.go`. Replace the merged branch handling section (approximately lines 144-197):

```go
	// Handle merged branches
	root, _ := tree.Build(cfg) //nolint:errcheck // nil root is fine, FindNode handles it

	// Collect fork points BEFORE deleting merged branches
	type retargetInfo struct {
		childName  string
		forkPoint  string
		childPR    int
	}
	var retargets []retargetInfo

	for _, branch := range merged {
		node := tree.FindNode(root, branch)
		if node == nil {
			continue
		}

		// For each child, calculate fork point while parent still exists
		for _, child := range node.Children {
			forkPoint, fpErr := g.GetMergeBase(child.Name, branch)
			if fpErr != nil {
				fmt.Printf("Warning: could not get fork point for %s: %v\n", child.Name, fpErr)
				forkPoint = "" // Will fall back to simple rebase
			}
			childPR, _ := cfg.GetPR(child.Name) //nolint:errcheck // 0 is fine
			retargets = append(retargets, retargetInfo{
				childName: child.Name,
				forkPoint: forkPoint,
				childPR:   childPR,
			})
		}

		// Now safe to delete the merged branch
		if syncDryRunFlag {
			fmt.Printf("Would delete merged branch %s\n", branch)
		} else {
			fmt.Printf("Deleting merged branch %s (PR was merged)\n", branch)
			_ = cfg.RemoveParent(branch) //nolint:errcheck // best effort cleanup
			_ = cfg.RemovePR(branch)     //nolint:errcheck // best effort cleanup
			_ = g.DeleteBranch(branch)   //nolint:errcheck // best effort cleanup
		}
	}

	// Retarget children to trunk
	for _, rt := range retargets {
		if syncDryRunFlag {
			fmt.Printf("Would retarget %s to %s (fork point: %s)\n", rt.childName, trunk, rt.forkPoint)
			continue
		}

		fmt.Printf("Retargeting %s to %s\n", rt.childName, trunk)
		_ = cfg.SetParent(rt.childName, trunk) //nolint:errcheck // best effort

		// Update PR base on GitHub
		if rt.childPR > 0 {
			if updateErr := gh.UpdatePRBase(rt.childPR, trunk); updateErr != nil {
				fmt.Printf("Warning: failed to update PR #%d base: %v\n", rt.childPR, updateErr)
			}

			// Check if this was a draft and now targets trunk
			pr, getPRErr := gh.GetPR(rt.childPR)
			if getPRErr == nil && pr.Draft {
				fmt.Printf("PR #%d (%s) now targets %s.\n", rt.childPR, rt.childName, trunk)
				fmt.Print("Mark as ready for review? [y/N]: ")

				var response string
				if _, scanErr := fmt.Scanln(&response); scanErr == nil {
					if strings.ToLower(strings.TrimSpace(response)) == "y" {
						if readyErr := gh.MarkPRReady(rt.childPR); readyErr != nil {
							fmt.Printf("Warning: failed to mark PR ready: %v\n", readyErr)
						} else {
							fmt.Printf("PR #%d marked as ready for review.\n", rt.childPR)
						}
					}
				}
			}
		}

		// Rebase using --onto if we have a fork point
		if rt.forkPoint != "" && g.CommitExists(rt.forkPoint) {
			fmt.Printf("Rebasing %s onto %s (from fork point %s)...\n", rt.childName, trunk, rt.forkPoint[:8])
			if err := g.RebaseOnto(trunk, rt.forkPoint, rt.childName); err != nil {
				fmt.Printf("Warning: --onto rebase failed, will try normal cascade: %v\n", err)
				// Don't return error - let cascade try
			} else {
				fmt.Printf("Rebased %s successfully\n", rt.childName)
			}
		}
	}
```

**Step 5: Run test to verify it passes**

Run: `go test -v -run TestSyncSquashMerge ./e2e`
Expected: PASS

**Step 6: Run full test suite**

Run: `make test`
Expected: All tests pass

**Step 7: Commit**

```bash
git add cmd/sync.go
git commit -m "fix(sync): use --onto rebase for squash-merged parents

When a parent PR is squash-merged, the original commits become
unreachable from trunk. A simple 'git rebase trunk' would try to
replay those commits, causing conflicts.

Now sync calculates the fork point (where child branched from parent)
before deleting the merged branch, then uses 'git rebase --onto'
to replay only the child's unique commits.
"
```

---

## Phase 2: Persistent Fork Point Tracking

This phase adds fork point storage to git config, enabling `--onto` rebases in all scenarios (not just sync).

### Task 4: Add Fork Point Config Methods

**Files:**

- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Step 1: Write the failing test**

Add to `internal/config/config_test.go`:

```go
func TestForkPoint(t *testing.T) {
	repo := setupTestRepo(t)
	cfg, _ := Load(repo)

	// Initially no fork point
	_, err := cfg.GetForkPoint("feature")
	if err != ErrNoForkPoint {
		t.Errorf("GetForkPoint = %v, want ErrNoForkPoint", err)
	}

	// Set fork point
	sha := "abc123def456"
	if err := cfg.SetForkPoint("feature", sha); err != nil {
		t.Fatalf("SetForkPoint failed: %v", err)
	}

	// Get fork point
	got, err := cfg.GetForkPoint("feature")
	if err != nil {
		t.Fatalf("GetForkPoint failed: %v", err)
	}
	if got != sha {
		t.Errorf("GetForkPoint = %q, want %q", got, sha)
	}

	// Remove fork point
	if err := cfg.RemoveForkPoint("feature"); err != nil {
		t.Fatalf("RemoveForkPoint failed: %v", err)
	}

	// Verify removed
	_, err = cfg.GetForkPoint("feature")
	if err != ErrNoForkPoint {
		t.Errorf("after remove, GetForkPoint = %v, want ErrNoForkPoint", err)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run TestForkPoint ./internal/config`
Expected: FAIL with undefined errors

**Step 3: Write minimal implementation**

Add to `internal/config/config.go`:

```go
// ErrNoForkPoint is returned when a branch has no stored fork point.
var ErrNoForkPoint = errors.New("no fork point stored for branch")

// GetForkPoint returns the stored fork point SHA for a branch.
// The fork point is where the branch originally diverged from its parent.
func (c *Config) GetForkPoint(branch string) (string, error) {
	key := "branch." + branch + ".stackForkPoint"
	out, err := exec.Command("git", "-C", c.repoPath, "config", "--get", key).Output()
	if err != nil {
		return "", ErrNoForkPoint
	}
	return strings.TrimSpace(string(out)), nil
}

// SetForkPoint stores the fork point SHA for a branch.
func (c *Config) SetForkPoint(branch, sha string) error {
	key := "branch." + branch + ".stackForkPoint"
	return exec.Command("git", "-C", c.repoPath, "config", key, sha).Run()
}

// RemoveForkPoint removes the stored fork point for a branch.
func (c *Config) RemoveForkPoint(branch string) error {
	key := "branch." + branch + ".stackForkPoint"
	_ = exec.Command("git", "-C", c.repoPath, "config", "--unset", key).Run() //nolint:errcheck
	return nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test -v -run TestForkPoint ./internal/config`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): add fork point storage methods

Stores branch.<name>.stackForkPoint in git config to track where
a branch originally diverged from its parent. Used for --onto rebases.
"
```

---

### Task 5: Store Fork Point on Branch Creation

**Files:**

- Modify: `cmd/create.go`
- Test: `cmd/create_test.go`

**Step 1: Review current create flow**

Read `cmd/create.go` to understand where branch creation happens and where to add fork point storage.

**Step 2: Write the failing test**

Add to `cmd/create_test.go`:

```go
func TestCreateStoresForkPoint(t *testing.T) {
	// Setup test environment
	repo := setupCmdTestRepo(t)
	cfg, _ := config.Load(repo)
	g := git.New(repo)

	// Initialize stack
	runCmd(t, repo, "init")

	// Create a commit on main
	writeFile(t, repo, "file.txt", "content")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "initial")
	mainTip, _ := g.GetTip("main")

	// Create a branch
	runGit(t, repo, "checkout", "-b", "feature")
	writeFile(t, repo, "feature.txt", "feature")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "feature")

	// Run create command
	runCmd(t, repo, "create", "feature")

	// Verify fork point was stored
	forkPoint, err := cfg.GetForkPoint("feature")
	if err != nil {
		t.Fatalf("GetForkPoint failed: %v", err)
	}
	if forkPoint != mainTip {
		t.Errorf("fork point = %s, want %s (main tip at creation)", forkPoint, mainTip)
	}
}
```

**Step 3: Run test to verify it fails**

Run: `go test -v -run TestCreateStoresForkPoint ./cmd`
Expected: FAIL (fork point not stored)

**Step 4: Implement fork point storage in create**

In `cmd/create.go`, after setting the parent, add:

```go
	// Store fork point (where this branch diverges from parent)
	forkPoint, fpErr := g.GetMergeBase(branchName, parent)
	if fpErr == nil {
		_ = cfg.SetForkPoint(branchName, forkPoint) //nolint:errcheck // best effort
	}
```

**Step 5: Run test to verify it passes**

Run: `go test -v -run TestCreateStoresForkPoint ./cmd`
Expected: PASS

**Step 6: Commit**

```bash
git add cmd/create.go cmd/create_test.go
git commit -m "feat(create): store fork point when creating branches

Records where the branch diverges from its parent, enabling
--onto rebases when the parent is later squash-merged.
"
```

---

### Task 6: Store Fork Point on Link

**Files:**

- Modify: `cmd/link.go`
- Test: `cmd/link_test.go`

**Step 1: Write the failing test**

Add to `cmd/link_test.go`:

```go
func TestLinkStoresForkPoint(t *testing.T) {
	repo := setupCmdTestRepo(t)
	cfg, _ := config.Load(repo)
	g := git.New(repo)

	runCmd(t, repo, "init")

	// Create parent branch
	runGit(t, repo, "checkout", "-b", "parent")
	writeFile(t, repo, "parent.txt", "content")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "parent")
	runCmd(t, repo, "create", "parent")
	parentTip, _ := g.GetTip("parent")

	// Create child branch (untracked)
	runGit(t, repo, "checkout", "-b", "child")
	writeFile(t, repo, "child.txt", "content")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "child")

	// Link child to parent
	runCmd(t, repo, "link", "child", "parent")

	// Verify fork point
	forkPoint, err := cfg.GetForkPoint("child")
	if err != nil {
		t.Fatalf("GetForkPoint failed: %v", err)
	}
	if forkPoint != parentTip {
		t.Errorf("fork point = %s, want %s", forkPoint, parentTip)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run TestLinkStoresForkPoint ./cmd`
Expected: FAIL

**Step 3: Implement in link.go**

Add fork point storage after setting parent in `cmd/link.go`:

```go
	// Store fork point
	forkPoint, fpErr := g.GetMergeBase(branch, parent)
	if fpErr == nil {
		_ = cfg.SetForkPoint(branch, forkPoint) //nolint:errcheck
	}
```

**Step 4: Run test to verify it passes**

Run: `go test -v -run TestLinkStoresForkPoint ./cmd`
Expected: PASS

**Step 5: Commit**

```bash
git add cmd/link.go cmd/link_test.go
git commit -m "feat(link): store fork point when linking branches"
```

---

### Task 7: Update Fork Point After Successful Cascade

**Files:**

- Modify: `cmd/cascade.go`

**Step 1: Write the failing test**

Add to `e2e/cascade_test.go`:

```go
func TestCascadeUpdatesForkPoint(t *testing.T) {
	env := setupE2EEnv(t)
	defer env.Cleanup()

	env.Run("gh", "stack", "init")

	// Create parent with initial commit
	env.Run("git", "checkout", "-b", "parent")
	env.WriteFile("parent.txt", "v1")
	env.Run("git", "add", ".")
	env.Run("git", "commit", "-m", "parent v1")
	env.Run("gh", "stack", "create", "parent")

	// Create child
	env.Run("git", "checkout", "-b", "child")
	env.WriteFile("child.txt", "content")
	env.Run("git", "add", ".")
	env.Run("git", "commit", "-m", "child")
	env.Run("gh", "stack", "create", "child")

	// Get initial fork point
	initialFP := env.GetConfig("branch.child.stackForkPoint")

	// Add commit to parent
	env.Run("git", "checkout", "parent")
	env.WriteFile("parent.txt", "v2")
	env.Run("git", "add", ".")
	env.Run("git", "commit", "-m", "parent v2")
	newParentTip := env.GetTip("parent")

	// Cascade child
	env.Run("git", "checkout", "child")
	env.Run("gh", "stack", "cascade")

	// Fork point should be updated to new parent tip
	updatedFP := env.GetConfig("branch.child.stackForkPoint")
	if updatedFP == initialFP {
		t.Error("fork point should have been updated after cascade")
	}
	if updatedFP != newParentTip {
		t.Errorf("fork point = %s, want %s (parent tip)", updatedFP, newParentTip)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run TestCascadeUpdatesForkPoint ./e2e`
Expected: FAIL

**Step 3: Implement fork point update in cascade**

In `doCascade` in `cmd/cascade.go`, after successful rebase (line ~148):

```go
		fmt.Printf("Cascading %s... ok\n", b.Name)

		// Update fork point to current parent tip
		parentTip, tipErr := g.GetTip(parent)
		if tipErr == nil {
			_ = cfg.SetForkPoint(b.Name, parentTip) //nolint:errcheck
		}
```

**Step 4: Run test to verify it passes**

Run: `go test -v -run TestCascadeUpdatesForkPoint ./e2e`
Expected: PASS

**Step 5: Commit**

```bash
git add cmd/cascade.go
git commit -m "feat(cascade): update fork point after successful rebase

Keeps the fork point fresh so future --onto rebases use the
correct base commit.
"
```

---

### Task 8: Use Stored Fork Point in Cascade

**Files:**

- Modify: `cmd/cascade.go`

This makes cascade use `--onto` when a stored fork point exists and is different from a simple rebase scenario.

**Step 1: Write test for --onto cascade**

```go
func TestCascadeUsesOntoWhenNeeded(t *testing.T) {
	// This tests the scenario where parent was rebased/amended
	// and child has a stale fork point
	env := setupE2EEnv(t)
	defer env.Cleanup()

	env.Run("gh", "stack", "init")

	// Create parent
	env.Run("git", "checkout", "-b", "parent")
	env.WriteFile("parent.txt", "content")
	env.Run("git", "add", ".")
	env.Run("git", "commit", "-m", "parent")
	env.Run("gh", "stack", "create", "parent")

	// Create child
	env.Run("git", "checkout", "-b", "child")
	env.WriteFile("child.txt", "content")
	env.Run("git", "add", ".")
	env.Run("git", "commit", "-m", "child")
	env.Run("gh", "stack", "create", "child")

	// Amend parent (simulating a force-push scenario)
	env.Run("git", "checkout", "parent")
	env.WriteFile("parent.txt", "amended content")
	env.Run("git", "add", ".")
	env.Run("git", "commit", "--amend", "-m", "parent amended")

	// Cascade child - should use stored fork point
	env.Run("git", "checkout", "child")
	err := env.RunMayFail("gh", "stack", "cascade")
	if err != nil {
		t.Fatalf("cascade failed: %v", err)
	}

	// Verify child is now based on amended parent
	// and child.txt still exists
	if _, statErr := os.Stat(filepath.Join(env.RepoPath, "child.txt")); statErr != nil {
		t.Error("child.txt should exist")
	}
}
```

**Step 2: Run test to verify behavior**

Run: `go test -v -run TestCascadeUsesOntoWhenNeeded ./e2e`

**Step 3: Implement --onto in cascade**

Modify the rebase section in `doCascade`:

```go
		// Check if we should use --onto rebase
		// This is needed when parent has been rebased/amended since child was created
		storedForkPoint, fpErr := cfg.GetForkPoint(b.Name)
		useOnto := false

		if fpErr == nil && g.CommitExists(storedForkPoint) {
			// We have a valid stored fork point
			// Use --onto if the stored fork point differs from merge-base
			currentMergeBase, mbErr := g.GetMergeBase(b.Name, parent)
			if mbErr == nil && currentMergeBase != storedForkPoint {
				useOnto = true
			}
		}

		if useOnto {
			fmt.Printf("Cascading %s onto %s (using fork point)...\n", b.Name, parent)
			if err := g.RebaseOnto(parent, storedForkPoint, b.Name); err != nil {
				// Save state for recovery...
				// (existing conflict handling code)
			}
		} else {
			fmt.Printf("Cascading %s onto %s...\n", b.Name, parent)
			if err := g.Rebase(parent); err != nil {
				// Save state for recovery...
				// (existing conflict handling code)
			}
		}
```

**Step 4: Run tests**

Run: `make test`
Expected: All pass

**Step 5: Commit**

```bash
git add cmd/cascade.go
git commit -m "feat(cascade): use --onto rebase when fork point available

When a stored fork point exists and differs from the current
merge-base, use 'git rebase --onto' to avoid replaying commits
that may have been amended or rebased in the parent.
"
```

---

### Task 9: Clean Up Fork Point on Unlink/Orphan

**Files:**

- Modify: `cmd/unlink.go`
- Modify: `cmd/orphan.go`
- Test: `cmd/unlink_test.go`

**Step 1: Add fork point removal to unlink**

In `cmd/unlink.go`, add after removing parent:

```go
	_ = cfg.RemoveForkPoint(branch) //nolint:errcheck
```

**Step 2: Add fork point removal to orphan**

In `cmd/orphan.go`, add after removing parent:

```go
	_ = cfg.RemoveForkPoint(branch) //nolint:errcheck
```

**Step 3: Write test**

```go
func TestUnlinkRemovesForkPoint(t *testing.T) {
	repo := setupCmdTestRepo(t)
	cfg, _ := config.Load(repo)

	// Setup tracked branch with fork point
	runCmd(t, repo, "init")
	runGit(t, repo, "checkout", "-b", "feature")
	writeFile(t, repo, "f.txt", "c")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "c")
	runCmd(t, repo, "create", "feature")

	// Verify fork point exists
	if _, err := cfg.GetForkPoint("feature"); err != nil {
		t.Fatal("fork point should exist before unlink")
	}

	// Unlink
	runCmd(t, repo, "unlink", "feature")

	// Verify fork point removed
	if _, err := cfg.GetForkPoint("feature"); err == nil {
		t.Error("fork point should be removed after unlink")
	}
}
```

**Step 4: Run tests**

Run: `go test -v -run TestUnlinkRemovesForkPoint ./cmd`
Expected: PASS

**Step 5: Commit**

```bash
git add cmd/unlink.go cmd/orphan.go cmd/unlink_test.go
git commit -m "chore: clean up fork point on unlink/orphan"
```

---

### Task 10: Update Sync to Use Stored Fork Points

**Files:**

- Modify: `cmd/sync.go`

**Step 1: Update sync to prefer stored fork point**

In the retargeting section, prefer stored fork point over calculated:

```go
		// Get fork point - prefer stored, fall back to calculated
		forkPoint, fpErr := cfg.GetForkPoint(child.Name)
		if fpErr != nil || !g.CommitExists(forkPoint) {
			// Fall back to calculating from parent (before it's deleted)
			forkPoint, fpErr = g.GetMergeBase(child.Name, branch)
			if fpErr != nil {
				fmt.Printf("Warning: could not get fork point for %s: %v\n", child.Name, fpErr)
				forkPoint = ""
			}
		}
```

**Step 2: Run full test suite**

Run: `make test`
Expected: All pass

**Step 3: Commit**

```bash
git add cmd/sync.go
git commit -m "feat(sync): prefer stored fork point over calculated

Uses the stored fork point if available and valid, otherwise
falls back to calculating from the (about to be deleted) parent.
"
```

---

## Verification

After completing all tasks:

1. Run full test suite: `make ci`
2. Manual test with actual squash-merge scenario
3. Verify no regressions in existing functionality

---

## Summary

**Phase 1** (Tasks 1-3): Fixes the immediate sync issue by calculating fork points on-demand before deleting merged branches. Minimal changes, immediate value.

**Phase 2** (Tasks 4-10): Adds persistent fork point tracking for robust handling in all scenarios. More comprehensive but requires touching more code paths.

Both phases are backward compatible - existing stacks without fork points will fall back to current behavior.
