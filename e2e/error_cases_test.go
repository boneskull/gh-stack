// e2e/error_cases_test.go
package e2e_test

import "testing"

func TestCreateWithUntrackedFile(t *testing.T) {
	env := NewTestEnv(t)
	env.MustRun("init")

	// Untracked files shouldn't block create
	env.WriteFile("uncommitted.txt", "dirty")

	result := env.Run("create", "feature-1")

	if result.Failed() {
		t.Errorf("create should succeed with untracked files, got: %s", result.Stderr)
	}
	env.AssertBranch("feature-1")
}

func TestCreateWithModifiedTrackedFile(t *testing.T) {
	env := NewTestEnv(t)
	env.MustRun("init")

	env.WriteFile("tracked.txt", "original")
	env.Git("add", "tracked.txt")
	env.Git("commit", "-m", "add tracked file")

	// Modify tracked file without staging
	env.WriteFile("tracked.txt", "modified")

	result := env.Run("create", "feature-1")

	// Create allows this (just creates branch, doesn't rebase)
	if result.Failed() {
		t.Errorf("create should succeed with modified files, got: %s", result.Stderr)
	}
	env.AssertBranch("feature-1")
}

func TestCascadeWithDirtyTree(t *testing.T) {
	env := NewTestEnv(t)
	env.MustRun("init")

	env.MustRun("create", "feature-1")
	env.CreateCommit("work")

	env.Git("checkout", "main")
	env.CreateCommit("main update")
	env.Git("checkout", "feature-1")

	// Make dirty with modified tracked file
	env.WriteFile("README.md", "modified content")

	result := env.Run("cascade")

	// Should auto-stash and succeed
	if result.Failed() {
		t.Errorf("cascade should auto-stash and succeed, got: %s", result.Stderr)
	}
	if !result.ContainsStdout("Auto-stashed") {
		t.Errorf("expected auto-stash message, got: %s", result.Stdout)
	}
}
