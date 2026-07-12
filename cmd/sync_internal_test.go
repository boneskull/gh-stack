// cmd/sync_internal_test.go
//
// Internal tests (package cmd) for unexported sync helpers.
package cmd

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/boneskull/gh-stack/internal/git"
	"github.com/boneskull/gh-stack/internal/style"
)

// forceInteractive returns an isInteractive func that always returns true,
// allowing tests to exercise the interactive branch of handleMergedBranch
// regardless of whether stdin/stdout are a TTY.
func forceInteractive() func() bool { return func() bool { return true } }

// forceNonInteractive returns an isInteractive func that always returns false.
func forceNonInteractive() func() bool { return func() bool { return false } }

// TestHandleMergedBranchYesDeletesInInteractiveMode verifies that yes=true
// causes handleMergedBranch to delete without prompting even when the terminal
// is reported as interactive. Without the yes flag this would reach the prompt,
// so forcing isInteractive=true proves yes overrides the interactive branch.
func TestHandleMergedBranchYesDeletesInInteractiveMode(t *testing.T) {
	cfg, dir := setupTestRepoWithDir(t)
	g := git.New(dir)
	s := style.New()

	trunk := setupBranches(t, dir, cfg)

	currentBranch := trunk
	// isInteractive forced to true: without yes=true the function would reach
	// the interactive prompt path and hang waiting for stdin input.
	action := handleMergedBranch(g, cfg, "feature-a", trunk, &currentBranch, true, forceInteractive(), s)

	if action != mergedActionDelete {
		t.Errorf("expected mergedActionDelete with yes=true, got %v", action)
	}

	assertBranchDeleted(t, g, dir, "feature-a", cfg)
}

// TestHandleMergedBranchNoYesNonInteractiveDeletes verifies the existing
// non-interactive default: yes=false with a non-interactive terminal still
// deletes (no prompt).
func TestHandleMergedBranchNoYesNonInteractiveDeletes(t *testing.T) {
	cfg, dir := setupTestRepoWithDir(t)
	g := git.New(dir)
	s := style.New()

	trunk := setupBranches(t, dir, cfg)

	currentBranch := trunk
	action := handleMergedBranch(g, cfg, "feature-a", trunk, &currentBranch, false, forceNonInteractive(), s)

	if action != mergedActionDelete {
		t.Errorf("expected mergedActionDelete for non-interactive+yes=false, got %v", action)
	}

	assertBranchDeleted(t, g, dir, "feature-a", cfg)
}

// setupBranches creates a trunk+feature-a repo for handleMergedBranch tests and
// returns the trunk branch name. feature-a is registered in the stack with a
// commit so git will allow force-deletion.
func setupBranches(t *testing.T, dir string, cfg interface {
	SetTrunk(string) error
	SetParent(string, string) error
	SetPR(string, int) error
}) string {
	t.Helper()

	out, err := exec.Command("git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		t.Fatalf("could not determine trunk branch: %v", err)
	}
	trunk := strings.TrimSpace(string(out))

	if err := cfg.SetTrunk(trunk); err != nil {
		t.Fatalf("SetTrunk failed: %v", err)
	}
	if err := exec.Command("git", "-C", dir, "checkout", "-b", "feature-a").Run(); err != nil {
		t.Fatalf("create branch failed: %v", err)
	}
	if err := exec.Command("git", "-C", dir, "commit", "--allow-empty", "-m", "feat").Run(); err != nil {
		t.Fatalf("commit failed: %v", err)
	}
	if err := exec.Command("git", "-C", dir, "checkout", trunk).Run(); err != nil {
		t.Fatalf("checkout trunk failed: %v", err)
	}
	if err := cfg.SetParent("feature-a", trunk); err != nil {
		t.Fatalf("SetParent failed: %v", err)
	}
	if err := cfg.SetPR("feature-a", 42); err != nil {
		t.Fatalf("SetPR failed: %v", err)
	}
	return trunk
}

// assertBranchDeleted checks that the git branch no longer exists and that
// stackParent / stackPR have been cleared from config.
func assertBranchDeleted(t *testing.T, g *git.Git, dir string, branch string, cfg interface {
	GetParent(string) (string, error)
	GetPR(string) (int, error)
}) {
	t.Helper()

	if g.BranchExists(branch) {
		t.Errorf("expected branch %q to be deleted, but it still exists", branch)
	}
	if _, err := cfg.GetParent(branch); err == nil {
		t.Errorf("expected stackParent to be removed after delete, but it is still set for %q", branch)
	}
	if _, err := cfg.GetPR(branch); err == nil {
		t.Errorf("expected stackPR to be removed after delete, but it is still set for %q", branch)
	}
}
