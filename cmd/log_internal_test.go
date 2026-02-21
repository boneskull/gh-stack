// cmd/log_internal_test.go
//
// This file uses package cmd (not cmd_test) to unit-test the unexported
// injectDetectedNodes function directly.
package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/git"
	"github.com/boneskull/gh-stack/internal/tree"
)

// setupInternalTestRepo creates a temp git repo with an initial commit.
// It mirrors setupTestRepo from init_test.go but lives in the internal
// test package so we can call unexported functions.
func setupInternalTestRepo(t *testing.T) string {
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

	f := filepath.Join(dir, "README.md")
	os.WriteFile(f, []byte("# Test"), 0644)
	run("add", ".")
	run("commit", "-m", "initial")

	return dir
}

// addCommit creates a file and commits it in the given repo directory.
func addCommit(t *testing.T, dir, filename, content string) {
	t.Helper()
	os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644)
	cmd := exec.Command("git", "-C", dir, "add", ".")
	cmd.Run()
	cmd = exec.Command("git", "-C", dir, "commit", "-m", "add "+filename)
	cmd.Run()
}

func TestLogDetectsUntrackedBranches(t *testing.T) {
	dir := setupInternalTestRepo(t)
	g := git.New(dir)
	cfg, _ := config.Load(dir)
	trunk, _ := g.CurrentBranch()
	cfg.SetTrunk(trunk)

	// Create tracked branch A off main
	g.CreateAndCheckout("feature-a")
	addCommit(t, dir, "a.txt", "a")
	cfg.SetParent("feature-a", trunk)

	// Create untracked branch B off A
	g.CreateAndCheckout("feature-b")
	addCommit(t, dir, "b.txt", "b")

	// Build tree WITHOUT detection -- B should not appear
	root, err := tree.Build(cfg)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if tree.FindNode(root, "feature-b") != nil {
		t.Error("feature-b should NOT be in tracked tree")
	}

	// Now run detection and inject into tree
	injectDetectedNodes(root, cfg, g)

	// Now B should appear as detected child of A
	nodeB := tree.FindNode(root, "feature-b")
	if nodeB == nil {
		t.Fatal("feature-b should appear after detection")
	}
	if !nodeB.Detected {
		t.Error("feature-b should be marked as Detected")
	}
	if nodeB.Parent.Name != "feature-a" {
		t.Errorf("expected parent 'feature-a', got %q", nodeB.Parent.Name)
	}
}
