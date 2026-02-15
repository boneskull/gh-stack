// cmd/create_test.go
package cmd_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/git"
)

func TestCreateFromTrunk(t *testing.T) {
	dir := setupTestRepo(t)

	g := git.New(dir)
	cfg, _ := config.New(g)
	cfg.SetTrunk("main")

	// Simulate what create command does
	currentBranch, _ := g.CurrentBranch()
	trunk, _ := cfg.GetTrunk()

	if currentBranch != trunk && currentBranch != "master" {
		t.Skipf("not on trunk branch, got %q", currentBranch)
	}

	// Create branch
	err := g.CreateAndCheckout("feature-a")
	if err != nil {
		t.Fatalf("CreateAndCheckout failed: %v", err)
	}

	// Set parent
	err = cfg.SetParent("feature-a", currentBranch)
	if err != nil {
		t.Fatalf("SetParent failed: %v", err)
	}

	// Verify
	parent, err := cfg.GetParent("feature-a")
	if err != nil {
		t.Fatalf("GetParent failed: %v", err)
	}
	if parent != currentBranch {
		t.Errorf("expected parent %q, got %q", currentBranch, parent)
	}

	newBranch, _ := g.CurrentBranch()
	if newBranch != "feature-a" {
		t.Errorf("expected to be on feature-a, got %q", newBranch)
	}
}

func TestCreateFromTrackedBranch(t *testing.T) {
	dir := setupTestRepo(t)

	g := git.New(dir)
	cfg, _ := config.New(g)

	currentBranch, _ := g.CurrentBranch()
	cfg.SetTrunk(currentBranch)

	// Create first feature branch
	g.CreateAndCheckout("feature-a")
	cfg.SetParent("feature-a", currentBranch)

	// Create second feature branch stacked on first
	g.CreateAndCheckout("feature-b")
	cfg.SetParent("feature-b", "feature-a")

	// Verify chain
	parentB, _ := cfg.GetParent("feature-b")
	if parentB != "feature-a" {
		t.Errorf("expected parent 'feature-a', got %q", parentB)
	}

	parentA, _ := cfg.GetParent("feature-a")
	if parentA != currentBranch {
		t.Errorf("expected parent %q, got %q", currentBranch, parentA)
	}
}

func TestCreateRejectsUntrackedBranch(t *testing.T) {
	dir := setupTestRepo(t)

	g := git.New(dir)
	cfg, _ := config.New(g)

	trunk, _ := g.CurrentBranch()
	cfg.SetTrunk(trunk)

	// Create an untracked branch manually (not via stack)
	g.CreateAndCheckout("untracked-branch")

	// Now try to verify it's not tracked
	_, err := cfg.GetParent("untracked-branch")
	if err == nil {
		t.Error("expected error for untracked branch")
	}

	// The create command would reject creating from here
	// since untracked-branch is not trunk and not tracked
	currentBranch, _ := g.CurrentBranch()
	trunkName, _ := cfg.GetTrunk()

	if currentBranch == trunkName {
		t.Skip("unexpectedly on trunk")
	}

	_, err = cfg.GetParent(currentBranch)
	if err == nil {
		t.Error("expected untracked branch to have no parent")
	}
}

func TestCreateWithStagedChanges(t *testing.T) {
	dir := setupTestRepo(t)

	g := git.New(dir)
	cfg, _ := config.New(g)

	trunk, _ := g.CurrentBranch()
	cfg.SetTrunk(trunk)

	// Create and stage a file
	f := filepath.Join(dir, "newfile.txt")
	os.WriteFile(f, []byte("content"), 0644)
	exec.Command("git", "-C", dir, "add", f).Run()

	// Verify staged changes
	hasStaged, _ := g.HasStagedChanges()
	if !hasStaged {
		t.Fatal("expected staged changes")
	}

	// Create branch and commit
	g.CreateAndCheckout("feature-with-commit")
	cfg.SetParent("feature-with-commit", trunk)
	g.Commit("feat: add new file")

	// Verify no more staged changes
	hasStaged, _ = g.HasStagedChanges()
	if hasStaged {
		t.Error("expected no staged changes after commit")
	}
}

func TestCreateEmptyWithStagedChanges(t *testing.T) {
	dir := setupTestRepo(t)

	g := git.New(dir)
	cfg, _ := config.New(g)

	trunk, _ := g.CurrentBranch()
	cfg.SetTrunk(trunk)

	// Create and stage a file
	f := filepath.Join(dir, "newfile.txt")
	os.WriteFile(f, []byte("content"), 0644)
	exec.Command("git", "-C", dir, "add", f).Run()

	// Create branch WITHOUT committing (--empty behavior)
	g.CreateAndCheckout("feature-empty")
	cfg.SetParent("feature-empty", trunk)
	// Don't commit - this simulates --empty flag

	// Staged changes should still exist
	hasStaged, _ := g.HasStagedChanges()
	if !hasStaged {
		t.Error("expected staged changes to remain with --empty")
	}
}

func TestBranchAlreadyExists(t *testing.T) {
	dir := setupTestRepo(t)

	g := git.New(dir)

	// Create a branch
	g.CreateBranch("existing-branch")

	// Verify it exists
	if !g.BranchExists("existing-branch") {
		t.Fatal("branch should exist")
	}

	// The create command would reject this
	// We just verify the check works
	if !g.BranchExists("existing-branch") {
		t.Error("BranchExists should return true")
	}
}

func TestCreateStoresForkPoint(t *testing.T) {
	dir := setupTestRepo(t)

	g := git.New(dir)
	cfg, _ := config.New(g)

	trunk, _ := g.CurrentBranch()
	cfg.SetTrunk(trunk)

	// Get the tip of trunk before creating branch
	trunkTip, _ := g.GetTip(trunk)

	// Simulate create command: create branch and set parent + fork point
	g.CreateAndCheckout("feature")
	cfg.SetParent("feature", trunk)

	// Store fork point (what create command should now do)
	forkPoint, fpErr := g.GetMergeBase("feature", trunk)
	if fpErr != nil {
		t.Fatalf("GetMergeBase failed: %v", fpErr)
	}
	cfg.SetForkPoint("feature", forkPoint)

	// Verify fork point was stored and equals trunk tip
	storedFP, err := cfg.GetForkPoint("feature")
	if err != nil {
		t.Fatalf("GetForkPoint failed: %v", err)
	}
	if storedFP != trunkTip {
		t.Errorf("fork point = %s, want %s (trunk tip at creation)", storedFP, trunkTip)
	}
}
