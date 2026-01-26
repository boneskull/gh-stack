// e2e/init_create_test.go
package e2e_test

import "testing"

func TestInitBasic(t *testing.T) {
	env := NewTestEnv(t)

	result := env.MustRun("init")

	if !result.ContainsStdout("main") {
		t.Errorf("should mention trunk branch")
	}

	// Verify config was set
	trunk := env.GetStackConfig("stack.trunk")
	if trunk != "main" {
		t.Errorf("expected trunk 'main', got %q", trunk)
	}
}

func TestInitIdempotent(t *testing.T) {
	env := NewTestEnv(t)

	env.MustRun("init")
	result := env.Run("init")

	// Second init should succeed (idempotent)
	if result.Failed() {
		t.Errorf("second init should succeed, got exit=%d stderr=%s", result.ExitCode, result.Stderr)
	}
}

func TestCreateBranch(t *testing.T) {
	env := NewTestEnv(t)
	env.MustRun("init")

	env.MustRun("create", "feature-1")

	env.AssertBranch("feature-1")
	env.AssertStackParent("feature-1", "main")
}

func TestCreateStackedBranches(t *testing.T) {
	env := NewTestEnv(t)
	env.MustRun("init")

	env.MustRun("create", "feature-a")
	env.CreateCommit("feature a work")

	env.MustRun("create", "feature-b")
	env.CreateCommit("feature b work")

	env.MustRun("create", "feature-c")
	env.CreateCommit("feature c work")

	env.AssertStackParent("feature-a", "main")
	env.AssertStackParent("feature-b", "feature-a")
	env.AssertStackParent("feature-c", "feature-b")

	result := env.MustRun("log")
	if !result.ContainsStdout("feature-a") ||
		!result.ContainsStdout("feature-b") ||
		!result.ContainsStdout("feature-c") {
		t.Errorf("log missing branches: %s", result.Stdout)
	}
}
