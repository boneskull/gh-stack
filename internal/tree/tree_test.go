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
