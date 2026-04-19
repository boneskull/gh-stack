// e2e/unlink_test.go
package e2e_test

import "testing"

func TestUnlinkClearsPRBase(t *testing.T) {
	env := NewTestEnv(t)
	env.MustRun("init")

	env.MustRun("create", "feature-a")
	env.CreateCommit("feature a work")

	// Simulate a prior submit run that stored a PR base.
	env.Git("config", "branch.feature-a.stackPR", "42")
	env.Git("config", "branch.feature-a.stackPRBase", "main")

	// Verify both keys are present before unlink.
	if env.GetStackConfig("branch.feature-a.stackPR") != "42" {
		t.Fatal("stackPR should be set before unlink")
	}
	if env.GetStackConfig("branch.feature-a.stackPRBase") != "main" {
		t.Fatal("stackPRBase should be set before unlink")
	}

	env.Git("checkout", "feature-a")
	env.MustRun("unlink")

	// Both PR-related keys should be cleared.
	if v := env.GetStackConfig("branch.feature-a.stackPR"); v != "" {
		t.Errorf("expected stackPR to be removed after unlink, got %q", v)
	}
	if v := env.GetStackConfig("branch.feature-a.stackPRBase"); v != "" {
		t.Errorf("expected stackPRBase to be removed after unlink, got %q", v)
	}
}
