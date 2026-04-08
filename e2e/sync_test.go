// e2e/sync_test.go
package e2e_test

import "testing"

// Note: Full E2E testing of sync's return-to-original-branch behavior is limited
// because sync requires a real GitHub remote. The underlying return-to-branch logic
// is tested via TestCascadeReturnsToOriginalBranch in cascade_test.go, since sync
// delegates restacking to the same doCascadeWithState function.
//
// The fix for issue #58 (sync leaving user on wrong branch) is validated by:
// 1. TestCascadeReturnsToOriginalBranch - tests the shared restack infrastructure
// 2. cmd/sync_test.go unit tests - test sync-specific starting branch capture:
//    - TestSyncStartingBranchCapture - normal branch detection
//    - TestSyncStartingBranchDetachedHEAD - "HEAD" normalization for detached state
//    - TestSyncReturnsToBranchAfterOperations - successful return to starting branch
//    - TestSyncCheckoutFailure - checkout failure while returning to starting branch
//    - TestSyncStartingBranchDeleted - handling when branch is deleted during sync
//    - TestSyncEmptyStartingBranchSkipsReturn - empty starting branch skips return

func TestSyncAcceptsDryRunFlag(t *testing.T) {
	// Verify --dry-run flag is recognized by sync
	env := NewTestEnv(t)
	env.MustRun("init")

	result := env.Run("sync", "--help")
	if !result.ContainsStdout("--dry-run") {
		t.Errorf("expected sync --help to show --dry-run flag, got:\n%s", result.Stdout)
	}
}
