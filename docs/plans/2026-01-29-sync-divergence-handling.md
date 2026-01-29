# Sync Divergence Handling Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Handle the case where local trunk has diverged from origin/trunk during sync, offering to rebase local commits onto origin with user confirmation.

**Architecture:** Add a method to detect branch divergence (local ahead of remote). When sync detects divergence during fast-forward, prompt the user to either rebase local commits onto origin or abort. This maintains gh-stack's pattern of being helpful but not destructive without confirmation.

**Tech Stack:** Go, git CLI

---

## Background

When running `gh stack sync`, the command fetches from origin and attempts to fast-forward the trunk branch. If the local trunk has commits that origin doesn't have (local is ahead), the fast-forward fails:

```
Warning: could not fast-forward main: git merge: hint: Diverging branches can't be fast-forwarded
```

The sync continues but leaves the user needing to manually run `git pull`. We should detect this case and offer to resolve it.

---

## Task 1: Add GetRemoteTip Method to Git Package

**Files:**
- Modify: `internal/git/git.go`
- Test: `internal/git/git_test.go`

**Step 1: Write the failing test**

Add to `internal/git/git_test.go`:

```go
func TestGetRemoteTip(t *testing.T) {
	// This test requires a remote, so we'll create a bare repo
	dir := t.TempDir()
	remoteDir := t.TempDir()

	// Create bare remote
	exec.Command("git", "init", "--bare", remoteDir).Run()

	// Initialize local repo
	exec.Command("git", "-C", dir, "init").Run()
	exec.Command("git", "-C", dir, "config", "user.email", "test@test.com").Run()
	exec.Command("git", "-C", dir, "config", "user.name", "Test").Run()
	exec.Command("git", "-C", dir, "remote", "add", "origin", remoteDir).Run()

	// Create initial commit and push
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("content"), 0644)
	exec.Command("git", "-C", dir, "add", ".").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "initial").Run()
	exec.Command("git", "-C", dir, "push", "-u", "origin", "main").Run()

	g := git.New(dir)

	// Get remote tip
	remoteTip, err := g.GetRemoteTip("origin", "main")
	if err != nil {
		t.Fatalf("GetRemoteTip failed: %v", err)
	}

	// Should match local tip since we just pushed
	localTip, _ := g.GetTip("main")
	if remoteTip != localTip {
		t.Errorf("remote tip %s != local tip %s", remoteTip, localTip)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run TestGetRemoteTip ./internal/git`
Expected: FAIL with "g.GetRemoteTip undefined"

**Step 3: Write minimal implementation**

Add to `internal/git/git.go` after `GetTip`:

```go
// GetRemoteTip returns the commit SHA at the tip of a remote branch.
func (g *Git) GetRemoteTip(remote, branch string) (string, error) {
	return g.run("rev-parse", remote+"/"+branch)
}
```

**Step 4: Run test to verify it passes**

Run: `go test -v -run TestGetRemoteTip ./internal/git`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/git/git.go internal/git/git_test.go
git commit -m "feat(git): add GetRemoteTip method"
```

---

## Task 2: Add IsDiverged Method to Git Package

**Files:**
- Modify: `internal/git/git.go`
- Test: `internal/git/git_test.go`

**Step 1: Write the failing test**

Add to `internal/git/git_test.go`:

```go
func TestIsDiverged(t *testing.T) {
	dir := t.TempDir()
	remoteDir := t.TempDir()

	// Create bare remote
	exec.Command("git", "init", "--bare", remoteDir).Run()

	// Initialize local repo
	exec.Command("git", "-C", dir, "init").Run()
	exec.Command("git", "-C", dir, "config", "user.email", "test@test.com").Run()
	exec.Command("git", "-C", dir, "config", "user.name", "Test").Run()
	exec.Command("git", "-C", dir, "remote", "add", "origin", remoteDir).Run()

	// Create initial commit and push
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("content"), 0644)
	exec.Command("git", "-C", dir, "add", ".").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "initial").Run()
	exec.Command("git", "-C", dir, "push", "-u", "origin", "main").Run()

	g := git.New(dir)

	// Initially not diverged
	diverged, localAhead, err := g.IsDiverged("main", "origin")
	if err != nil {
		t.Fatalf("IsDiverged failed: %v", err)
	}
	if diverged {
		t.Error("should not be diverged initially")
	}
	if localAhead {
		t.Error("should not be ahead initially")
	}

	// Add local commit (makes local ahead, but not diverged since remote hasn't moved)
	os.WriteFile(filepath.Join(dir, "local.txt"), []byte("local"), 0644)
	exec.Command("git", "-C", dir, "add", ".").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "local commit").Run()

	diverged, localAhead, err = g.IsDiverged("main", "origin")
	if err != nil {
		t.Fatalf("IsDiverged failed: %v", err)
	}
	if diverged {
		t.Error("should not be diverged (remote hasn't moved)")
	}
	if !localAhead {
		t.Error("should be ahead after local commit")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run TestIsDiverged ./internal/git`
Expected: FAIL with "g.IsDiverged undefined"

**Step 3: Write minimal implementation**

Add to `internal/git/git.go`:

```go
// IsDiverged checks if a local branch has diverged from its remote.
// Returns (diverged, localAhead, error).
// diverged = true when both local and remote have unique commits
// localAhead = true when local has commits not on remote
func (g *Git) IsDiverged(branch, remote string) (bool, bool, error) {
	localTip, err := g.GetTip(branch)
	if err != nil {
		return false, false, err
	}

	remoteTip, err := g.GetRemoteTip(remote, branch)
	if err != nil {
		return false, false, err
	}

	if localTip == remoteTip {
		return false, false, nil
	}

	mergeBase, err := g.GetMergeBase(branch, remote+"/"+branch)
	if err != nil {
		return false, false, err
	}

	localAhead := mergeBase != localTip
	remoteAhead := mergeBase != remoteTip
	diverged := localAhead && remoteAhead

	return diverged, localAhead, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test -v -run TestIsDiverged ./internal/git`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/git/git.go internal/git/git_test.go
git commit -m "feat(git): add IsDiverged method for branch comparison"
```

---

## Task 3: Add PullRebase Method to Git Package

**Files:**
- Modify: `internal/git/git.go`
- Test: `internal/git/git_test.go`

**Step 1: Write the failing test**

Add to `internal/git/git_test.go`:

```go
func TestPullRebase(t *testing.T) {
	dir := t.TempDir()
	remoteDir := t.TempDir()

	// Create bare remote
	exec.Command("git", "init", "--bare", remoteDir).Run()

	// Initialize local repo
	exec.Command("git", "-C", dir, "init").Run()
	exec.Command("git", "-C", dir, "config", "user.email", "test@test.com").Run()
	exec.Command("git", "-C", dir, "config", "user.name", "Test").Run()
	exec.Command("git", "-C", dir, "remote", "add", "origin", remoteDir).Run()

	// Create initial commit and push
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("content"), 0644)
	exec.Command("git", "-C", dir, "add", ".").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "initial").Run()
	exec.Command("git", "-C", dir, "push", "-u", "origin", "main").Run()

	// Clone to a second repo and push a commit (simulates remote moving forward)
	cloneDir := t.TempDir()
	exec.Command("git", "clone", remoteDir, cloneDir).Run()
	exec.Command("git", "-C", cloneDir, "config", "user.email", "test@test.com").Run()
	exec.Command("git", "-C", cloneDir, "config", "user.name", "Test").Run()
	os.WriteFile(filepath.Join(cloneDir, "remote.txt"), []byte("remote"), 0644)
	exec.Command("git", "-C", cloneDir, "add", ".").Run()
	exec.Command("git", "-C", cloneDir, "commit", "-m", "remote commit").Run()
	exec.Command("git", "-C", cloneDir, "push").Run()

	// Add local commit (now local and remote have diverged)
	os.WriteFile(filepath.Join(dir, "local.txt"), []byte("local"), 0644)
	exec.Command("git", "-C", dir, "add", ".").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "local commit").Run()

	// Fetch to update remote refs
	exec.Command("git", "-C", dir, "fetch", "origin").Run()

	g := git.New(dir)

	// Pull with rebase should succeed
	err := g.PullRebase("main")
	if err != nil {
		t.Fatalf("PullRebase failed: %v", err)
	}

	// Both files should exist
	if _, err := os.Stat(filepath.Join(dir, "local.txt")); err != nil {
		t.Error("local.txt should exist after rebase")
	}
	if _, err := os.Stat(filepath.Join(dir, "remote.txt")); err != nil {
		t.Error("remote.txt should exist after rebase")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run TestPullRebase ./internal/git`
Expected: FAIL with "g.PullRebase undefined"

**Step 3: Write minimal implementation**

Add to `internal/git/git.go`:

```go
// PullRebase rebases the current branch onto its remote tracking branch.
func (g *Git) PullRebase(branch string) error {
	if err := g.Checkout(branch); err != nil {
		return err
	}
	return g.runInteractive("pull", "--rebase", "origin", branch)
}
```

**Step 4: Run test to verify it passes**

Run: `go test -v -run TestPullRebase ./internal/git`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/git/git.go internal/git/git_test.go
git commit -m "feat(git): add PullRebase method"
```

---

## Task 4: Update Sync to Handle Divergence

**Files:**
- Modify: `cmd/sync.go`

**Step 1: Review current fast-forward section**

Current code (lines 109-118):
```go
// Fast-forward trunk
currentBranch, _ := g.CurrentBranch()
fmt.Printf("Fast-forwarding %s...\n", trunk)
if !syncDryRunFlag {
    if ffErr := g.FastForward(trunk); ffErr != nil {
        fmt.Printf("Warning: could not fast-forward %s: %v\n", trunk, ffErr)
    }
    // Return to original branch
    _ = g.Checkout(currentBranch)
}
```

**Step 2: Replace with divergence-aware logic**

Replace the fast-forward section with:

```go
	// Update trunk from remote
	currentBranch, _ := g.CurrentBranch() //nolint:errcheck // empty string is fine
	fmt.Printf("Updating %s from origin...\n", trunk)
	if !syncDryRunFlag {
		// Check if local trunk has diverged from origin
		diverged, localAhead, divErr := g.IsDiverged(trunk, "origin")
		if divErr != nil {
			fmt.Printf("Warning: could not check divergence for %s: %v\n", trunk, divErr)
		}

		if diverged {
			// Both local and remote have unique commits - need user decision
			fmt.Printf("\n%s has diverged from origin/%s.\n", trunk, trunk)
			fmt.Printf("Local %s has commits not on origin, and origin has commits not on local.\n", trunk)
			fmt.Print("Rebase local commits onto origin? [y/N]: ")

			var response string
			if _, scanErr := fmt.Scanln(&response); scanErr == nil {
				if strings.ToLower(strings.TrimSpace(response)) == "y" {
					if rebaseErr := g.PullRebase(trunk); rebaseErr != nil {
						return fmt.Errorf("failed to rebase %s: %w", trunk, rebaseErr)
					}
					fmt.Printf("Rebased %s onto origin/%s\n", trunk, trunk)
				} else {
					fmt.Printf("Skipping %s update. Run 'git pull' manually to resolve.\n", trunk)
				}
			}
		} else if localAhead {
			// Local is ahead but remote hasn't moved - can fast-forward after push
			fmt.Printf("%s is ahead of origin/%s by local commits.\n", trunk, trunk)
			fmt.Printf("Consider pushing your local %s commits.\n", trunk)
			// Still try fast-forward in case remote is also ahead
			if ffErr := g.FastForward(trunk); ffErr != nil {
				// Expected to fail if truly ahead, that's fine
			}
		} else {
			// Normal case: fast-forward
			if ffErr := g.FastForward(trunk); ffErr != nil {
				fmt.Printf("Warning: could not fast-forward %s: %v\n", trunk, ffErr)
			}
		}

		// Return to original branch
		_ = g.Checkout(currentBranch) //nolint:errcheck // best effort
	}
```

**Step 3: Run tests**

Run: `make test`
Expected: All tests pass

**Step 4: Run lint**

Run: `golangci-lint run ./...`
Expected: No issues

**Step 5: Commit**

```bash
git add cmd/sync.go
git commit -m "feat(sync): handle trunk divergence with user confirmation

When local trunk has diverged from origin (both have unique commits),
sync now prompts the user to rebase local commits onto origin rather
than silently failing the fast-forward.
"
```

---

## Verification

After completing all tasks:

1. Run full test suite: `make ci`
2. Manual test scenarios:
   - Sync when local and remote are in sync (should fast-forward)
   - Sync when remote is ahead (should fast-forward)
   - Sync when local is ahead (should warn about pushing)
   - Sync when diverged (should prompt for rebase)
3. Verify no regressions in existing functionality

---

## Summary

| Task | Component | Description |
|------|-----------|-------------|
| 1 | git.GetRemoteTip | Get SHA of remote branch tip |
| 2 | git.IsDiverged | Detect divergence between local and remote |
| 3 | git.PullRebase | Rebase local onto remote tracking branch |
| 4 | sync.go | Handle divergence with user confirmation |

All changes are backward compatible - the new behavior only activates when divergence is detected.
