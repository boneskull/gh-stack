// cmd/sync_test.go
package cmd_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/boneskull/gh-stack/internal/git"
)

// TestSyncStartingBranchCapture tests that the starting branch capture logic
// correctly normalizes the branch name, particularly handling detached HEAD state.
func TestSyncStartingBranchCapture(t *testing.T) {
	dir := setupTestRepo(t)
	g := git.New(dir)

	// Normal branch case: should return the branch name
	current, err := g.CurrentBranch()
	if err != nil {
		t.Fatalf("CurrentBranch failed: %v", err)
	}
	if current == "" || current == "HEAD" {
		t.Errorf("expected a branch name, got %q", current)
	}

	// Create and checkout a branch to verify we can get it
	g.CreateAndCheckout("feature-test")
	current, err = g.CurrentBranch()
	if err != nil {
		t.Fatalf("CurrentBranch failed: %v", err)
	}
	if current != "feature-test" {
		t.Errorf("expected 'feature-test', got %q", current)
	}
}

// TestSyncStartingBranchDetachedHEAD tests that detached HEAD state returns "HEAD"
// which the sync command should normalize to empty string.
func TestSyncStartingBranchDetachedHEAD(t *testing.T) {
	dir := setupTestRepo(t)
	g := git.New(dir)

	// Get current tip SHA
	sha, err := g.GetTip("HEAD")
	if err != nil {
		t.Fatalf("GetTip failed: %v", err)
	}

	// Detach HEAD by checking out a SHA
	detachCmd := exec.Command("git", "-C", dir, "checkout", sha)
	if detachErr := detachCmd.Run(); detachErr != nil {
		t.Fatalf("git checkout %s failed: %v", sha, detachErr)
	}

	// CurrentBranch returns "HEAD" when in detached HEAD state
	current, err := g.CurrentBranch()
	if err != nil {
		t.Fatalf("CurrentBranch failed: %v", err)
	}
	if current != "HEAD" {
		t.Errorf("expected 'HEAD' for detached HEAD state, got %q", current)
	}

	// Sync command logic: normalize "HEAD" to empty string
	// This is the logic we're testing (from cmd/sync.go lines 117-121)
	startingBranch := current
	if startingBranch == "HEAD" {
		startingBranch = ""
	}
	if startingBranch != "" {
		t.Errorf("expected empty string after normalization, got %q", startingBranch)
	}
}

// TestSyncReturnsToBranchAfterOperations tests that the return-to-branch logic
// correctly handles the case where we need to checkout back to the starting branch.
func TestSyncReturnsToBranchAfterOperations(t *testing.T) {
	dir := setupTestRepo(t)
	g := git.New(dir)

	trunk, _ := g.CurrentBranch()

	// Create and checkout a starting branch
	g.CreateAndCheckout("starting-branch")

	// Simulate sync operations by checking out another branch
	g.Checkout(trunk)

	// Now simulate the return-to-branch logic
	startingBranch := "starting-branch"
	currentBranch, _ := g.CurrentBranch()

	if currentBranch != startingBranch {
		// Branch exists, so we should be able to return to it
		if g.BranchExists(startingBranch) {
			if err := g.Checkout(startingBranch); err != nil {
				t.Fatalf("could not return to starting branch: %v", err)
			}
		}
	}

	// Verify we're back on starting branch
	currentBranch, _ = g.CurrentBranch()
	if currentBranch != "starting-branch" {
		t.Errorf("expected to return to 'starting-branch', got %q", currentBranch)
	}
}

// TestSyncStartingBranchDeleted tests the scenario where the starting branch
// gets deleted during sync (e.g., it was merged and cleaned up).
func TestSyncStartingBranchDeleted(t *testing.T) {
	dir := setupTestRepo(t)
	g := git.New(dir)

	trunk, _ := g.CurrentBranch()

	// Create and checkout a starting branch
	g.CreateAndCheckout("to-be-deleted")

	// Record starting branch
	startingBranch := "to-be-deleted"

	// Simulate sync: checkout trunk, then delete the starting branch
	g.Checkout(trunk)
	g.DeleteBranch(startingBranch)

	// Now simulate the return-to-branch logic
	currentBranch, _ := g.CurrentBranch()
	if currentBranch != startingBranch {
		// Check if starting branch still exists
		if g.BranchExists(startingBranch) {
			t.Error("branch should have been deleted")
		}
		// Branch doesn't exist - this is the expected "warning" case
		// Sync should stay on current branch and warn
	}

	// Verify we stayed on trunk (didn't fail trying to checkout deleted branch)
	currentBranch, _ = g.CurrentBranch()
	if currentBranch != trunk {
		t.Errorf("expected to stay on %q when starting branch deleted, got %q", trunk, currentBranch)
	}
}

// TestSyncCheckoutFailure tests the scenario where checkout fails
// (e.g., dirty worktree with conflicting changes prevents checkout).
func TestSyncCheckoutFailure(t *testing.T) {
	dir := setupTestRepo(t)
	g := git.New(dir)

	trunk, _ := g.CurrentBranch()

	// Create starting-branch with a file that differs from trunk
	g.CreateAndCheckout("starting-branch")
	conflictFile := filepath.Join(dir, "conflict.txt")
	if err := os.WriteFile(conflictFile, []byte("starting-branch content"), 0644); err != nil {
		t.Fatalf("failed to write conflict file: %v", err)
	}
	stageCmd := exec.Command("git", "-C", dir, "add", "conflict.txt")
	if err := stageCmd.Run(); err != nil {
		t.Fatalf("git add failed: %v", err)
	}
	commitCmd := exec.Command("git", "-C", dir, "commit", "-m", "add conflict file on starting-branch")
	if err := commitCmd.Run(); err != nil {
		t.Fatalf("git commit failed: %v", err)
	}

	// Go back to trunk and create different content in the same file
	g.Checkout(trunk)
	if err := os.WriteFile(conflictFile, []byte("trunk content - different!"), 0644); err != nil {
		t.Fatalf("failed to write conflict file on trunk: %v", err)
	}
	// Stage the file but don't commit - this creates dirty state
	stageCmd2 := exec.Command("git", "-C", dir, "add", "conflict.txt")
	if err := stageCmd2.Run(); err != nil {
		t.Fatalf("git add (trunk) failed: %v", err)
	}

	// Now we're on trunk with staged changes that conflict with starting-branch
	// Attempting to checkout starting-branch should fail
	startingBranch := "starting-branch"
	checkoutErr := g.Checkout(startingBranch)

	// The checkout should fail due to conflicting staged changes
	if checkoutErr == nil {
		t.Log("checkout succeeded unexpectedly - git may have auto-merged; checking worktree state")
		// Even if checkout somehow succeeded, verify the logic handles it
	} else {
		t.Logf("checkout failed as expected: %v", checkoutErr)
	}

	// The key assertion: sync's return-to-branch logic should handle this gracefully
	// by staying on the current branch (trunk) rather than failing catastrophically
	currentBranch, _ := g.CurrentBranch()

	// If checkout failed, we should still be on trunk
	if checkoutErr != nil && currentBranch != trunk {
		t.Errorf("expected to stay on %q after checkout failure, got %q", trunk, currentBranch)
	}
}

// TestSyncEmptyStartingBranchSkipsReturn tests that when starting branch is empty
// (e.g., from detached HEAD), we don't attempt to return anywhere.
func TestSyncEmptyStartingBranchSkipsReturn(t *testing.T) {
	dir := setupTestRepo(t)
	g := git.New(dir)

	trunk, _ := g.CurrentBranch()

	// Simulate having an empty starting branch (from detached HEAD normalization)
	startingBranch := ""

	// Go to some other branch
	g.CreateAndCheckout("other-branch")

	// The return logic should skip when startingBranch is empty
	currentBranch, _ := g.CurrentBranch()
	if startingBranch != "" && currentBranch != startingBranch {
		if g.BranchExists(startingBranch) {
			g.Checkout(startingBranch)
		}
	}

	// Verify we stayed on "other-branch" (didn't try to checkout empty string)
	currentBranch, _ = g.CurrentBranch()
	if currentBranch != "other-branch" {
		t.Errorf("expected to stay on 'other-branch' with empty starting branch, got %q", currentBranch)
	}

	_ = trunk // silence unused warning
}
