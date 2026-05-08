// e2e/adopt_orphan_test.go
package e2e_test

import "testing"

func TestAdoptReparentsTrackedBranch(t *testing.T) {
	env := NewTestEnv(t)
	env.MustRun("init")

	// Create feat-a and feat-b both off trunk (main)
	env.MustRun("create", "feat-a")
	env.CreateCommit("feat-a work")

	env.Git("checkout", "main")
	env.MustRun("create", "feat-b")
	env.CreateCommit("feat-b work")

	// feat-b is currently tracked with parent main; reparent onto feat-a
	result := env.MustRun("adopt", "--branch", "feat-b", "feat-a")

	env.AssertStackParent("feat-b", "feat-a")
	if !result.ContainsStdout("feat-a") {
		t.Errorf("expected stdout to mention feat-a, got: %s", result.Stdout)
	}
	if !result.ContainsStdout("main") {
		t.Errorf("expected stdout to mention old parent main, got: %s", result.Stdout)
	}
}

func TestAdoptNoOpWhenParentUnchangedE2E(t *testing.T) {
	env := NewTestEnv(t)
	env.MustRun("init")

	env.Git("checkout", "-b", "existing-branch")
	env.CreateCommit("some work")
	env.MustRun("adopt", "main")

	// Adopt again with same parent: should succeed and print a warning
	result := env.Run("adopt", "main")
	if !result.Success() {
		t.Errorf("expected no-op adopt to succeed, got exit %d: %s", result.ExitCode, result.Stderr)
	}
	if !result.ContainsStdout("already tracked") {
		t.Errorf("expected 'already tracked' in stdout, got: %s", result.Stdout)
	}
}

func TestAdoptSelfParentRejected(t *testing.T) {
	env := NewTestEnv(t)
	env.MustRun("init")

	env.MustRun("create", "feat-a")
	env.CreateCommit("a work")

	result := env.Run("adopt", "--branch", "feat-a", "feat-a")
	if result.Success() {
		t.Error("expected self-parent adopt to fail, but it succeeded")
	}
	if !result.ContainsStderr("own parent") {
		t.Errorf("expected 'own parent' in stderr, got: %s", result.Stderr)
	}
}

func TestAdoptReparentCycleDetected(t *testing.T) {
	env := NewTestEnv(t)
	env.MustRun("init")

	// Build trunk → feat-a → feat-b
	env.MustRun("create", "feat-a")
	env.CreateCommit("a work")
	env.MustRun("create", "feat-b")
	env.CreateCommit("b work")

	// Attempting to reparent feat-a under feat-b would create a cycle
	result := env.Run("adopt", "--branch", "feat-a", "feat-b")
	if result.Success() {
		t.Error("expected cycle detection to fail, but adopt succeeded")
	}
	if !result.ContainsStderr("cycle") {
		t.Errorf("expected 'cycle' in stderr, got: %s", result.Stderr)
	}
}

func TestAdoptExistingBranch(t *testing.T) {
	env := NewTestEnv(t)
	env.MustRun("init")

	// Create branch outside gh-stack
	env.Git("checkout", "-b", "external-branch")
	env.CreateCommit("external work")

	// Adopt it (current branch with main as parent)
	env.MustRun("adopt", "main")

	env.AssertStackParent("external-branch", "main")
}

func TestOrphanBranch(t *testing.T) {
	env := NewTestEnv(t)
	env.MustRun("init")

	env.MustRun("create", "feature-1")
	env.CreateCommit("work")

	// Verify tracked
	env.AssertStackParent("feature-1", "main")

	// Orphan it
	env.MustRun("orphan", "feature-1")

	// Should no longer have parent
	parent := env.GetStackConfig("branch.feature-1.stackparent")
	if parent != "" {
		t.Errorf("expected no parent after orphan, got %q", parent)
	}
}

func TestOrphanMiddleOfStackRequiresForce(t *testing.T) {
	env := NewTestEnv(t)
	env.MustRun("init")

	env.MustRun("create", "feat-a")
	env.CreateCommit("a work")

	env.MustRun("create", "feat-b")
	env.CreateCommit("b work")

	env.MustRun("create", "feat-c")
	env.CreateCommit("c work")

	// Orphan middle branch without force should fail
	result := env.Run("orphan", "feat-b")
	if result.Success() {
		t.Error("orphan middle branch without --force should fail")
	}
	if !result.ContainsStderr("has children") {
		t.Errorf("expected error about children, got: %s", result.Stderr)
	}
}

func TestOrphanMiddleOfStackWithForce(t *testing.T) {
	env := NewTestEnv(t)
	env.MustRun("init")

	env.MustRun("create", "feat-a")
	env.CreateCommit("a work")

	env.MustRun("create", "feat-b")
	env.CreateCommit("b work")

	env.MustRun("create", "feat-c")
	env.CreateCommit("c work")

	// Orphan middle branch with --force
	env.MustRun("orphan", "feat-b", "--force")

	// feat-b should no longer have parent
	parent := env.GetStackConfig("branch.feat-b.stackparent")
	if parent != "" {
		t.Errorf("expected no parent for feat-b after orphan, got %q", parent)
	}

	// --force also orphans descendants, so feat-c should have no parent
	parentC := env.GetStackConfig("branch.feat-c.stackparent")
	if parentC != "" {
		t.Errorf("expected no parent for feat-c after orphan --force, got %q", parentC)
	}
}
