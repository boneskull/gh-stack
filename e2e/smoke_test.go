// e2e/smoke_test.go
package e2e_test

import (
	"testing"
)

func TestSmokeInitAndCreate(t *testing.T) {
	env := NewTestEnv(t)

	// Test init
	result := env.MustRun("init")
	if !result.ContainsStdout("main") {
		t.Errorf("init output should mention trunk: %s", result.Stdout)
	}

	// Test create
	_ = env.MustRun("create", "feature-1")
	env.AssertBranch("feature-1")

	// Make a commit
	env.CreateCommit("feature 1 work")

	// Test log
	result = env.MustRun("log")
	if !result.ContainsStdout("feature-1") {
		t.Errorf("log should show branch: %s", result.Stdout)
	}
}
