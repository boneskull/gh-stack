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
	if !strings.Contains(result.Stdout, "Phase 1: Restack") {
		t.Error("expected restack phase output")
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

	// Submit --current should NOT restack feat-b
	result := env.MustRun("submit", "--dry-run", "--current")

	// Should mention feat-a but not feat-b in push phase
	output := result.Stdout
	if !strings.Contains(output, "Would push feat-a") {
		t.Error("expected feat-a push in output")
	}
	if strings.Contains(output, "Would push feat-b") {
		t.Error("feat-b should not be pushed with --current")
	}
}

func TestSubmitWithRestackNeeded(t *testing.T) {
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
		t.Error("expected restack of feat-b onto feat-a")
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
		t.Error("expected 'already up to date' for branch that doesn't need restack")
	}
}

func TestSubmitUpdateOnlyDryRun(t *testing.T) {
	env := NewTestEnvWithRemote(t)
	env.MustRun("init")

	env.MustRun("create", "feature-1")
	env.CreateCommit("feature 1 work")

	// With --update, should skip PR creation for branches without PRs
	result := env.MustRun("submit", "--dry-run", "--update")

	// Should not show "Would create PR" since update skips creation
	// (In dry-run, it should show the skip message)
	if strings.Contains(result.Stdout, "Would create PR") {
		t.Error("should not create PR with --update")
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

	// Default submit (entire stack) should fail because there are no stack branches
	result := env.Run("submit", "--dry-run")

	if result.Success() {
		t.Error("expected submit to fail with no stack branches")
	}
	if !strings.Contains(result.Stderr, "no stack branches") {
		t.Errorf("expected error about no stack branches, got: %s", result.Stderr)
	}
}

func TestSubmitFromRejectsUntrackedBranch(t *testing.T) {
	env := NewTestEnvWithRemote(t)
	env.MustRun("init")

	// Create an untracked branch
	env.Git("checkout", "-b", "untracked-branch")
	env.CreateCommit("work")

	// Bare --from on untracked branch should report it's not tracked
	result := env.Run("submit", "--dry-run", "--from")

	if result.Success() {
		t.Error("expected submit --from to fail on untracked branch")
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

	// Submit --current from trunk should fail
	result := env.Run("submit", "--dry-run", "--current")

	if result.Success() {
		t.Error("expected submit --current from trunk to fail")
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

func TestSubmitPushOnlyDryRun(t *testing.T) {
	env := NewTestEnvWithRemote(t)
	env.MustRun("init")

	env.MustRun("create", "feature-1")
	env.CreateCommit("feature 1 work")

	result := env.MustRun("submit", "--dry-run", "--skip-prs")

	// Should show all three phases
	if !strings.Contains(result.Stdout, "Phase 1: Restack") {
		t.Error("expected restack phase output")
	}
	if !strings.Contains(result.Stdout, "Phase 2: Push") {
		t.Error("expected push phase output")
	}
	if !strings.Contains(result.Stdout, "Phase 3: PRs") {
		t.Error("expected PR phase header")
	}

	// Should show skipped message for PR phase
	if !strings.Contains(result.Stdout, "Skipped (--skip-prs)") {
		t.Error("expected 'Skipped (--skip-prs)' message")
	}

	// Should NOT mention PR creation
	if strings.Contains(result.Stdout, "Would create PR") {
		t.Error("should not mention PR creation with --skip-prs")
	}
}

func TestSubmitPushOnlyWithUpdateOnlyFails(t *testing.T) {
	env := NewTestEnvWithRemote(t)
	env.MustRun("init")

	env.MustRun("create", "feature-1")
	env.CreateCommit("feature 1 work")

	result := env.Run("submit", "--dry-run", "--skip-prs", "--update")

	if result.Success() {
		t.Error("expected --skip-prs and --update to fail")
	}
	if !strings.Contains(result.Stderr, "--skip-prs and --update cannot be used together") {
		t.Errorf("expected error about conflicting flags, got: %s", result.Stderr)
	}
}

func TestSubmitPushOnlyWithWebFails(t *testing.T) {
	env := NewTestEnvWithRemote(t)
	env.MustRun("init")

	env.MustRun("create", "feature-1")
	env.CreateCommit("feature 1 work")

	result := env.Run("submit", "--dry-run", "--skip-prs", "--web")

	if result.Success() {
		t.Error("expected --skip-prs and --web to fail")
	}
	if !strings.Contains(result.Stderr, "--skip-prs and --web cannot be used together") {
		t.Errorf("expected error about conflicting flags, got: %s", result.Stderr)
	}
}

func TestSubmitEntireStackDryRun(t *testing.T) {
	env := NewTestEnvWithRemote(t)
	env.MustRun("init")

	// Create 3-level stack: main -> feat-a -> feat-b -> feat-c
	env.MustRun("create", "feat-a")
	env.CreateCommit("a work")

	env.MustRun("create", "feat-b")
	env.CreateCommit("b work")

	env.MustRun("create", "feat-c")
	env.CreateCommit("c work")

	// Checkout middle branch
	env.Git("checkout", "feat-b")

	// Submit without --from should process the entire stack
	result := env.MustRun("submit", "--dry-run")

	output := result.Stdout
	if !strings.Contains(output, "Would push feat-a") {
		t.Error("expected feat-a (ancestor) in push output")
	}
	if !strings.Contains(output, "Would push feat-b") {
		t.Error("expected feat-b in push output")
	}
	if !strings.Contains(output, "Would push feat-c") {
		t.Error("expected feat-c (descendant) in push output")
	}
}

func TestSubmitFromFlagDryRun(t *testing.T) {
	env := NewTestEnvWithRemote(t)
	env.MustRun("init")

	// Create 3-level stack: main -> feat-a -> feat-b -> feat-c
	env.MustRun("create", "feat-a")
	env.CreateCommit("a work")

	env.MustRun("create", "feat-b")
	env.CreateCommit("b work")

	env.MustRun("create", "feat-c")
	env.CreateCommit("c work")

	// Stay on feat-c, but use --from=feat-a
	result := env.MustRun("submit", "--dry-run", "--from=feat-a")

	output := result.Stdout
	if !strings.Contains(output, "Would push feat-a") {
		t.Error("expected feat-a in push output")
	}
	if !strings.Contains(output, "Would push feat-b") {
		t.Error("expected feat-b in push output")
	}
	if !strings.Contains(output, "Would push feat-c") {
		t.Error("expected feat-c in push output")
	}
}

func TestSubmitFromFlagCurrentBranchDryRun(t *testing.T) {
	env := NewTestEnvWithRemote(t)
	env.MustRun("init")

	// Create 3-level stack: main -> feat-a -> feat-b -> feat-c
	env.MustRun("create", "feat-a")
	env.CreateCommit("a work")

	env.MustRun("create", "feat-b")
	env.CreateCommit("b work")

	env.MustRun("create", "feat-c")
	env.CreateCommit("c work")

	// Checkout middle branch
	env.Git("checkout", "feat-b")

	// Bare --from should use current branch (feat-b) + descendants only
	result := env.MustRun("submit", "--dry-run", "--from")

	output := result.Stdout
	if strings.Contains(output, "Would push feat-a") {
		t.Error("feat-a should NOT be in output with bare --from on feat-b")
	}
	if !strings.Contains(output, "Would push feat-b") {
		t.Error("expected feat-b in push output")
	}
	if !strings.Contains(output, "Would push feat-c") {
		t.Error("expected feat-c in push output")
	}
}

func TestSubmitFromFlagUntrackedBranch(t *testing.T) {
	env := NewTestEnvWithRemote(t)
	env.MustRun("init")

	env.MustRun("create", "feat-a")
	env.CreateCommit("a work")

	result := env.Run("submit", "--dry-run", "--from=nonexistent")

	if result.Success() {
		t.Error("expected --from=nonexistent to fail")
	}
	if !strings.Contains(result.Stderr, "not tracked") {
		t.Errorf("expected error about untracked branch, got: %s", result.Stderr)
	}
}

func TestSubmitFromAndCurrentOnlyMutualExclusion(t *testing.T) {
	env := NewTestEnvWithRemote(t)
	env.MustRun("init")

	env.MustRun("create", "feat-a")
	env.CreateCommit("a work")

	result := env.Run("submit", "--dry-run", "--from=feat-a", "--current")

	if result.Success() {
		t.Error("expected --from and --current to fail")
	}
	if !strings.Contains(result.Stderr, "--from and --current cannot be used together") {
		t.Errorf("expected error about conflicting flags, got: %s", result.Stderr)
	}
}

func TestSubmitFromTrunkFallback(t *testing.T) {
	env := NewTestEnvWithRemote(t)
	env.MustRun("init")

	// Create stack: main -> feat-a -> feat-b
	env.MustRun("create", "feat-a")
	env.CreateCommit("a work")

	env.MustRun("create", "feat-b")
	env.CreateCommit("b work")

	// --from=main should behave like default (entire stack)
	result := env.MustRun("submit", "--dry-run", "--from=main")

	output := result.Stdout
	if !strings.Contains(output, "Would push feat-a") {
		t.Error("expected feat-a in push output")
	}
	if !strings.Contains(output, "Would push feat-b") {
		t.Error("expected feat-b in push output")
	}
	if strings.Contains(output, "Would push main") {
		t.Error("should not push trunk branch")
	}
}

// TestSubmitBatchPush verifies that a multi-branch submit advances all remote
// refs in one atomic push and emits the expected summary line.
func TestSubmitBatchPush(t *testing.T) {
	env := NewTestEnvWithRemote(t)
	env.MustRun("init")

	// Build a 3-branch stack: main -> feat-a -> feat-b -> feat-c
	env.MustRun("create", "feat-a")
	tipA := env.CreateCommit("a work")

	env.MustRun("create", "feat-b")
	tipB := env.CreateCommit("b work")

	env.MustRun("create", "feat-c")
	tipC := env.CreateCommit("c work")

	result := env.MustRun("submit", "--skip-prs", "--yes")

	// Output should mention the batch push summary
	if !strings.Contains(result.Stdout, "Pushing") {
		t.Error("expected batch push summary line in output")
	}
	if !strings.Contains(result.Stdout, "atomic") {
		t.Error("expected 'atomic' in push summary line")
	}

	// All three remote refs must match local tips
	remoteA := env.GitRemote("rev-parse", "refs/heads/feat-a")
	if remoteA != tipA {
		t.Errorf("remote feat-a = %s, want %s", remoteA, tipA)
	}
	remoteB := env.GitRemote("rev-parse", "refs/heads/feat-b")
	if remoteB != tipB {
		t.Errorf("remote feat-b = %s, want %s", remoteB, tipB)
	}
	remoteC := env.GitRemote("rev-parse", "refs/heads/feat-c")
	if remoteC != tipC {
		t.Errorf("remote feat-c = %s, want %s", remoteC, tipC)
	}
}
