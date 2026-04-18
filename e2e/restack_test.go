// e2e/restack_test.go
package e2e_test

import "testing"

func TestRestackSimple(t *testing.T) {
	env := NewTestEnv(t)
	env.MustRun("init")

	// Create stack
	env.MustRun("create", "feature-a")
	env.CreateCommit("feature a work")

	env.MustRun("create", "feature-b")
	env.CreateCommit("feature b work")

	// Add commit to main
	env.Git("checkout", "main")
	env.CreateCommit("main moved forward")

	// Restack from feature-a
	env.Git("checkout", "feature-a")
	env.MustRun("restack")

	// Verify ancestry
	env.AssertAncestor("main", "feature-a")
	env.AssertAncestor("feature-a", "feature-b")
	env.AssertNoRebaseInProgress()
}

func TestRestackDeepStack(t *testing.T) {
	env := NewTestEnv(t)
	env.MustRun("init")

	// Create 5-branch stack
	branches := []string{"feat-1", "feat-2", "feat-3", "feat-4", "feat-5"}
	for _, b := range branches {
		env.MustRun("create", b)
		env.CreateCommit(b + " work")
	}

	// Move main forward
	env.Git("checkout", "main")
	env.CreateCommit("main update")

	// Restack from first branch
	env.Git("checkout", "feat-1")
	env.MustRun("restack")

	// Verify chain
	env.AssertAncestor("main", "feat-1")
	for i := 1; i < len(branches); i++ {
		env.AssertAncestor(branches[i-1], branches[i])
	}
}

func TestRestackWithConflict(t *testing.T) {
	env := NewTestEnv(t)
	env.MustRun("init")

	_ = env.CreateStackWithConflict()

	result := env.Run("restack")

	// Restack returns non-zero on conflict (consistent with git rebase)
	if result.Success() {
		t.Error("restack should return non-zero exit on conflict")
	}
	if !result.ContainsStdout("CONFLICT") {
		t.Errorf("expected CONFLICT in output, got: %s", result.Stdout)
	}
	env.AssertRebaseInProgress()
}

func TestRestackAbort(t *testing.T) {
	env := NewTestEnv(t)
	env.MustRun("init")

	env.CreateStackWithConflict()
	result := env.Run("restack")

	if result.Success() {
		t.Fatal("expected restack to fail on conflict")
	}
	env.AssertRebaseInProgress()

	env.MustRun("abort")

	env.AssertNoRebaseInProgress()
}

func TestRestackContinue(t *testing.T) {
	env := NewTestEnv(t)
	env.MustRun("init")

	conflictFile := env.CreateStackWithConflict()
	result := env.Run("restack")

	if result.Success() {
		t.Fatal("expected restack to fail on conflict")
	}
	env.AssertRebaseInProgress()

	// Resolve and continue
	env.ResolveConflict(conflictFile)
	env.MustRun("continue")

	env.AssertNoRebaseInProgress()
}

func TestRestackReturnsToOriginalBranch(t *testing.T) {
	env := NewTestEnv(t)
	env.MustRun("init")

	// Create a 3-branch stack
	env.MustRun("create", "feature-a")
	env.CreateCommit("feature a work")

	env.MustRun("create", "feature-b")
	env.CreateCommit("feature b work")

	env.MustRun("create", "feature-c")
	env.CreateCommit("feature c work")

	// Move main forward
	env.Git("checkout", "main")
	env.CreateCommit("main moved forward")

	// Start restack from feature-a (not the deepest branch)
	env.Git("checkout", "feature-a")
	env.AssertBranch("feature-a")

	env.MustRun("restack")

	// Verify we returned to feature-a, not feature-c (the last restacked branch)
	env.AssertBranch("feature-a")

	// Verify all branches were restacked
	env.AssertAncestor("main", "feature-a")
	env.AssertAncestor("feature-a", "feature-b")
	env.AssertAncestor("feature-b", "feature-c")
}

func TestRestackStaleForkPointFromManualRebase(t *testing.T) {
	// Reproduces the bug where a manual rebase outside gh-stack leaves the
	// fork point stale. On the next restack after main advances, the stale
	// fork point would trigger an --onto rebase that replays too many commits.
	env := NewTestEnv(t)
	env.MustRun("init")

	env.MustRun("create", "feature-a")
	env.CreateCommit("feature a work")

	// Advance main
	env.Git("checkout", "main")
	env.CreateCommit("main update 1")

	// Manually rebase (outside gh-stack) -- fork point stays stale
	env.Git("checkout", "feature-a")
	env.Git("rebase", "main")

	// Run restack while already up-to-date so fork point gets refreshed
	env.MustRun("restack")

	// Advance main again
	env.Git("checkout", "main")
	env.CreateCommit("main update 2")

	// This restack should use a simple rebase (not --onto with stale fork point)
	env.Git("checkout", "feature-a")
	result := env.MustRun("restack")

	// Should NOT say "using fork point" -- that would mean the stale fork
	// point incorrectly triggered the --onto path
	if result.ContainsStdout("using fork point") {
		t.Error("restack should use simple rebase, not --onto with stale fork point")
	}

	env.AssertAncestor("main", "feature-a")
	env.AssertNoRebaseInProgress()
}

func TestRestackStaleForkPointDetectedDuringRebase(t *testing.T) {
	// Even if the "already up to date" refresh was missed, the ancestor check
	// in the useOnto logic should prevent --onto with a stale fork point.
	env := NewTestEnv(t)
	env.MustRun("init")

	env.MustRun("create", "feature-a")
	env.CreateCommit("feature a work")

	// Record fork point before any manipulation
	originalFP := env.GetStackConfig("branch.feature-a.stackforkpoint")
	if originalFP == "" {
		t.Fatal("expected fork point to be set after create")
	}

	// Advance main
	env.Git("checkout", "main")
	env.CreateCommit("main advance 1")
	env.CreateCommit("main advance 2")

	// Manually rebase feature-a onto current main
	env.Git("checkout", "feature-a")
	env.Git("rebase", "main")

	// Fork point is still the old value (stale)
	fpAfterManual := env.GetStackConfig("branch.feature-a.stackforkpoint")
	if fpAfterManual != originalFP {
		t.Fatal("expected fork point to still be the original (stale) value")
	}

	// Advance main further
	env.Git("checkout", "main")
	env.CreateCommit("main advance 3")

	// Restack from feature-a -- this triggers NeedsRebase=true with a stale
	// fork point that differs from merge-base. The fix should detect the stale
	// fork point is an ancestor of the merge-base and use a simple rebase.
	env.Git("checkout", "feature-a")
	result := env.MustRun("restack")

	if result.ContainsStdout("using fork point") {
		t.Error("restack should NOT use --onto with a stale fork point that is an ancestor of merge-base")
	}

	env.AssertAncestor("main", "feature-a")
	env.AssertNoRebaseInProgress()
}

func TestRestackForkPointUpdatedAfterContinue(t *testing.T) {
	env := NewTestEnv(t)
	env.MustRun("init")

	// Create a stack that will conflict
	conflictFile := env.CreateStackWithConflict()

	// Restack will hit a conflict on feature-b
	result := env.Run("restack")
	if result.Success() {
		t.Fatal("expected conflict")
	}

	// Resolve and continue
	env.ResolveConflict(conflictFile)
	env.MustRun("continue")

	// After continue, fork point for feature-a should be updated to main's tip.
	// (feature-a was the one that got rebased before the conflict on feature-b.)
	mainTip := env.BranchTip("main")
	featureAFP := env.GetStackConfig("branch.feature-a.stackforkpoint")
	if featureAFP != mainTip {
		t.Errorf("feature-a fork point after continue = %s, want main tip %s", featureAFP[:7], mainTip[:7])
	}

	// feature-b should also have its fork point updated (to feature-a's tip)
	featureATip := env.BranchTip("feature-a")
	featureBFP := env.GetStackConfig("branch.feature-b.stackforkpoint")
	if featureBFP != featureATip {
		t.Errorf("feature-b fork point after continue = %s, want feature-a tip %s", featureBFP[:7], featureATip[:7])
	}
}

func TestRestackOntoUsedForRewrittenParent(t *testing.T) {
	// Verify that --onto IS used when the parent's history was actually
	// rewritten (not just a stale fork point).
	env := NewTestEnv(t)
	env.MustRun("init")

	env.MustRun("create", "feature-a")
	env.CreateCommit("feature a work")

	env.MustRun("create", "feature-b")
	env.CreateCommit("feature b work")

	// Amend feature-a's commit (rewrites its history)
	env.Git("checkout", "feature-a")
	env.WriteFile("feature-a-amended.txt", "amended content")
	env.Git("add", ".")
	env.Git("commit", "--amend", "--no-edit")

	// Restack from feature-a. feature-b's fork point (old feature-a tip)
	// is now on a different history line → --onto should be used.
	result := env.MustRun("restack")

	if !result.ContainsStdout("using fork point") {
		t.Error("restack should use --onto when parent history was rewritten")
	}

	env.AssertAncestor("feature-a", "feature-b")
	env.AssertNoRebaseInProgress()
}
