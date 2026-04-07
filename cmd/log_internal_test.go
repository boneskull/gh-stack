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
		if err := cmd.Run(); err != nil {
			t.Fatalf("git %v failed: %v", args, err)
		}
	}

	run("init")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "Test")
	run("config", "commit.gpgsign", "false")

	f := filepath.Join(dir, "README.md")
	if err := os.WriteFile(f, []byte("# Test"), 0644); err != nil {
		t.Fatalf("WriteFile README.md: %v", err)
	}
	run("add", ".")
	run("commit", "-m", "initial")

	return dir
}

// addCommit creates a file and commits it in the given repo directory.
func addCommit(t *testing.T, dir, filename, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile %s: %v", filename, err)
	}
	if err := exec.Command("git", "-C", dir, "add", ".").Run(); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if err := exec.Command("git", "-C", dir, "commit", "-m", "add "+filename).Run(); err != nil {
		t.Fatalf("git commit: %v", err)
	}
}

func TestLogDetectsUntrackedBranches(t *testing.T) {
	dir := setupInternalTestRepo(t)
	g := git.New(dir)

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("Load config: %v", err)
	}

	trunk, err := g.CurrentBranch()
	if err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}

	err = cfg.SetTrunk(trunk)
	if err != nil {
		t.Fatalf("SetTrunk: %v", err)
	}

	// Create tracked branch A off main
	err = g.CreateAndCheckout("feature-a")
	if err != nil {
		t.Fatalf("CreateAndCheckout feature-a: %v", err)
	}
	addCommit(t, dir, "a.txt", "a")

	err = cfg.SetParent("feature-a", trunk)
	if err != nil {
		t.Fatalf("SetParent feature-a: %v", err)
	}

	// Create untracked branch B off A
	err = g.CreateAndCheckout("feature-b")
	if err != nil {
		t.Fatalf("CreateAndCheckout feature-b: %v", err)
	}
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
