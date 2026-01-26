// e2e/chaos_manual_git_test.go
package e2e_test

import "testing"

func TestManualGitCommit(t *testing.T) {
	env := NewTestEnv(t)
	env.MustRun("init")
	env.MustRun("create", "feature-1")

	// User commits directly with git instead of through workflow
	env.WriteFile("manual.txt", "manual change")
	env.Git("add", "manual.txt")
	env.Git("commit", "-m", "manual commit")

	// Stack should still work
	result := env.MustRun("log")
	if !result.ContainsStdout("feature-1") {
		t.Error("log should still show branch")
	}

	// Can still create child branches
	env.MustRun("create", "feature-2")
	env.AssertStackParent("feature-2", "feature-1")
}

func TestManualBranchCreate(t *testing.T) {
	env := NewTestEnv(t)
	env.MustRun("init")

	// User creates branch with git, not gh-stack
	env.Git("checkout", "-b", "manual-branch")
	env.CreateCommit("manual work")

	// Branch not tracked initially
	parent := env.GetStackConfig("branch.manual-branch.stackparent")
	if parent != "" {
		t.Errorf("manual branch should not be tracked, got parent %q", parent)
	}

	// Can adopt it into the stack
	env.MustRun("adopt", "manual-branch", "--parent", "main")
	env.AssertStackParent("manual-branch", "main")

	// Now it shows in log
	result := env.MustRun("log")
	if !result.ContainsStdout("manual-branch") {
		t.Error("adopted branch should appear in log")
	}
}

func TestManualRebase(t *testing.T) {
	env := NewTestEnv(t)
	env.MustRun("init")

	env.MustRun("create", "feature-1")
	env.CreateCommit("feature 1 work")
	feature1OldTip := env.BranchTip("feature-1")

	env.Git("checkout", "main")
	env.CreateCommit("main moved")

	// User rebases manually with git instead of cascade
	env.Git("checkout", "feature-1")
	env.Git("rebase", "main")

	// Verify branch moved (tip changed due to rebase)
	feature1NewTip := env.BranchTip("feature-1")
	if feature1NewTip == feature1OldTip {
		t.Error("tip should have changed after rebase")
	}

	// Stack commands should still work
	result := env.MustRun("log")
	if !result.ContainsStdout("feature-1") {
		t.Error("log should still show branch after manual rebase")
	}

	// Stack parent relationship preserved
	env.AssertStackParent("feature-1", "main")
}

func TestManualBranchDelete(t *testing.T) {
	env := NewTestEnv(t)
	env.MustRun("init")

	env.MustRun("create", "feature-1")
	env.CreateCommit("work")

	env.MustRun("create", "feature-2")
	env.CreateCommit("more work")

	// User deletes middle branch with git
	env.Git("checkout", "main")
	env.Git("branch", "-D", "feature-1")

	// Log handles missing branch gracefully - shows only valid branches
	result := env.Run("log")
	if result.Failed() {
		t.Errorf("log should succeed even with deleted branch, got: %s", result.Stderr)
	}
	// feature-2 becomes orphaned (parent gone), so only main shows
	if !result.ContainsStdout("main") {
		t.Error("log should still show main")
	}
}

func TestManualRebaseAbortDuringCascade(t *testing.T) {
	env := NewTestEnv(t)
	env.MustRun("init")

	// Set up conflict scenario
	env.CreateStackWithConflict()
	env.Run("cascade") // Will conflict and leave rebase in progress

	env.AssertRebaseInProgress()

	// User manually aborts with git instead of gh-stack abort
	env.Git("rebase", "--abort")

	// gh-stack abort should still work (cleans up cascade state)
	result := env.Run("abort")
	if result.Failed() {
		t.Errorf("abort should succeed after manual rebase --abort, got: %s", result.Stderr)
	}
}
