// e2e/adopt_orphan_test.go
package e2e_test

import "testing"

func TestAdoptExistingBranch(t *testing.T) {
	env := NewTestEnv(t)
	env.MustRun("init")

	// Create branch outside gh-stack
	env.Git("checkout", "-b", "external-branch")
	env.CreateCommit("external work")

	// Adopt it with explicit parent
	env.MustRun("adopt", "external-branch", "--parent", "main")

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
