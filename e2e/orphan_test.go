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

func TestOrphanDisconnectedBranchIsAllowed(t *testing.T) {
	env := NewTestEnv(t)
	env.MustRun("init")

	// Create the branch chain main -> feat-a, then manually delete the
	// parent link so feat-a's recorded parent ("missing-branch") is not
	// itself a tracked branch. This mirrors the scenario in #116 where
	// the parent is no longer valid and `gh stack orphan` previously
	// refused to orphan the current branch.
	env.MustRun("create", "feat-a")
	env.CreateCommit("feat-a work")
	env.Git("config", "branch.feat-a.stackParent", "missing-branch")

	if env.GetStackConfig("branch.feat-a.stackParent") != "missing-branch" {
		t.Fatal("expected feat-a parent override to land before orphan")
	}

	result := env.Run("orphan", "feat-a")
	if !result.Success() {
		t.Fatalf("expected orphan of disconnected branch to succeed, got exit %d stderr=%q", result.ExitCode, result.Stderr)
	}
	if v := env.GetStackConfig("branch.feat-a.stackParent"); v != "" {
		t.Errorf("expected feat-a stackParent cleared after orphan, got %q", v)
	}
}
