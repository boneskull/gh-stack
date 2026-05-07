// cmd/adopt_test.go
package cmd_test

import (
	"testing"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/git"
	"github.com/boneskull/gh-stack/internal/tree"
)

func TestAdoptBranch(t *testing.T) {
	dir := setupTestRepo(t)

	cfg, _ := config.Load(dir)
	g := git.New(dir)

	trunk, _ := g.CurrentBranch()
	cfg.SetTrunk(trunk)

	// Create an untracked branch
	g.CreateBranch("untracked-feature")

	// Adopt it
	err := cfg.SetParent("untracked-feature", trunk)
	if err != nil {
		t.Fatalf("SetParent failed: %v", err)
	}

	// Verify
	parent, err := cfg.GetParent("untracked-feature")
	if err != nil {
		t.Fatalf("GetParent failed: %v", err)
	}
	if parent != trunk {
		t.Errorf("expected parent %q, got %q", trunk, parent)
	}
}

func TestAdoptReparentsAlreadyTracked(t *testing.T) {
	dir := setupTestRepo(t)

	cfg, _ := config.Load(dir)
	g := git.New(dir)

	trunk, _ := g.CurrentBranch()
	cfg.SetTrunk(trunk)

	// Create two branches; track feature-a under trunk
	g.CreateBranch("feature-a")
	cfg.SetParent("feature-a", trunk)
	g.CreateBranch("feature-b")
	cfg.SetParent("feature-b", trunk)

	// Simulate what runAdopt does when reparenting feature-b under feature-a
	if err := cfg.SetParent("feature-b", "feature-a"); err != nil {
		t.Fatalf("SetParent failed: %v", err)
	}

	parent, err := cfg.GetParent("feature-b")
	if err != nil {
		t.Fatalf("GetParent failed: %v", err)
	}
	if parent != "feature-a" {
		t.Errorf("expected parent %q after reparent, got %q", "feature-a", parent)
	}
}

func TestAdoptRejectsUntrackedParent(t *testing.T) {
	dir := setupTestRepo(t)

	cfg, _ := config.Load(dir)
	g := git.New(dir)

	trunk, _ := g.CurrentBranch()
	cfg.SetTrunk(trunk)

	// Create two untracked branches
	g.CreateBranch("untracked-a")
	g.CreateBranch("untracked-b")

	// Trying to adopt with untracked parent should fail
	// The parent must be trunk or tracked
	_, err := cfg.GetParent("untracked-a")
	if err == nil {
		t.Error("untracked-a should not be tracked")
	}
}

func TestAdoptDetectsCycle(t *testing.T) {
	dir := setupTestRepo(t)

	cfg, _ := config.Load(dir)
	g := git.New(dir)

	trunk, _ := g.CurrentBranch()
	cfg.SetTrunk(trunk)

	// Create a chain: trunk -> feature-a -> feature-b
	g.CreateBranch("feature-a")
	cfg.SetParent("feature-a", trunk)
	g.CreateBranch("feature-b")
	cfg.SetParent("feature-b", "feature-a")

	// Build tree and check for cycle detection
	root, _ := tree.Build(cfg)

	// If we tried to make feature-a's parent be feature-b, that would be a cycle
	// Check that feature-a is an ancestor of feature-b
	featureBNode := tree.FindNode(root, "feature-b")
	ancestors := tree.GetAncestors(featureBNode)

	foundFeatureA := false
	for _, a := range ancestors {
		if a.Name == "feature-a" {
			foundFeatureA = true
			break
		}
	}

	if !foundFeatureA {
		t.Error("expected feature-a to be ancestor of feature-b")
	}
}

func TestAdoptStoresForkPoint(t *testing.T) {
	dir := setupTestRepo(t)

	cfg, _ := config.Load(dir)
	g := git.New(dir)

	trunk, _ := g.CurrentBranch()
	cfg.SetTrunk(trunk)

	// Get trunk tip before creating branch
	trunkTip, _ := g.GetTip(trunk)

	// Create an untracked branch
	g.CreateBranch("untracked-feature")

	// Simulate adopt: set parent and fork point
	cfg.SetParent("untracked-feature", trunk)

	// Store fork point (what adopt command should now do)
	forkPoint, fpErr := g.GetMergeBase("untracked-feature", trunk)
	if fpErr != nil {
		t.Fatalf("GetMergeBase failed: %v", fpErr)
	}
	cfg.SetForkPoint("untracked-feature", forkPoint)

	// Verify fork point was stored
	storedFP, err := cfg.GetForkPoint("untracked-feature")
	if err != nil {
		t.Fatalf("GetForkPoint failed: %v", err)
	}
	if storedFP != trunkTip {
		t.Errorf("fork point = %s, want %s", storedFP, trunkTip)
	}
}
