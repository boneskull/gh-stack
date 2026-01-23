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
