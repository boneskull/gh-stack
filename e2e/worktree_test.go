// e2e/worktree_test.go
package e2e_test

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRestackWithWorktree(t *testing.T) {
	env := NewTestEnv(t)
	env.MustRun("init")

	// Create a 3-branch stack: main -> feature-a -> feature-b -> feature-c
	env.MustRun("create", "feature-a")
	env.CreateCommit("feature a work")

	env.MustRun("create", "feature-b")
	env.CreateCommit("feature b work")

	env.MustRun("create", "feature-c")
	env.CreateCommit("feature c work")

	// Move feature-b to a linked worktree
	wtPath := filepath.Join(t.TempDir(), "wt-feature-b")
	env.CreateWorktree("feature-b", wtPath)

	// Add a commit to main to force restack
	env.Git("checkout", "main")
	env.CreateCommit("main moved forward")

	// Restack from feature-a with --worktrees
	env.Git("checkout", "feature-a")
	result := env.MustRun("restack", "--worktrees")

	// Verify the output mentions the worktree
	if !result.ContainsStdout("worktree") {
		t.Errorf("expected output to mention worktree, got:\n%s", result.Stdout)
	}

	// Verify ancestry chain is correct
	env.AssertAncestor("main", "feature-a")
	env.AssertAncestor("feature-a", "feature-b")
	env.AssertAncestor("feature-b", "feature-c")
	env.AssertNoRebaseInProgress()
}

func TestRestackWithWorktreeConflict(t *testing.T) {
	env := NewTestEnv(t)
	env.MustRun("init")

	conflictFile := "shared.txt"

	// Initial state on main
	env.WriteFile(conflictFile, "initial content\n")
	env.Git("add", conflictFile)
	env.Git("commit", "-m", "initial shared.txt")

	// Create feature-a (doesn't modify shared.txt)
	env.MustRun("create", "feature-a")
	env.CreateCommit("feature-a work")

	// Create feature-b (modifies shared.txt -- will conflict)
	env.MustRun("create", "feature-b")
	env.WriteFile(conflictFile, "feature-b modified this\n")
	env.Git("add", conflictFile)
	env.Git("commit", "-m", "feature-b: modify shared.txt")

	// Switch away from feature-b so we can create a worktree for it
	env.Git("checkout", "main")

	// Move feature-b to a linked worktree
	wtPath := filepath.Join(t.TempDir(), "wt-feature-b")
	env.CreateWorktree("feature-b", wtPath)

	// Move main forward with a conflicting change (already on main)
	env.WriteFile(conflictFile, "main modified this differently\n")
	env.Git("add", conflictFile)
	env.Git("commit", "-m", "main: modify shared.txt")

	// Restack from feature-a -- should conflict on feature-b
	env.Git("checkout", "feature-a")
	result := env.Run("restack", "--worktrees")

	if result.Success() {
		t.Fatal("expected restack to fail on conflict")
	}
	if !result.ContainsStdout("CONFLICT") {
		t.Errorf("expected CONFLICT in output, got:\n%s", result.Stdout)
	}
	if !result.ContainsStdout(wtPath) {
		t.Errorf("expected worktree path %q in conflict output, got:\n%s", wtPath, result.Stdout)
	}

	// The rebase should be in progress in the worktree, not the main repo
	rebaseMerge := filepath.Join(wtPath, ".git")
	// In a linked worktree, .git is a file. Rebase state lives in the
	// per-worktree gitdir, which we can find via git rev-parse.
	// Just verify the worktree has a rebase in progress via a git command.
	wtRebaseStatus := env.GitInWorktree(wtPath, "status")
	if !containsString(wtRebaseStatus, "rebase") {
		t.Errorf("expected rebase in progress in worktree, git status:\n%s\n.git: %s", wtRebaseStatus, rebaseMerge)
	}

	// Resolve the conflict in the worktree
	conflictPath := filepath.Join(wtPath, conflictFile)
	if err := os.WriteFile(conflictPath, []byte("resolved content\n"), 0644); err != nil {
		t.Fatalf("failed to write resolved file: %v", err)
	}
	env.GitInWorktree(wtPath, "add", conflictFile)

	// Continue from the main repo
	env.MustRun("continue")

	// Verify ancestry after resolution
	env.AssertAncestor("main", "feature-a")
	env.AssertAncestor("feature-a", "feature-b")
}

func TestRestackWithoutWorktreeFlagErrors(t *testing.T) {
	env := NewTestEnv(t)
	env.MustRun("init")

	// Create a 2-branch stack
	env.MustRun("create", "feature-a")
	env.CreateCommit("feature a work")

	env.MustRun("create", "feature-b")
	env.CreateCommit("feature b work")

	// Switch away from feature-b so we can create a worktree for it
	env.Git("checkout", "main")

	// Move feature-b to a linked worktree
	wtPath := filepath.Join(t.TempDir(), "wt-feature-b")
	env.CreateWorktree("feature-b", wtPath)

	// Move main forward to force restack (already on main)
	env.CreateCommit("main moved forward")

	// Restack WITHOUT --worktrees should fail when hitting the worktree branch
	env.Git("checkout", "feature-a")
	result := env.Run("restack")

	// Without --worktrees, git checkout will fail for the branch in the worktree
	if result.Success() {
		t.Error("expected restack to fail without --worktrees when branch is in a worktree")
	}
}

func TestSyncWithWorktree(t *testing.T) {
	// sync requires a real GitHub remote which we can't simulate in E2E tests.
	// Instead, verify the --worktrees flag is accepted by the sync command
	// and test the cascade-with-worktrees behavior (which sync delegates to)
	// via the restack tests above.
	env := NewTestEnv(t)
	env.MustRun("init")

	// Verify --worktrees flag is recognized by sync (help output check)
	result := env.Run("sync", "--help")
	if !result.ContainsStdout("--worktrees") {
		t.Errorf("expected sync --help to show --worktrees flag, got:\n%s", result.Stdout)
	}
}

func TestRestackAbortWithWorktree(t *testing.T) {
	env := NewTestEnv(t)
	env.MustRun("init")

	conflictFile := "shared.txt"

	// Initial state on main
	env.WriteFile(conflictFile, "initial content\n")
	env.Git("add", conflictFile)
	env.Git("commit", "-m", "initial shared.txt")

	// Create feature-a
	env.MustRun("create", "feature-a")
	env.CreateCommit("feature-a work")

	// Create feature-b (modifies shared.txt)
	env.MustRun("create", "feature-b")
	env.WriteFile(conflictFile, "feature-b modified this\n")
	env.Git("add", conflictFile)
	env.Git("commit", "-m", "feature-b: modify shared.txt")

	// Switch away and create worktree
	env.Git("checkout", "main")
	wtPath := filepath.Join(t.TempDir(), "wt-feature-b")
	env.CreateWorktree("feature-b", wtPath)

	// Move main forward with conflict
	env.WriteFile(conflictFile, "main modified this differently\n")
	env.Git("add", conflictFile)
	env.Git("commit", "-m", "main: modify shared.txt")

	// Restack from feature-a -- should conflict on feature-b in worktree
	env.Git("checkout", "feature-a")
	result := env.Run("restack", "--worktrees")
	if result.Success() {
		t.Fatal("expected conflict")
	}

	// Abort should work and clean up the rebase in the worktree
	env.MustRun("abort")

	// Verify worktree is clean (no rebase in progress)
	wtStatus := env.GitInWorktree(wtPath, "status")
	if containsString(wtStatus, "rebase") {
		t.Errorf("expected no rebase in progress after abort, got:\n%s", wtStatus)
	}
}

// containsString is a simple helper for string containment.
func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
