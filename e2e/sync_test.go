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
// 2. Unit tests in cmd/sync_test.go - test sync-specific starting branch capture

func TestSyncAcceptsDryRunFlag(t *testing.T) {
	// Verify --dry-run flag is recognized by sync
	env := NewTestEnv(t)
	env.MustRun("init")

	result := env.Run("sync", "--help")
	if !result.ContainsStdout("--dry-run") {
		t.Errorf("expected sync --help to show --dry-run flag, got:\n%s", result.Stdout)
	}
}
