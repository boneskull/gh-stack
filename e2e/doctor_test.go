// e2e/doctor_test.go
package e2e_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDoctorHealthyStack(t *testing.T) {
	env := NewTestEnv(t)

	env.MustRun("init")
	env.MustRun("create", "feature-a")
	env.CreateCommit("work on a")
	env.MustRun("create", "feature-b")
	env.CreateCommit("work on b")

	result := env.MustRun("doctor")
	if !result.ContainsStdout("healthy") {
		t.Errorf("expected healthy output, got: %s", result.Stdout)
	}
	if !result.ContainsStdout("All branches healthy") {
		t.Errorf("expected 'All branches healthy', got: %s", result.Stdout)
	}
}

func TestDoctorDetectsDrift(t *testing.T) {
	env := NewTestEnv(t)

	env.MustRun("init")
	env.MustRun("create", "feature-a")
	env.CreateCommit("work on a")
	env.MustRun("create", "feature-b")
	env.CreateCommit("work on b")

	// Move main forward and rebase feature-a to cause stored fork point drift
	env.Git("checkout", "feature-a")
	env.CreateCommit("extra work on a")
	env.Git("checkout", "main")
	env.CreateCommit("main moves forward")
	env.Git("checkout", "feature-a")
	env.Git("rebase", "main")

	// Now the stored fork point is stale
	result := env.Run("doctor")
	if !result.ContainsStdout("Drift detected") && !result.ContainsStdout("not an ancestor") {
		t.Errorf("expected drift or ancestor issue, got: %s", result.Stdout)
	}

	// Fix it
	result = env.MustRun("doctor", "--fix")
	if !result.ContainsStdout("updated fork point") && !result.ContainsStdout("set fork point") {
		t.Errorf("expected fix message, got: %s", result.Stdout)
	}

	// Should be healthy now
	result = env.MustRun("doctor")
	if !result.ContainsStdout("All branches healthy") {
		t.Errorf("expected all healthy after fix, got: %s", result.Stdout)
	}
}

func TestDoctorMissingBranch(t *testing.T) {
	env := NewTestEnv(t)

	env.MustRun("init")
	env.MustRun("create", "feature-a")
	env.CreateCommit("work on a")

	// Remove the branch ref but keep the stack config entries
	env.Git("checkout", "main")
	env.Git("update-ref", "-d", "refs/heads/feature-a")

	result := env.Run("doctor")
	if !result.ContainsStdout("does not exist locally") {
		t.Errorf("expected 'does not exist locally', got: %s", result.Stdout)
	}
}

func TestDoctorMissingParent(t *testing.T) {
	env := NewTestEnv(t)

	env.MustRun("init")
	env.MustRun("create", "feature-a")
	env.CreateCommit("work on a")
	env.MustRun("create", "feature-b")
	env.CreateCommit("work on b")

	// Delete the parent branch (feature-a)
	env.Git("checkout", "main")
	env.Git("branch", "-D", "feature-a")

	result := env.Run("doctor")
	// feature-b should report its parent missing
	if !result.ContainsStdout("does not exist locally") {
		t.Errorf("expected parent missing report, got: %s", result.Stdout)
	}
}

func TestDoctorFixUpdatesConfig(t *testing.T) {
	env := NewTestEnv(t)

	env.MustRun("init")
	env.MustRun("create", "feature-a")
	env.CreateCommit("work on a")

	// Record the original fork point
	originalFP := strings.TrimSpace(env.Git("config", "--get", "branch.feature-a.stackForkPoint"))

	// Move main forward and rebase feature-a to cause drift
	env.Git("checkout", "main")
	env.CreateCommit("main moves")
	env.Git("checkout", "feature-a")
	env.Git("rebase", "main")

	// The stored fork point should still be the old one
	storedFP := strings.TrimSpace(env.Git("config", "--get", "branch.feature-a.stackForkPoint"))
	if storedFP != originalFP {
		t.Fatalf("expected stored fork point to still be %s, got %s", originalFP, storedFP)
	}

	// Run doctor --fix
	env.MustRun("doctor", "--fix")

	// Read the config directly to verify it changed
	newFP := strings.TrimSpace(env.Git("config", "--get", "branch.feature-a.stackForkPoint"))
	if newFP == originalFP {
		t.Errorf("expected fork point to be updated, but it's still %s", originalFP)
	}

	// Verify the new fork point matches the current merge-base
	mergeBase := strings.TrimSpace(env.Git("merge-base", "main", "feature-a"))
	if newFP != mergeBase {
		t.Errorf("expected new fork point %s to match merge-base %s", newFP, mergeBase)
	}

	// Verify provenance comment was written to the config file
	configData, err := os.ReadFile(filepath.Join(env.WorkDir, ".git", "config"))
	if err != nil {
		t.Fatalf("failed to read .git/config: %v", err)
	}
	configStr := string(configData)
	if !strings.Contains(configStr, "# doctor fix") {
		t.Error("expected provenance comment '# doctor fix ...' in .git/config")
	}
	if !strings.Contains(configStr, "# "+originalFP) {
		t.Errorf("expected old fork point %s preserved in config comment", originalFP)
	}
}

func TestDoctorNotInitialized(t *testing.T) {
	env := NewTestEnv(t)

	result := env.Run("doctor")
	if result.Success() {
		t.Error("expected doctor to fail before init")
	}
	if !result.ContainsStderr("not initialized") {
		t.Errorf("expected 'not initialized' error, got stderr: %s", result.Stderr)
	}
}
