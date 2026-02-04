// e2e/submit_test.go
package e2e_test

import (
	"strings"
	"testing"
)

func TestSubmitDryRun(t *testing.T) {
	env := NewTestEnvWithRemote(t)
	env.MustRun("init")

	env.MustRun("create", "feature-1")
	env.CreateCommit("feature 1 work")

	result := env.MustRun("submit", "--dry-run")

	// Should show all three phases
	if !strings.Contains(result.Stdout, "Phase 1: Cascade") {
		t.Error("expected cascade phase output")
	}
	if !strings.Contains(result.Stdout, "Phase 2: Push") {
		t.Error("expected push phase output")
	}
	if !strings.Contains(result.Stdout, "Phase 3: PRs") {
		t.Error("expected PR phase output")
	}

	// Should show "Would" for actions
	if !strings.Contains(result.Stdout, "Would") {
		t.Error("expected dry-run output with 'Would'")
	}

	// Branch should NOT be on remote in dry-run
	remoteBranches := env.GitRemote("branch")
	if strings.Contains(remoteBranches, "feature-1") {
		t.Error("feature-1 should not be on remote in dry-run")
	}
}

func TestSubmitDryRunStack(t *testing.T) {
	env := NewTestEnvWithRemote(t)
	env.MustRun("init")

	// Create stack: main -> feat-a -> feat-b
	env.MustRun("create", "feat-a")
	env.CreateCommit("a work")

	env.MustRun("create", "feat-b")
	env.CreateCommit("b work")

	// Go back to feat-a
	env.Git("checkout", "feat-a")

	// Submit from feat-a (should include feat-a and feat-b)
	result := env.MustRun("submit", "--dry-run")

	// Should mention both branches
	if !strings.Contains(result.Stdout, "feat-a") {
		t.Error("expected feat-a in output")
	}
	if !strings.Contains(result.Stdout, "feat-b") {
		t.Error("expected feat-b in output")
	}
}

func TestSubmitCurrentOnlyDryRun(t *testing.T) {
	env := NewTestEnvWithRemote(t)
	env.MustRun("init")

	env.MustRun("create", "feat-a")
	env.CreateCommit("a work")

	env.MustRun("create", "feat-b")
	env.CreateCommit("b work")

	env.Git("checkout", "feat-a")

	// Submit --current-only should NOT cascade feat-b
	result := env.MustRun("submit", "--dry-run", "--current-only")

	// Should mention feat-a but not feat-b in push phase
	output := result.Stdout
	if !strings.Contains(output, "Would push feat-a") {
		t.Error("expected feat-a push in output")
	}
	if strings.Contains(output, "Would push feat-b") {
		t.Error("feat-b should not be pushed with --current-only")
	}
}

func TestSubmitWithCascadeNeeded(t *testing.T) {
	env := NewTestEnvWithRemote(t)
	env.MustRun("init")

	// Create stack
	env.MustRun("create", "feat-a")
	env.CreateCommit("a work")

	env.MustRun("create", "feat-b")
	env.CreateCommit("b work")

	// Go back to feat-a and add commit (feat-b now needs rebase)
	env.Git("checkout", "feat-a")
	env.CreateCommit("more a work")

	// Submit dry-run should show rebase needed
	result := env.MustRun("submit", "--dry-run")

	// Should show rebase would happen
	if !strings.Contains(result.Stdout, "Would rebase feat-b onto feat-a") {
		t.Error("expected cascade of feat-b onto feat-a")
	}
}

func TestSubmitAlreadyUpToDate(t *testing.T) {
	env := NewTestEnvWithRemote(t)
	env.MustRun("init")

	env.MustRun("create", "feature-1")
	env.CreateCommit("feature 1 work")

	// Branch is already up to date with main (just created)
	result := env.MustRun("submit", "--dry-run")

	// Should indicate already up to date
	if !strings.Contains(result.Stdout, "already up to date") {
		t.Error("expected 'already up to date' for branch that doesn't need cascade")
	}
}

func TestSubmitUpdateOnlyDryRun(t *testing.T) {
	env := NewTestEnvWithRemote(t)
	env.MustRun("init")

	env.MustRun("create", "feature-1")
	env.CreateCommit("feature 1 work")

	// With --update-only, should skip PR creation for branches without PRs
	result := env.MustRun("submit", "--dry-run", "--update-only")

	// Should not show "Would create PR" since update-only skips creation
	// (In dry-run, it should show the skip message)
	if strings.Contains(result.Stdout, "Would create PR") {
		t.Error("should not create PR with --update-only")
	}
}

func TestSubmitDryRunAllowsDirtyWorkTree(t *testing.T) {
	env := NewTestEnvWithRemote(t)
	env.MustRun("init")

	env.MustRun("create", "feature-1")
	env.CreateCommit("feature-1 work")
	env.Git("push", "-u", "origin", "feature-1")

	// Create uncommitted changes
	env.WriteFile("dirty.txt", "uncommitted")
	env.Git("add", "dirty.txt")

	// Submit --dry-run should succeed even with dirty working tree
	// (dry-run skips the undo snapshot which includes auto-stashing)
	result := env.Run("submit", "--dry-run")

	if result.Failed() {
		t.Errorf("submit --dry-run should succeed with dirty tree, got: %s", result.Stderr)
	}
}

func TestSubmitRejectsUntrackedBranch(t *testing.T) {
	env := NewTestEnvWithRemote(t)
	env.MustRun("init")

	// Create an untracked branch
	env.Git("checkout", "-b", "untracked-branch")
	env.CreateCommit("work")

	// Submit should fail
	result := env.Run("submit", "--dry-run")

	if result.Success() {
		t.Error("expected submit to fail on untracked branch")
	}
	if !strings.Contains(result.Stderr, "not tracked") {
		t.Errorf("expected error about untracked branch, got: %s", result.Stderr)
	}
}

func TestSubmitFromTrunkWithDescendants(t *testing.T) {
	env := NewTestEnvWithRemote(t)
	env.MustRun("init")

	// Create a branch off trunk
	env.MustRun("create", "feature-1")
	env.CreateCommit("feature 1 work")

	// Go back to trunk
	env.Git("checkout", "main")

	// Submit from trunk should process only descendants, not trunk itself
	result := env.MustRun("submit", "--dry-run")

	// Should NOT mention pushing main
	if strings.Contains(result.Stdout, "Would push main") {
		t.Error("should not push trunk branch")
	}
	// Should NOT mention creating PR for main
	if strings.Contains(result.Stdout, "Would create PR for main") {
		t.Error("should not create PR for trunk branch")
	}
	// Should mention feature-1
	if !strings.Contains(result.Stdout, "Would push feature-1") {
		t.Error("expected feature-1 in push output")
	}
	if !strings.Contains(result.Stdout, "Would create PR for feature-1") {
		t.Error("expected PR creation for feature-1")
	}
}

func TestSubmitFromTrunkCurrentOnlyFails(t *testing.T) {
	env := NewTestEnvWithRemote(t)
	env.MustRun("init")

	// Create a branch so stack is not empty
	env.MustRun("create", "feature-1")
	env.CreateCommit("feature 1 work")

	// Go back to trunk
	env.Git("checkout", "main")

	// Submit --current-only from trunk should fail
	result := env.Run("submit", "--dry-run", "--current-only")

	if result.Success() {
		t.Error("expected submit --current-only from trunk to fail")
	}
	if !strings.Contains(result.Stderr, "cannot submit trunk") {
		t.Errorf("expected error about trunk, got: %s", result.Stderr)
	}
}

func TestSubmitFromTrunkNoDescendantsFails(t *testing.T) {
	env := NewTestEnvWithRemote(t)
	env.MustRun("init")

	// Stay on trunk with no stack branches
	// Submit should fail - nothing to do
	result := env.Run("submit", "--dry-run")

	if result.Success() {
		t.Error("expected submit from trunk with no descendants to fail")
	}
	if !strings.Contains(result.Stderr, "no stack branches") {
		t.Errorf("expected error about no stack branches, got: %s", result.Stderr)
	}
}
