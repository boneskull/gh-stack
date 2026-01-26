// e2e/chaos_remote_test.go
package e2e_test

import "testing"

func TestRemoteTrunkAhead(t *testing.T) {
	env := NewTestEnvWithRemote(t)
	env.MustRun("init")

	env.MustRun("create", "feature-1")
	env.CreateCommit("feature work")
	env.MustRun("push")

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
	env.MustRun("push")

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

	// Push would need to recreate branch on remote (or fail)
	result = env.Run("push")
	// Just document behavior - may succeed or fail depending on implementation
	if result.Failed() {
		t.Logf("push after remote delete: %s", result.Stderr)
	}
}

func TestSomeoneElsePushedToMyBranch(t *testing.T) {
	env := NewTestEnvWithRemote(t)
	env.MustRun("init")

	env.MustRun("create", "feature-1")
	env.CreateCommit("my work")
	env.MustRun("push")

	// Someone else pushes to my branch (pair programming, CI, etc.)
	env.SimulateSomeoneElsePushed("feature-1")

	// I make more local changes
	env.CreateCommit("more of my work")

	// Push should fail - remote has diverged
	result := env.Run("push")
	if result.Success() {
		t.Error("push should fail when remote has diverged")
	}
}

func TestPushAfterCascade(t *testing.T) {
	env := NewTestEnvWithRemote(t)
	env.MustRun("init")

	env.MustRun("create", "feature-1")
	env.CreateCommit("feature 1 work")
	env.MustRun("push")

	// Move main forward
	env.Git("checkout", "main")
	env.CreateCommit("main moved")
	env.Git("push", "origin", "main")

	// Cascade rebases feature-1
	env.Git("checkout", "feature-1")
	env.MustRun("cascade")

	// Push after rebase needs force (history rewritten)
	result := env.Run("push")
	// gh-stack push should handle this (likely with --force-with-lease)
	if result.Failed() {
		// If it fails, it should give a clear error
		if !result.ContainsStderr("force") && !result.ContainsStderr("reject") {
			t.Logf("push after cascade failed: %s", result.Stderr)
		}
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
