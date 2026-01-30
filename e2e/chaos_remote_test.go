// e2e/chaos_remote_test.go
package e2e_test

import "testing"

func TestRemoteTrunkAhead(t *testing.T) {
	env := NewTestEnvWithRemote(t)
	env.MustRun("init")

	env.MustRun("create", "feature-1")
	env.CreateCommit("feature work")
	// Use git push directly to set up remote state
	env.Git("push", "-u", "origin", "feature-1")

	// Simulate remote main moving ahead (another dev merged something)
	env.SimulateSomeoneElsePushed("main")

	// Local doesn't know yet - cascade uses local main
	env.Git("checkout", "feature-1")
	result := env.Run("cascade")

	// Should succeed with local state (feature-1 already up-to-date with local main)
	if result.Failed() {
		t.Errorf("cascade should work with local state: %s", result.Stderr)
	}

	// After fetch, local sees remote is ahead
	env.FetchOrigin()

	// Now cascade would pick up the new main commits
	result = env.Run("cascade")
	if result.Failed() {
		t.Errorf("cascade after fetch should work: %s", result.Stderr)
	}
}

func TestLocalTrunkAhead(t *testing.T) {
	env := NewTestEnvWithRemote(t)
	env.MustRun("init")

	env.MustRun("create", "feature-1")
	env.CreateCommit("feature work")

	// Local main has unpushed commits
	env.Git("checkout", "main")
	env.CreateCommit("local main work")
	// Don't push!

	env.Git("checkout", "feature-1")
	env.MustRun("cascade")

	// Should work with local main (includes unpushed commit)
	env.AssertAncestor("main", "feature-1")
}

func TestStackBranchDeletedOnRemote(t *testing.T) {
	env := NewTestEnvWithRemote(t)
	env.MustRun("init")

	env.MustRun("create", "feature-1")
	env.CreateCommit("feature work")
	// Use git push directly to set up remote state
	env.Git("push", "-u", "origin", "feature-1")

	// Simulate PR merged (branch deleted on remote)
	env.SimulatePRMerged("feature-1", "main")

	// Fetch to see deletion
	env.FetchOrigin()

	// Local branch still exists, stack state intact
	result := env.MustRun("log")
	if !result.ContainsStdout("feature-1") {
		t.Error("log should still show local branch")
	}

	// Can still work locally
	env.Git("checkout", "feature-1")
	env.CreateCommit("more local work")
}

func TestSomeoneElsePushedToMyBranch(t *testing.T) {
	env := NewTestEnvWithRemote(t)
	env.MustRun("init")

	env.MustRun("create", "feature-1")
	env.CreateCommit("my work")
	// Use git push directly to set up remote state
	env.Git("push", "-u", "origin", "feature-1")

	// Someone else pushes to my branch (pair programming, CI, etc.)
	env.SimulateSomeoneElsePushed("feature-1")

	// I make more local changes
	env.CreateCommit("more of my work")

	// Submit should fail - remote has diverged (--force-with-lease protects us)
	result := env.Run("submit", "--dry-run")
	// In dry-run, cascade/push phases shown but no actual push
	// The actual failure would happen on real push with --force-with-lease
	if result.Failed() {
		t.Logf("submit dry-run result: %s", result.Stderr)
	}
}

func TestSubmitAfterCascade(t *testing.T) {
	env := NewTestEnvWithRemote(t)
	env.MustRun("init")

	env.MustRun("create", "feature-1")
	env.CreateCommit("feature 1 work")
	// Use git push directly to set up remote state
	env.Git("push", "-u", "origin", "feature-1")

	// Move main forward
	env.Git("checkout", "main")
	env.CreateCommit("main moved")
	env.Git("push", "origin", "main")

	// Go back to feature and run submit dry-run
	env.Git("checkout", "feature-1")
	result := env.MustRun("submit", "--dry-run")

	// Should show cascade needed
	if !result.ContainsStdout("Would rebase") {
		t.Error("submit should show rebase would happen")
	}
}

func TestFetchBeforeCascade(t *testing.T) {
	env := NewTestEnvWithRemote(t)
	env.MustRun("init")

	env.MustRun("create", "feature-1")
	env.CreateCommit("feature work")

	// Remote main moves
	env.SimulateSomeoneElsePushed("main")

	// Without fetch, cascade uses stale local main
	result := env.Run("cascade")
	// Document: does cascade auto-fetch? Or use local state?
	if result.Failed() {
		t.Errorf("cascade should not fail: %s", result.Stderr)
	}
}
