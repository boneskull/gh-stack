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

// TestHandleMergedBranchYesDeletes verifies that passing yes=true causes
// handleMergedBranch to delete the branch and clear stack config without
// showing an interactive prompt, even when stdin/stdout are a TTY.
func TestHandleMergedBranchYesDeletes(t *testing.T) {
	cfg, dir := setupTestRepoWithDir(t)
	g := git.New(dir)
	s := style.New()

	// Determine trunk from the current HEAD branch.
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		t.Fatalf("could not determine trunk branch: %v", err)
	}
	trunk := strings.TrimSpace(string(out))

	if err := cfg.SetTrunk(trunk); err != nil {
		t.Fatalf("SetTrunk failed: %v", err)
	}

	// Create feature-a with a commit so git can delete it.
	if err := exec.Command("git", "-C", dir, "checkout", "-b", "feature-a").Run(); err != nil {
		t.Fatalf("create branch failed: %v", err)
	}
	if err := exec.Command("git", "-C", dir, "commit", "--allow-empty", "-m", "feat").Run(); err != nil {
		t.Fatalf("commit failed: %v", err)
	}
	if err := exec.Command("git", "-C", dir, "checkout", trunk).Run(); err != nil {
		t.Fatalf("checkout trunk failed: %v", err)
	}

	// Register feature-a in the stack.
	if err := cfg.SetParent("feature-a", trunk); err != nil {
		t.Fatalf("SetParent failed: %v", err)
	}
	if err := cfg.SetPR("feature-a", 42); err != nil {
		t.Fatalf("SetPR failed: %v", err)
	}

	currentBranch := trunk
	action := handleMergedBranch(g, cfg, "feature-a", trunk, &currentBranch, true, s)

	if action != mergedActionDelete {
		t.Errorf("expected mergedActionDelete, got %v", action)
	}

	// Stack config should be cleared.
	if _, err := cfg.GetParent("feature-a"); err == nil {
		t.Error("expected stackParent to be removed after --yes delete")
	}
	if _, err := cfg.GetPR("feature-a"); err == nil {
		t.Error("expected stackPR to be removed after --yes delete")
	}

	// Git branch should be gone.
	branchOut, _ := exec.Command("git", "-C", dir, "branch", "--list", "feature-a").Output()
	if len(strings.TrimSpace(string(branchOut))) != 0 {
		t.Errorf("expected feature-a to be deleted, but git branch still shows: %q", string(branchOut))
	}
}
