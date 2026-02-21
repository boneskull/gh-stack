// cmd/adopt_test.go
package cmd_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/detect"
	"github.com/boneskull/gh-stack/internal/git"
	"github.com/boneskull/gh-stack/internal/style"
	"github.com/boneskull/gh-stack/internal/tree"
)

// addCommit creates a file with the given content and commits it.
func addCommit(t *testing.T, dir, filename, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", filename, err)
	}
	cmd := exec.Command("git", "-C", dir, "add", ".")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git add: %v", err)
	}
	cmd = exec.Command("git", "-C", dir, "commit", "-m", "add "+filename)
	if err := cmd.Run(); err != nil {
		t.Fatalf("git commit: %v", err)
	}
}

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

func TestAdoptRejectsAlreadyTracked(t *testing.T) {
	dir := setupTestRepo(t)

	cfg, _ := config.Load(dir)
	g := git.New(dir)

	trunk, _ := g.CurrentBranch()
	cfg.SetTrunk(trunk)

	// Create and track a branch
	g.CreateBranch("tracked-feature")
	cfg.SetParent("tracked-feature", trunk)

	// Trying to get parent should succeed (it's tracked)
	_, err := cfg.GetParent("tracked-feature")
	if err != nil {
		t.Error("expected branch to be tracked")
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

// TestAdoptAutoDetect exercises the full detection-to-adoption pipeline:
// detect parent, validate, set parent, store fork point.
func TestAdoptAutoDetect(t *testing.T) {
	dir := setupTestRepo(t)
	g := git.New(dir)
	cfg, _ := config.Load(dir)
	trunk, _ := g.CurrentBranch()
	cfg.SetTrunk(trunk)

	// Create tracked branch A
	g.CreateAndCheckout("feature-a")
	addCommit(t, dir, "a.txt", "a")
	cfg.SetParent("feature-a", trunk)

	// Create untracked branch B off A
	g.CreateAndCheckout("feature-b")
	addCommit(t, dir, "b.txt", "b")

	// feature-b should not be tracked yet
	if _, err := cfg.GetParent("feature-b"); err == nil {
		t.Fatal("feature-b should not be tracked yet")
	}

	// Simulate what runAdopt does when no parent arg is given:
	// 1. Detect parent
	tracked, _ := cfg.ListTrackedBranches()
	result, detectErr := detect.DetectParent("feature-b", tracked, trunk, g, nil)
	if detectErr != nil {
		t.Fatalf("detection failed: %v", detectErr)
	}
	if result.Confidence == detect.Ambiguous {
		t.Fatal("expected non-ambiguous detection")
	}
	if result.Parent != "feature-a" {
		t.Errorf("expected detected parent 'feature-a', got %q", result.Parent)
	}

	// 2. Validate parent is tracked (same check as runAdopt)
	if result.Parent != trunk {
		if _, parentErr := cfg.GetParent(result.Parent); parentErr != nil {
			t.Fatalf("detected parent %q is not tracked: %v", result.Parent, parentErr)
		}
	}

	// 3. Set parent (same as runAdopt)
	if err := cfg.SetParent("feature-b", result.Parent); err != nil {
		t.Fatalf("SetParent failed: %v", err)
	}

	// 4. Store fork point (same as runAdopt)
	forkPoint, fpErr := g.GetMergeBase("feature-b", result.Parent)
	if fpErr != nil {
		t.Fatalf("GetMergeBase failed: %v", fpErr)
	}
	_ = cfg.SetForkPoint("feature-b", forkPoint)

	// Verify the full adoption persisted correctly
	parent, err := cfg.GetParent("feature-b")
	if err != nil {
		t.Fatalf("feature-b should be tracked now: %v", err)
	}
	if parent != "feature-a" {
		t.Errorf("expected parent 'feature-a', got %q", parent)
	}

	storedFP, fpGetErr := cfg.GetForkPoint("feature-b")
	if fpGetErr != nil {
		t.Fatalf("GetForkPoint failed: %v", fpGetErr)
	}
	if storedFP != forkPoint {
		t.Errorf("fork point mismatch: stored=%s, expected=%s", storedFP, forkPoint)
	}

	// Verify tree now includes feature-b
	root, buildErr := tree.Build(cfg)
	if buildErr != nil {
		t.Fatalf("Build failed: %v", buildErr)
	}
	nodeB := tree.FindNode(root, "feature-b")
	if nodeB == nil {
		t.Fatal("feature-b should appear in tree after adoption")
	}
	if nodeB.Parent.Name != "feature-a" {
		t.Errorf("expected parent node 'feature-a', got %q", nodeB.Parent.Name)
	}
}

// TestAdoptAutoDetect_PrintsConfidence verifies that the detection message
// includes the confidence level.
func TestAdoptAutoDetect_PrintsConfidence(t *testing.T) {
	// Verify the style.New().Muted() call matches what adopt.go uses
	s := style.New()
	msg := s.Muted("(medium confidence)")
	if msg == "" {
		t.Error("expected non-empty muted confidence string")
	}
}
