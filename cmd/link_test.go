// cmd/link_test.go
package cmd_test

import (
	"testing"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/git"
)

func TestLinkPR(t *testing.T) {
	dir := setupTestRepo(t)

	g := git.New(dir)
	cfg, _ := config.New(g)

	trunk, _ := g.CurrentBranch()
	cfg.SetTrunk(trunk)

	// Create and track a branch
	g.CreateAndCheckout("feature-a")
	cfg.SetParent("feature-a", trunk)

	// Link a PR
	err := cfg.SetPR("feature-a", 456)
	if err != nil {
		t.Fatalf("SetPR failed: %v", err)
	}

	// Verify
	pr, err := cfg.GetPR("feature-a")
	if err != nil {
		t.Fatalf("GetPR failed: %v", err)
	}
	if pr != 456 {
		t.Errorf("expected PR 456, got %d", pr)
	}
}

func TestLinkRejectsUntrackedBranch(t *testing.T) {
	dir := setupTestRepo(t)

	g := git.New(dir)
	cfg, _ := config.New(g)

	trunk, _ := g.CurrentBranch()
	cfg.SetTrunk(trunk)

	// Create untracked branch
	g.CreateAndCheckout("untracked")

	// Should not be tracked
	_, err := cfg.GetParent("untracked")
	if err == nil {
		t.Error("branch should not be tracked")
	}
}

func TestLinkOverwritesPR(t *testing.T) {
	dir := setupTestRepo(t)

	g := git.New(dir)
	cfg, _ := config.New(g)

	trunk, _ := g.CurrentBranch()
	cfg.SetTrunk(trunk)

	// Create and track a branch with PR
	g.CreateBranch("feature-a")
	cfg.SetParent("feature-a", trunk)
	cfg.SetPR("feature-a", 100)

	// Overwrite with new PR
	cfg.SetPR("feature-a", 200)

	// Verify new PR
	pr, _ := cfg.GetPR("feature-a")
	if pr != 200 {
		t.Errorf("expected PR 200, got %d", pr)
	}
}
