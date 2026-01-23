// cmd/orphan_test.go
package cmd_test

import (
	"testing"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/git"
	"github.com/boneskull/gh-stack/internal/tree"
)

func TestOrphanBranch(t *testing.T) {
	dir := setupTestRepo(t)

	cfg, _ := config.Load(dir)
	g := git.New(dir)

	trunk, _ := g.CurrentBranch()
	cfg.SetTrunk(trunk)

	// Create and track a branch
	g.CreateBranch("feature-a")
	cfg.SetParent("feature-a", trunk)

	// Verify it's tracked
	_, err := cfg.GetParent("feature-a")
	if err != nil {
		t.Fatal("branch should be tracked")
	}

	// Orphan it
	cfg.RemoveParent("feature-a")

	// Verify it's no longer tracked
	_, err = cfg.GetParent("feature-a")
	if err == nil {
		t.Error("branch should no longer be tracked after orphan")
	}
}

func TestOrphanRejectsWithChildren(t *testing.T) {
	dir := setupTestRepo(t)

	cfg, _ := config.Load(dir)
	g := git.New(dir)

	trunk, _ := g.CurrentBranch()
	cfg.SetTrunk(trunk)

	// Create chain: trunk -> feature-a -> feature-b
	g.CreateBranch("feature-a")
	cfg.SetParent("feature-a", trunk)
	g.CreateBranch("feature-b")
	cfg.SetParent("feature-b", "feature-a")

	// Build tree
	root, _ := tree.Build(cfg)
	node := tree.FindNode(root, "feature-a")

	// feature-a has children, so orphan should require --force
	if len(node.Children) == 0 {
		t.Error("expected feature-a to have children")
	}
}

func TestOrphanForceWithDescendants(t *testing.T) {
	dir := setupTestRepo(t)

	cfg, _ := config.Load(dir)
	g := git.New(dir)

	trunk, _ := g.CurrentBranch()
	cfg.SetTrunk(trunk)

	// Create chain: trunk -> feature-a -> feature-b -> feature-c
	g.CreateBranch("feature-a")
	cfg.SetParent("feature-a", trunk)
	g.CreateBranch("feature-b")
	cfg.SetParent("feature-b", "feature-a")
	g.CreateBranch("feature-c")
	cfg.SetParent("feature-c", "feature-b")

	// Build tree and get descendants
	root, _ := tree.Build(cfg)
	node := tree.FindNode(root, "feature-a")
	descendants := tree.GetDescendants(node)

	// Should have 2 descendants
	if len(descendants) != 2 {
		t.Errorf("expected 2 descendants, got %d", len(descendants))
	}

	// Simulate --force orphan
	for _, desc := range descendants {
		cfg.RemoveParent(desc.Name)
	}
	cfg.RemoveParent("feature-a")

	// Verify all are orphaned
	_, err := cfg.GetParent("feature-a")
	if err == nil {
		t.Error("feature-a should be orphaned")
	}
	_, err = cfg.GetParent("feature-b")
	if err == nil {
		t.Error("feature-b should be orphaned")
	}
	_, err = cfg.GetParent("feature-c")
	if err == nil {
		t.Error("feature-c should be orphaned")
	}
}

func TestOrphanRemovesPR(t *testing.T) {
	dir := setupTestRepo(t)

	cfg, _ := config.Load(dir)
	g := git.New(dir)

	trunk, _ := g.CurrentBranch()
	cfg.SetTrunk(trunk)

	// Create branch with PR
	g.CreateBranch("feature-a")
	cfg.SetParent("feature-a", trunk)
	cfg.SetPR("feature-a", 123)

	// Verify PR is set
	pr, err := cfg.GetPR("feature-a")
	if err != nil || pr != 123 {
		t.Fatal("PR should be set")
	}

	// Orphan (which should also remove PR)
	cfg.RemoveParent("feature-a")
	cfg.RemovePR("feature-a")

	// Verify PR is gone
	_, err = cfg.GetPR("feature-a")
	if err == nil {
		t.Error("PR should be removed after orphan")
	}
}
