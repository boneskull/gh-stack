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
