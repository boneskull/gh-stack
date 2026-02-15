// cmd/unlink_test.go
package cmd_test

import (
	"testing"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/git"
)

func TestUnlinkPR(t *testing.T) {
	dir := setupTestRepo(t)

	g := git.New(dir)
	cfg, _ := config.New(g)

	trunk, _ := g.CurrentBranch()
	cfg.SetTrunk(trunk)

	// Create branch with PR
	g.CreateBranch("feature-a")
	cfg.SetParent("feature-a", trunk)
	cfg.SetPR("feature-a", 789)

	// Verify PR exists
	pr, _ := cfg.GetPR("feature-a")
	if pr != 789 {
		t.Fatalf("expected PR 789, got %d", pr)
	}

	// Unlink
	cfg.RemovePR("feature-a")

	// Verify PR is gone
	_, err := cfg.GetPR("feature-a")
	if err == nil {
		t.Error("expected PR to be removed")
	}
}

func TestUnlinkIdempotent(t *testing.T) {
	dir := setupTestRepo(t)

	g := git.New(dir)
	cfg, _ := config.New(g)

	trunk, _ := g.CurrentBranch()
	cfg.SetTrunk(trunk)

	// Create branch without PR
	g.CreateBranch("feature-a")
	cfg.SetParent("feature-a", trunk)

	// Unlink should not error even if no PR
	err := cfg.RemovePR("feature-a")
	if err != nil {
		t.Errorf("RemovePR should be idempotent: %v", err)
	}
}
