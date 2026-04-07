package detect_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/boneskull/gh-stack/internal/detect"
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
	run("config", "commit.gpgsign", "false")

	f := filepath.Join(dir, "README.md")
	if err := os.WriteFile(f, []byte("# Test"), 0644); err != nil {
		t.Fatalf("WriteFile README.md: %v", err)
	}
	run("add", ".")
	run("commit", "-m", "initial")

	return dir
}

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

// Linear stack: main -> A -> B -> C (untracked)
// C should detect B as parent with Medium confidence
func TestDetectParentLocal_LinearStack(t *testing.T) {
	dir := setupTestRepo(t)
	g := git.New(dir)
	trunk, _ := g.CurrentBranch()

	g.CreateAndCheckout("feature-a")
	addCommit(t, dir, "a.txt", "a")

	g.CreateAndCheckout("feature-b")
	addCommit(t, dir, "b.txt", "b")

	g.CreateAndCheckout("feature-c")
	addCommit(t, dir, "c.txt", "c")

	tracked := []string{"feature-a", "feature-b"}
	result, err := detect.DetectParentLocal("feature-c", tracked, trunk, g)
	if err != nil {
		t.Fatalf("DetectParentLocal failed: %v", err)
	}
	if result.Parent != "feature-b" {
		t.Errorf("expected parent 'feature-b', got %q", result.Parent)
	}
	if result.Confidence != detect.Medium {
		t.Errorf("expected Medium confidence, got %v", result.Confidence)
	}
}

// Two branches off main at the same commit: ambiguous
func TestDetectParentLocal_Ambiguous(t *testing.T) {
	dir := setupTestRepo(t)
	g := git.New(dir)
	trunk, _ := g.CurrentBranch()

	g.CreateBranch("feature-a")
	g.CreateBranch("feature-b")

	g.CreateAndCheckout("feature-c")
	addCommit(t, dir, "c.txt", "c")

	tracked := []string{"feature-a", "feature-b"}
	result, err := detect.DetectParentLocal("feature-c", tracked, trunk, g)
	if err != nil {
		t.Fatalf("DetectParentLocal failed: %v", err)
	}
	if result.Confidence != detect.Ambiguous {
		t.Errorf("expected Ambiguous confidence, got %v", result.Confidence)
	}
	if len(result.Candidates) < 2 {
		t.Errorf("expected at least 2 tied candidates, got %v", result.Candidates)
	}
}

// Branch forked directly from trunk; no tracked branches are closer.
func TestDetectParentLocal_TrunkIsParent(t *testing.T) {
	dir := setupTestRepo(t)
	g := git.New(dir)
	trunk, _ := g.CurrentBranch()

	g.CreateAndCheckout("feature-a")
	addCommit(t, dir, "a.txt", "a")

	g.Checkout(trunk)
	addCommit(t, dir, "main1.txt", "main1")

	g.CreateAndCheckout("feature-x")
	addCommit(t, dir, "x.txt", "x")

	tracked := []string{"feature-a"}
	result, err := detect.DetectParentLocal("feature-x", tracked, trunk, g)
	if err != nil {
		t.Fatalf("DetectParentLocal failed: %v", err)
	}
	if result.Parent != trunk {
		t.Errorf("expected parent %q (trunk), got %q", trunk, result.Parent)
	}
	if result.Confidence != detect.Medium {
		t.Errorf("expected Medium confidence, got %v", result.Confidence)
	}
}

// No tracked branches, only trunk as candidate
func TestDetectParentLocal_NoTrackedBranches(t *testing.T) {
	dir := setupTestRepo(t)
	g := git.New(dir)
	trunk, _ := g.CurrentBranch()

	g.CreateAndCheckout("feature-x")
	addCommit(t, dir, "x.txt", "x")

	result, err := detect.DetectParentLocal("feature-x", nil, trunk, g)
	if err != nil {
		t.Fatalf("DetectParentLocal failed: %v", err)
	}
	if result.Parent != trunk {
		t.Errorf("expected parent %q (trunk), got %q", trunk, result.Parent)
	}
	if result.Confidence != detect.Medium {
		t.Errorf("expected Medium confidence, got %v", result.Confidence)
	}
}

// DetectParent with nil GitHub client should produce the same result as DetectParentLocal
func TestDetectParent_NilGitHub(t *testing.T) {
	dir := setupTestRepo(t)
	g := git.New(dir)
	trunk, _ := g.CurrentBranch()

	g.CreateAndCheckout("feature-a")
	addCommit(t, dir, "a.txt", "a")

	g.CreateAndCheckout("feature-b")
	addCommit(t, dir, "b.txt", "b")

	tracked := []string{"feature-a"}
	result, err := detect.DetectParent("feature-b", tracked, trunk, g, nil)
	if err != nil {
		t.Fatalf("DetectParent failed: %v", err)
	}
	if result.Parent != "feature-a" {
		t.Errorf("expected parent 'feature-a', got %q", result.Parent)
	}
	if result.Confidence != detect.Medium {
		t.Errorf("expected Medium confidence, got %v", result.Confidence)
	}
	if result.PRNumber != 0 {
		t.Errorf("expected no PR number, got %d", result.PRNumber)
	}
}

// FindUntrackedCandidates should return branches not in tracked set and not trunk
func TestFindUntrackedCandidates(t *testing.T) {
	dir := setupTestRepo(t)
	g := git.New(dir)
	trunk, _ := g.CurrentBranch()

	g.CreateBranch("tracked-a")
	g.CreateBranch("tracked-b")
	g.CreateBranch("untracked-x")
	g.CreateBranch("untracked-y")

	tracked := []string{"tracked-a", "tracked-b"}
	candidates, err := detect.FindUntrackedCandidates(g, tracked, trunk)
	if err != nil {
		t.Fatalf("FindUntrackedCandidates failed: %v", err)
	}

	candidateSet := make(map[string]bool)
	for _, c := range candidates {
		candidateSet[c] = true
	}

	// Trunk and tracked branches should NOT be in candidates
	for _, excluded := range []string{trunk, "tracked-a", "tracked-b"} {
		if candidateSet[excluded] {
			t.Errorf("expected %q to be excluded from candidates", excluded)
		}
	}

	// Untracked branches SHOULD be in candidates
	for _, expected := range []string{"untracked-x", "untracked-y"} {
		if !candidateSet[expected] {
			t.Errorf("expected %q in candidates, got %v", expected, candidates)
		}
	}
}

// FindUntrackedCandidates with empty tracked list
func TestFindUntrackedCandidates_EmptyTracked(t *testing.T) {
	dir := setupTestRepo(t)
	g := git.New(dir)
	trunk, _ := g.CurrentBranch()

	g.CreateBranch("some-branch")

	candidates, err := detect.FindUntrackedCandidates(g, nil, trunk)
	if err != nil {
		t.Fatalf("FindUntrackedCandidates failed: %v", err)
	}

	if len(candidates) != 1 || candidates[0] != "some-branch" {
		t.Errorf("expected [some-branch], got %v", candidates)
	}
}

func TestConfidence_String(t *testing.T) {
	tests := []struct {
		c    detect.Confidence
		want string
	}{
		{detect.Ambiguous, "ambiguous"},
		{detect.Medium, "medium"},
		{detect.High, "high"},
		{detect.Confidence(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.c.String(); got != tt.want {
			t.Errorf("Confidence(%d).String() = %q, want %q", tt.c, got, tt.want)
		}
	}
}
