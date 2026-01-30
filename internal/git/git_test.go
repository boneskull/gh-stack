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

func TestGetCommits(t *testing.T) {
	dir := setupTestRepo(t)
	g := git.New(dir)

	current, _ := g.CurrentBranch()

	// Create feature branch and add commits
	g.CreateAndCheckout("feature")

	// Commit 1: subject only
	os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("content1"), 0644)
	exec.Command("git", "-C", dir, "add", ".").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "feat: first commit").Run()

	// Commit 2: subject and body
	os.WriteFile(filepath.Join(dir, "file2.txt"), []byte("content2"), 0644)
	exec.Command("git", "-C", dir, "add", ".").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "feat: second commit\n\nThis is the body of the commit.\nIt has multiple lines.").Run()

	// Get commits between main and feature
	commits, err := g.GetCommits(current, "feature")
	if err != nil {
		t.Fatalf("GetCommits failed: %v", err)
	}

	if len(commits) != 2 {
		t.Fatalf("expected 2 commits, got %d", len(commits))
	}

	// Commits are in reverse chronological order (newest first)
	if commits[0].Subject != "feat: second commit" {
		t.Errorf("expected first commit subject 'feat: second commit', got %q", commits[0].Subject)
	}
	if commits[0].Body == "" {
		t.Error("expected first commit to have a body")
	}

	if commits[1].Subject != "feat: first commit" {
		t.Errorf("expected second commit subject 'feat: first commit', got %q", commits[1].Subject)
	}
	if commits[1].Body != "" {
		t.Errorf("expected second commit to have no body, got %q", commits[1].Body)
	}
}

func TestGetCommitsNoCommits(t *testing.T) {
	dir := setupTestRepo(t)
	g := git.New(dir)

	current, _ := g.CurrentBranch()

	// Create feature branch at same commit (no new commits)
	g.CreateBranch("feature")

	// Should return empty slice
	commits, err := g.GetCommits(current, "feature")
	if err != nil {
		t.Fatalf("GetCommits failed: %v", err)
	}

	if len(commits) != 0 {
		t.Errorf("expected 0 commits, got %d", len(commits))
	}
}

func TestGetCommitsSingleCommit(t *testing.T) {
	dir := setupTestRepo(t)
	g := git.New(dir)

	current, _ := g.CurrentBranch()

	// Create feature branch with one commit
	g.CreateAndCheckout("feature")
	os.WriteFile(filepath.Join(dir, "single.txt"), []byte("single"), 0644)
	exec.Command("git", "-C", dir, "add", ".").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "fix: single commit\n\nThis fixes the bug.").Run()

	commits, err := g.GetCommits(current, "feature")
	if err != nil {
		t.Fatalf("GetCommits failed: %v", err)
	}

	if len(commits) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(commits))
	}

	if commits[0].Subject != "fix: single commit" {
		t.Errorf("expected subject 'fix: single commit', got %q", commits[0].Subject)
	}
	if commits[0].Body != "This fixes the bug." {
		t.Errorf("expected body 'This fixes the bug.', got %q", commits[0].Body)
	}
}

func TestCommitExists(t *testing.T) {
	dir := setupTestRepo(t)
	g := git.New(dir)

	// Create a commit
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("content"), 0644)
	exec.Command("git", "-C", dir, "add", ".").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "test commit").Run()
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

func TestRebaseOnto(t *testing.T) {
	dir := setupTestRepo(t)
	g := git.New(dir)

	// Determine the trunk branch name (may be "main" or "master" depending on system)
	trunk, err := g.CurrentBranch()
	if err != nil {
		t.Fatalf("CurrentBranch failed: %v", err)
	}

	// Create initial commit on trunk
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("initial"), 0644)
	exec.Command("git", "-C", dir, "add", ".").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "initial").Run()

	// Create parent branch with a commit
	exec.Command("git", "-C", dir, "checkout", "-b", "parent").Run()
	os.WriteFile(filepath.Join(dir, "parent.txt"), []byte("parent content"), 0644)
	exec.Command("git", "-C", dir, "add", ".").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "parent commit").Run()
	parentTip, _ := g.GetTip("parent")

	// Create child branch with a commit
	exec.Command("git", "-C", dir, "checkout", "-b", "child").Run()
	os.WriteFile(filepath.Join(dir, "child.txt"), []byte("child content"), 0644)
	exec.Command("git", "-C", dir, "add", ".").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "child commit").Run()

	// Go back to trunk and add a new commit (simulating trunk moving forward)
	exec.Command("git", "-C", dir, "checkout", trunk).Run()
	os.WriteFile(filepath.Join(dir, "trunk2.txt"), []byte("trunk moved forward"), 0644)
	exec.Command("git", "-C", dir, "add", ".").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "trunk moved forward").Run()

	// Now rebase child onto trunk, using parent tip as the fork point
	// This should only replay "child commit", not "parent commit"
	err = g.RebaseOnto(trunk, parentTip, "child")
	if err != nil {
		t.Fatalf("RebaseOnto failed: %v", err)
	}

	// Verify child is now based on trunk
	exec.Command("git", "-C", dir, "checkout", "child").Run()
	mergeBase, _ := g.GetMergeBase("child", trunk)
	trunkTip, _ := g.GetTip(trunk)
	if mergeBase != trunkTip {
		t.Errorf("child should be based on trunk tip, got merge-base %s, trunk tip %s", mergeBase, trunkTip)
	}

	// Verify child.txt exists (child's commit was replayed)
	if _, err := os.Stat(filepath.Join(dir, "child.txt")); err != nil {
		t.Error("child.txt should exist after rebase")
	}

	// Verify parent.txt does NOT exist (parent's commit was not replayed)
	if _, err := os.Stat(filepath.Join(dir, "parent.txt")); os.IsNotExist(err) {
		// This is expected - parent.txt should not be on child after --onto rebase
	} else {
		t.Error("parent.txt should NOT exist - only child's commits should be replayed")
	}
}
