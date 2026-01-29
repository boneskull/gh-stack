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

	// Make it dirty with untracked file
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

func TestGetTip(t *testing.T) {
	dir := setupTestRepo(t)
	g := git.New(dir)

	current, _ := g.CurrentBranch()
	tip, err := g.GetTip(current)
	if err != nil {
		t.Fatalf("GetTip failed: %v", err)
	}

	// Should be a 40-character hex SHA
	if len(tip) != 40 {
		t.Errorf("expected 40-char SHA, got %q (len %d)", tip, len(tip))
	}
}

func TestGetMergeBase(t *testing.T) {
	dir := setupTestRepo(t)
	g := git.New(dir)

	current, _ := g.CurrentBranch()

	// Create a branch, make a commit on each
	g.CreateBranch("feature")
	originalTip, _ := g.GetTip(current)

	// Commit on main
	os.WriteFile(filepath.Join(dir, "main.txt"), []byte("main"), 0644)
	exec.Command("git", "-C", dir, "add", ".").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "main commit").Run()

	// Commit on feature
	g.Checkout("feature")
	os.WriteFile(filepath.Join(dir, "feature.txt"), []byte("feature"), 0644)
	exec.Command("git", "-C", dir, "add", ".").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "feature commit").Run()

	// Merge base should be the original tip
	base, err := g.GetMergeBase(current, "feature")
	if err != nil {
		t.Fatalf("GetMergeBase failed: %v", err)
	}
	if base != originalTip {
		t.Errorf("expected merge base %q, got %q", originalTip, base)
	}
}

func TestDeleteBranch(t *testing.T) {
	dir := setupTestRepo(t)
	g := git.New(dir)

	g.CreateBranch("to-delete")
	if !g.BranchExists("to-delete") {
		t.Fatal("branch should exist before deletion")
	}

	err := g.DeleteBranch("to-delete")
	if err != nil {
		t.Fatalf("DeleteBranch failed: %v", err)
	}

	if g.BranchExists("to-delete") {
		t.Error("branch should not exist after deletion")
	}
}

func TestGetGitDir(t *testing.T) {
	dir := setupTestRepo(t)
	g := git.New(dir)

	gitDir := g.GetGitDir()
	expected := filepath.Join(dir, ".git")
	if gitDir != expected {
		t.Errorf("expected %q, got %q", expected, gitDir)
	}
}

func TestNeedsRebase(t *testing.T) {
	dir := setupTestRepo(t)
	g := git.New(dir)

	current, _ := g.CurrentBranch()

	// Create feature branch
	g.CreateBranch("feature")

	// Initially, feature doesn't need rebase (same commit)
	needs, err := g.NeedsRebase("feature", current)
	if err != nil {
		t.Fatalf("NeedsRebase failed: %v", err)
	}
	if needs {
		t.Error("feature should not need rebase initially")
	}

	// Add commit to main - now feature needs rebase
	os.WriteFile(filepath.Join(dir, "new.txt"), []byte("new"), 0644)
	exec.Command("git", "-C", dir, "add", ".").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "new commit").Run()

	needs, err = g.NeedsRebase("feature", current)
	if err != nil {
		t.Fatalf("NeedsRebase failed: %v", err)
	}
	if !needs {
		t.Error("feature should need rebase after main moved forward")
	}
}

