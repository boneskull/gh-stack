// e2e/push_test.go
package e2e_test

import (
	"strings"
	"testing"
)

func TestPushSingleBranch(t *testing.T) {
	env := NewTestEnvWithRemote(t)
	env.MustRun("init")

	env.MustRun("create", "feature-1")
	env.CreateCommit("feature 1 work")

	env.MustRun("push")

	// Verify branch on remote
	remoteBranches := env.GitRemote("branch")
	if !strings.Contains(remoteBranches, "feature-1") {
		t.Errorf("feature-1 not on remote: %s", remoteBranches)
	}
}

func TestPushStack(t *testing.T) {
	env := NewTestEnvWithRemote(t)
	env.MustRun("init")

	env.MustRun("create", "feat-a")
	env.CreateCommit("a work")

	env.MustRun("create", "feat-b")
	env.CreateCommit("b work")

	env.MustRun("push")

	remoteBranches := env.GitRemote("branch")
	if !strings.Contains(remoteBranches, "feat-a") ||
		!strings.Contains(remoteBranches, "feat-b") {
		t.Errorf("stack not fully pushed: %s", remoteBranches)
	}
}

func TestPushFailsWhenNotRebased(t *testing.T) {
	env := NewTestEnvWithRemote(t)
	env.MustRun("init")

	// Create stack: main -> feat-a -> feat-b
	env.MustRun("create", "feat-a")
	env.CreateCommit("a work")

	env.MustRun("create", "feat-b")
	env.CreateCommit("b work")

	// Go back to feat-a and add a new commit
	// This makes feat-b no longer rebased onto feat-a
	env.Git("checkout", "feat-a")
	env.CreateCommit("more a work")

	// Go to feat-b and try to push - should fail
	env.Git("checkout", "feat-b")
	result := env.Run("push")

	if result.Success() {
		t.Error("expected push to fail when branch is not rebased")
	}

	if !strings.Contains(result.Stderr, "not rebased onto") {
		t.Errorf("expected error about rebase, got: %s", result.Stderr)
	}

	if !strings.Contains(result.Stderr, "gh stack cascade") {
		t.Errorf("expected error to mention 'gh stack cascade', got: %s", result.Stderr)
	}
}
