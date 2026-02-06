// e2e/undo_test.go
package e2e_test

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUndoCascade(t *testing.T) {
	env := NewTestEnv(t)
	env.MustRun("init")

	// Create stack
	env.MustRun("create", "feature-a")
	env.CreateCommit("feature a work")
	featureASha := env.BranchTip("feature-a")

	env.MustRun("create", "feature-b")
	env.CreateCommit("feature b work")
	featureBSha := env.BranchTip("feature-b")

	// Add commit to main
	env.Git("checkout", "main")
	env.CreateCommit("main moved forward")

	// Cascade from feature-a
	env.Git("checkout", "feature-a")
	env.MustRun("cascade")

	// Verify branches moved
	newFeatureASha := env.BranchTip("feature-a")
	newFeatureBSha := env.BranchTip("feature-b")
	if newFeatureASha == featureASha {
		t.Error("feature-a should have been rebased")
	}
	if newFeatureBSha == featureBSha {
		t.Error("feature-b should have been rebased")
	}

	// Undo the cascade
	env.MustRun("undo", "--force")

	// Verify branches restored
	restoredFeatureASha := env.BranchTip("feature-a")
	restoredFeatureBSha := env.BranchTip("feature-b")
	if restoredFeatureASha != featureASha {
		t.Errorf("feature-a not restored: expected %s, got %s", featureASha, restoredFeatureASha)
	}
	if restoredFeatureBSha != featureBSha {
		t.Errorf("feature-b not restored: expected %s, got %s", featureBSha, restoredFeatureBSha)
	}
}

func TestUndoDryRun(t *testing.T) {
	env := NewTestEnv(t)
	env.MustRun("init")

	// Create stack and cascade
	env.MustRun("create", "feature-a")
	env.CreateCommit("feature a work")
	featureASha := env.BranchTip("feature-a")

	env.Git("checkout", "main")
	env.CreateCommit("main moved forward")

	env.Git("checkout", "feature-a")
	env.MustRun("cascade")

	// Verify branch moved
	cascadedSha := env.BranchTip("feature-a")
	if cascadedSha == featureASha {
		t.Fatal("cascade didn't change SHA")
	}

	// Dry run should not change anything
	result := env.MustRun("undo", "--dry-run")
	if !result.ContainsStdout("Dry run") {
		t.Error("expected dry-run message in output")
	}

	// Branch should still be at cascaded SHA
	afterDryRunSha := env.BranchTip("feature-a")
	if afterDryRunSha != cascadedSha {
		t.Error("dry-run should not have changed branch")
	}
}

func TestUndoNothingToUndo(t *testing.T) {
	env := NewTestEnv(t)
	env.MustRun("init")

	result := env.MustRun("undo", "--force")
	if !result.ContainsStdout("Nothing to undo") {
		t.Error("expected 'Nothing to undo' message")
	}
}

func TestUndoRestoresConfig(t *testing.T) {
	env := NewTestEnv(t)
	env.MustRun("init")

	// Create stack
	env.MustRun("create", "feature-a")
	env.CreateCommit("feature a work")
	originalForkPoint := env.GetStackConfig("branch.feature-a.stackforkpoint")

	env.Git("checkout", "main")
	env.CreateCommit("main moved forward")

	env.Git("checkout", "feature-a")
	env.MustRun("cascade")

	// Cascade updates fork point
	newForkPoint := env.GetStackConfig("branch.feature-a.stackforkpoint")
	if newForkPoint == originalForkPoint {
		// Fork point should have changed (or been set)
		t.Log("Fork point unchanged (may be expected if not set initially)")
	}

	// Undo
	env.MustRun("undo", "--force")

	// Fork point should be restored
	restoredForkPoint := env.GetStackConfig("branch.feature-a.stackforkpoint")
	if restoredForkPoint != originalForkPoint {
		t.Errorf("fork point not restored: expected %q, got %q", originalForkPoint, restoredForkPoint)
	}
}

func TestUndoArchivesSnapshot(t *testing.T) {
	env := NewTestEnv(t)
	env.MustRun("init")

	env.MustRun("create", "feature-a")
	env.CreateCommit("feature a work")

	env.Git("checkout", "main")
	env.CreateCommit("main moved forward")

	env.Git("checkout", "feature-a")
	env.MustRun("cascade")

	// Verify snapshot exists
	undoDir := filepath.Join(env.WorkDir, ".git", "stack-undo")
	entries, err := os.ReadDir(undoDir)
	if err != nil {
		t.Fatalf("failed to read undo dir: %v", err)
	}
	snapshotCount := 0
	for _, e := range entries {
		if !e.IsDir() {
			snapshotCount++
		}
	}
	if snapshotCount != 1 {
		t.Errorf("expected 1 snapshot, got %d", snapshotCount)
	}

	// Undo
	env.MustRun("undo", "--force")

	// Snapshot should be archived
	entries, _ = os.ReadDir(undoDir)
	snapshotCount = 0
	for _, e := range entries {
		if !e.IsDir() {
			snapshotCount++
		}
	}
	if snapshotCount != 0 {
		t.Error("snapshot should have been archived after undo")
	}

	// Check done/ directory
	doneDir := filepath.Join(undoDir, "done")
	entries, err = os.ReadDir(doneDir)
	if err != nil {
		t.Fatalf("failed to read done dir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 archived snapshot, got %d", len(entries))
	}
}

func TestCascadeWithAutoStashRestoresAfterSuccess(t *testing.T) {
	env := NewTestEnv(t)
	env.MustRun("init")

	env.MustRun("create", "feature-a")
	env.CreateCommit("feature a work")

	env.Git("checkout", "main")
	env.CreateCommit("main moved forward")

	env.Git("checkout", "feature-a")

	// Create uncommitted changes
	env.WriteFile("uncommitted.txt", "uncommitted content\n")

	// Cascade should auto-stash and then restore after success
	result := env.MustRun("cascade")
	if !result.ContainsStdout("Auto-stashed") {
		t.Error("expected auto-stash message")
	}
	if !result.ContainsStdout("Restoring auto-stashed") {
		t.Error("expected restore message after successful cascade")
	}

	// Uncommitted file should be restored after successful cascade
	content, err := os.ReadFile(filepath.Join(env.WorkDir, "uncommitted.txt"))
	if err != nil {
		t.Errorf("uncommitted file should be present after cascade: %v", err)
	} else if string(content) != "uncommitted content\n" {
		t.Errorf("uncommitted file has wrong content: %q", content)
	}
}

func TestUndoRestoresOriginalBranch(t *testing.T) {
	env := NewTestEnv(t)
	env.MustRun("init")

	env.MustRun("create", "feature-a")
	env.CreateCommit("feature a work")

	env.MustRun("create", "feature-b")
	env.CreateCommit("feature b work")

	env.Git("checkout", "main")
	env.CreateCommit("main moved forward")

	// Cascade from feature-a
	env.Git("checkout", "feature-a")
	env.MustRun("cascade")

	// Should still be on feature-a
	env.AssertBranch("feature-a")

	// Now checkout something else
	env.Git("checkout", "feature-b")

	// Undo should restore to feature-a (original head at time of cascade)
	env.MustRun("undo", "--force")
	env.AssertBranch("feature-a")
}

func TestUndoBlockedDuringCascade(t *testing.T) {
	env := NewTestEnv(t)
	env.MustRun("init")

	conflictFile := env.CreateStackWithConflict()
	_ = conflictFile

	// Start cascade (will conflict)
	result := env.Run("cascade")
	if result.Success() {
		t.Fatal("expected cascade to fail on conflict")
	}
	env.AssertRebaseInProgress()

	// Undo should be blocked
	result = env.Run("undo", "--force")
	if result.Success() {
		t.Error("undo should fail during cascade in progress")
	}
	if !result.ContainsStdout("operation is in progress") && !result.ContainsStderr("operation is in progress") {
		t.Errorf("expected message about operation in progress, got stdout: %s, stderr: %s", result.Stdout, result.Stderr)
	}

	// Abort the cascade
	env.MustRun("abort")
}

func TestMultipleCascadesUndoLatest(t *testing.T) {
	env := NewTestEnv(t)
	env.MustRun("init")

	// Create stack
	env.MustRun("create", "feature-a")
	env.CreateCommit("feature a v1")
	featureAv1 := env.BranchTip("feature-a")

	// First cascade trigger
	env.Git("checkout", "main")
	env.CreateCommit("main update 1")
	env.Git("checkout", "feature-a")
	env.MustRun("cascade")

	featureAAfterFirst := env.BranchTip("feature-a")
	if featureAAfterFirst == featureAv1 {
		t.Fatal("first cascade didn't change SHA")
	}

	// Second cascade trigger
	env.Git("checkout", "main")
	env.CreateCommit("main update 2")
	env.Git("checkout", "feature-a")
	env.MustRun("cascade")

	featureAAfterSecond := env.BranchTip("feature-a")
	if featureAAfterSecond == featureAAfterFirst {
		t.Fatal("second cascade didn't change SHA")
	}

	// Undo should restore to state before SECOND cascade (not first)
	env.MustRun("undo", "--force")
	featureAAfterUndo := env.BranchTip("feature-a")
	if featureAAfterUndo != featureAAfterFirst {
		t.Errorf("undo should restore to state before second cascade: expected %s, got %s",
			featureAAfterFirst, featureAAfterUndo)
	}
}
