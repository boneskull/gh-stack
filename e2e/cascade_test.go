// e2e/cascade_test.go
package e2e_test

import "testing"

func TestCascadeSimple(t *testing.T) {
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

	// Cascade from feature-a
	env.Git("checkout", "feature-a")
	env.MustRun("cascade")

	// Verify ancestry
	env.AssertAncestor("main", "feature-a")
	env.AssertAncestor("feature-a", "feature-b")
	env.AssertNoRebaseInProgress()
}

func TestCascadeDeepStack(t *testing.T) {
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

	// Cascade from first branch
	env.Git("checkout", "feat-1")
	env.MustRun("cascade")

	// Verify chain
	env.AssertAncestor("main", "feat-1")
	for i := 1; i < len(branches); i++ {
		env.AssertAncestor(branches[i-1], branches[i])
	}
}

func TestCascadeWithConflict(t *testing.T) {
	env := NewTestEnv(t)
	env.MustRun("init")

	_ = env.CreateStackWithConflict()

	result := env.Run("cascade")

	// Cascade returns non-zero on conflict (consistent with git rebase)
	if result.Success() {
		t.Error("cascade should return non-zero exit on conflict")
	}
	if !result.ContainsStdout("CONFLICT") {
		t.Errorf("expected CONFLICT in output, got: %s", result.Stdout)
	}
	env.AssertRebaseInProgress()
}

func TestCascadeAbort(t *testing.T) {
	env := NewTestEnv(t)
	env.MustRun("init")

	env.CreateStackWithConflict()
	result := env.Run("cascade")

	if result.Success() {
		t.Fatal("expected cascade to fail on conflict")
	}
	env.AssertRebaseInProgress()

	env.MustRun("abort")

	env.AssertNoRebaseInProgress()
}

func TestCascadeContinue(t *testing.T) {
	env := NewTestEnv(t)
	env.MustRun("init")

	conflictFile := env.CreateStackWithConflict()
	result := env.Run("cascade")

	if result.Success() {
		t.Fatal("expected cascade to fail on conflict")
	}
	env.AssertRebaseInProgress()

	// Resolve and continue
	env.ResolveConflict(conflictFile)
	env.MustRun("continue")

	env.AssertNoRebaseInProgress()
}
