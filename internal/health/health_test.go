// internal/health/health_test.go
package health_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/git"
	"github.com/boneskull/gh-stack/internal/health"
)

// setupTestRepo creates a temp git repo with an initial commit and returns
// the repo path, a *git.Git, and a *config.Config. The default branch is
// configured as the stack trunk.
func setupTestRepo(t *testing.T) (string, *git.Git, *config.Config) {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}

	run("init")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "Test")

	f := filepath.Join(dir, "README.md")
	os.WriteFile(f, []byte("# Test"), 0644)
	run("add", ".")
	run("commit", "-m", "initial")

	g := git.New(dir)
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("config.Load failed: %v", err)
	}

	trunk, err := g.CurrentBranch()
	if err != nil {
		t.Fatalf("CurrentBranch failed: %v", err)
	}
	if err := cfg.SetTrunk(trunk); err != nil {
		t.Fatalf("SetTrunk failed: %v", err)
	}

	return dir, g, cfg
}

// addTrackedBranch creates a branch off parent with one commit and
// configures stack metadata (parent + fork point).
func addTrackedBranch(t *testing.T, dir string, g *git.Git, cfg *config.Config, branch, parent string) {
	t.Helper()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}

	// Record fork point before creating branch
	parentTip, err := g.GetTip(parent)
	if err != nil {
		t.Fatalf("GetTip(%s) failed: %v", parent, err)
	}

	run("checkout", "-b", branch, parent)

	f := filepath.Join(dir, branch+".txt")
	os.WriteFile(f, []byte(branch+" content"), 0644)
	run("add", ".")
	run("commit", "-m", branch+" commit")

	if err := cfg.SetParent(branch, parent); err != nil {
		t.Fatalf("SetParent failed: %v", err)
	}
	if err := cfg.SetForkPoint(branch, parentTip); err != nil {
		t.Fatalf("SetForkPoint failed: %v", err)
	}
}

func TestCheckBranch_Healthy(t *testing.T) {
	dir, g, cfg := setupTestRepo(t)
	trunk, _ := g.CurrentBranch()
	addTrackedBranch(t, dir, g, cfg, "feature", trunk)

	issues := health.CheckBranch(g, cfg, "feature")
	if len(issues) != 0 {
		t.Errorf("expected no issues, got %v", issues)
	}
}

func TestCheckBranch_MissingBranch(t *testing.T) {
	_, g, cfg := setupTestRepo(t)

	// Track a branch that doesn't exist in git
	cfg.SetParent("ghost", "main")

	issues := health.CheckBranch(g, cfg, "ghost")
	if len(issues) == 0 {
		t.Fatal("expected issues for missing branch")
	}
	if issues[0].Message != "Branch does not exist locally" {
		t.Errorf("unexpected message: %s", issues[0].Message)
	}
	if issues[0].Fixable {
		t.Error("missing branch should not be fixable")
	}
}

func TestCheckBranch_MissingParent(t *testing.T) {
	dir, g, cfg := setupTestRepo(t)
	trunk, _ := g.CurrentBranch()

	// Create a branch tracked against a parent that will be deleted
	addTrackedBranch(t, dir, g, cfg, "child", trunk)

	// Now create "middle" as parent, point child at it, then delete it
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
	run("checkout", trunk)
	run("checkout", "-b", "middle")
	run("checkout", trunk)

	cfg.SetParent("child", "middle")

	// Delete the parent branch
	run("branch", "-D", "middle")

	issues := health.CheckBranch(g, cfg, "child")
	if len(issues) == 0 {
		t.Fatal("expected issues for missing parent")
	}
	if got := issues[0].Message; got != "Parent branch middle does not exist locally" {
		t.Errorf("unexpected message: %s", got)
	}
}

func TestCheckBranch_MissingTrunk(t *testing.T) {
	dir, g, cfg := setupTestRepo(t)
	trunk, _ := g.CurrentBranch()

	addTrackedBranch(t, dir, g, cfg, "feature", trunk)

	// Point the feature's parent at trunk, then rename trunk so it disappears
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}

	run("checkout", "feature")
	run("branch", "-m", trunk, "gone-trunk")

	// feature's parent is still trunk (which no longer exists)
	issues := health.CheckBranch(g, cfg, "feature")
	if len(issues) == 0 {
		t.Fatal("expected issues for missing trunk")
	}
	found := false
	for _, iss := range issues {
		if iss.Message == "Trunk branch "+trunk+" does not exist locally" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected trunk-missing message, got %v", issues)
	}
}

func TestCheckBranch_NoForkPoint(t *testing.T) {
	dir, g, cfg := setupTestRepo(t)
	trunk, _ := g.CurrentBranch()

	addTrackedBranch(t, dir, g, cfg, "feature", trunk)

	// Remove the fork point from config
	cfg.RemoveForkPoint("feature")

	issues := health.CheckBranch(g, cfg, "feature")
	if len(issues) == 0 {
		t.Fatal("expected issues for missing fork point")
	}
	if issues[0].Message != "No fork point stored" {
		t.Errorf("unexpected message: %s", issues[0].Message)
	}
	if !issues[0].Fixable {
		t.Error("missing fork point should be fixable")
	}
}

func TestCheckBranch_NonexistentForkPointCommit(t *testing.T) {
	dir, g, cfg := setupTestRepo(t)
	trunk, _ := g.CurrentBranch()

	addTrackedBranch(t, dir, g, cfg, "feature", trunk)

	// Set fork point to a SHA that doesn't exist
	fakeSHA := "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	cfg.SetForkPoint("feature", fakeSHA)

	issues := health.CheckBranch(g, cfg, "feature")
	if len(issues) == 0 {
		t.Fatal("expected issues for nonexistent fork point commit")
	}
	if !issues[0].Fixable {
		t.Error("nonexistent fork point should be fixable")
	}
}

func TestCheckBranch_ForkPointNotAncestor(t *testing.T) {
	dir, g, cfg := setupTestRepo(t)
	trunk, _ := g.CurrentBranch()

	addTrackedBranch(t, dir, g, cfg, "feature", trunk)

	// Create an orphan commit that exists but isn't an ancestor of feature
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}

	run("checkout", trunk)
	os.WriteFile(filepath.Join(dir, "diverged.txt"), []byte("diverged"), 0644)
	run("add", ".")
	run("commit", "-m", "diverged commit")
	divergedSHA, _ := g.GetTip("HEAD")

	// Set fork point to this diverged commit (exists, but not ancestor of feature)
	cfg.SetForkPoint("feature", divergedSHA)

	issues := health.CheckBranch(g, cfg, "feature")
	if len(issues) == 0 {
		t.Fatal("expected issues for fork point not ancestor")
	}
	if !issues[0].Fixable {
		t.Error("fork point not-ancestor should be fixable")
	}
}

func TestCheckBranch_Drift(t *testing.T) {
	dir, g, cfg := setupTestRepo(t)
	trunk, _ := g.CurrentBranch()

	addTrackedBranch(t, dir, g, cfg, "feature", trunk)

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}

	// Rebase feature onto a new commit on trunk, which moves the actual
	// merge-base forward while the stored fork point stays at the old value.
	run("checkout", trunk)
	os.WriteFile(filepath.Join(dir, "trunk2.txt"), []byte("new trunk"), 0644)
	run("add", ".")
	run("commit", "-m", "trunk advance")

	run("checkout", "feature")
	run("rebase", trunk)

	// Now the stored fork point is the old trunk tip, but merge-base is
	// the new trunk tip. That's drift.
	issues := health.CheckBranch(g, cfg, "feature")
	if len(issues) == 0 {
		t.Fatal("expected drift issues")
	}

	// The last issue in a drift result should mention "Drift detected"
	last := issues[len(issues)-1]
	if last.Message != "Drift detected: stored fork point does not match merge-base" {
		t.Errorf("expected drift message, got: %s", last.Message)
	}
	if !last.Fixable {
		t.Error("drift should be fixable")
	}
}
