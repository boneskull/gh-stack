// e2e/orphan_test.go
package e2e_test

import "testing"

func TestOrphanClearsPRBase(t *testing.T) {
	env := NewTestEnv(t)
	env.MustRun("init")

	env.MustRun("create", "feature-a")
	env.CreateCommit("feature a work")

	// Simulate a prior submit run that stored a PR base.
	env.Git("config", "branch.feature-a.stackPR", "7")
	env.Git("config", "branch.feature-a.stackPRBase", "main")

	// Verify both keys are present before orphan.
	if env.GetStackConfig("branch.feature-a.stackPRBase") != "main" {
		t.Fatal("stackPRBase should be set before orphan")
	}

	env.Git("checkout", "main")
	env.MustRun("orphan", "feature-a")

	if v := env.GetStackConfig("branch.feature-a.stackPR"); v != "" {
		t.Errorf("expected stackPR to be removed after orphan, got %q", v)
	}
	if v := env.GetStackConfig("branch.feature-a.stackPRBase"); v != "" {
		t.Errorf("expected stackPRBase to be removed after orphan, got %q", v)
	}
}

func TestOrphanForceClearsPRBaseOnDescendants(t *testing.T) {
	env := NewTestEnv(t)
	env.MustRun("init")

	env.MustRun("create", "feat-a")
	env.CreateCommit("a work")
	env.MustRun("create", "feat-b")
	env.CreateCommit("b work")

	// Simulate stored PR bases on both branches.
	env.Git("config", "branch.feat-a.stackPRBase", "main")
	env.Git("config", "branch.feat-b.stackPRBase", "feat-a")

	env.Git("checkout", "main")
	env.MustRun("orphan", "--force", "feat-a")

	if v := env.GetStackConfig("branch.feat-a.stackPRBase"); v != "" {
		t.Errorf("expected feat-a stackPRBase cleared, got %q", v)
	}
	if v := env.GetStackConfig("branch.feat-b.stackPRBase"); v != "" {
		t.Errorf("expected feat-b stackPRBase cleared, got %q", v)
	}
}
