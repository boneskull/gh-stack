// e2e/scenario_helpers_test.go
package e2e_test

import (
	"os/exec"
	"strings"
)

// CreateConflict sets up rebase conflict between two branches.
func (e *TestEnv) CreateConflict(baseBranch, featureBranch string) string {
	e.t.Helper()

	conflictFile := "conflict.txt"

	e.Git("checkout", baseBranch)
	e.Git("checkout", "-b", featureBranch)

	e.WriteFile(conflictFile, "feature content\n")
	e.Git("add", conflictFile)
	e.Git("commit", "-m", "feature: add conflict.txt")

	e.Git("checkout", baseBranch)
	e.WriteFile(conflictFile, "base content\n")
	e.Git("add", conflictFile)
	e.Git("commit", "-m", "base: add conflict.txt")

	e.Git("checkout", featureBranch)

	return conflictFile
}

// CreateStackWithConflict creates a 3-branch stack where cascading will conflict.
// After setup, you should be on feature-a and cascade from there.
// The conflict occurs when feature-b rebases onto feature-a (after feature-a picks up main's changes).
func (e *TestEnv) CreateStackWithConflict() string {
	e.t.Helper()

	conflictFile := "shared.txt"

	// Initial state on main
	e.WriteFile(conflictFile, "initial content\n")
	e.Git("add", conflictFile)
	e.Git("commit", "-m", "initial shared.txt")

	// Create feature-a (doesn't modify shared.txt)
	e.MustRun("create", "feature-a")
	e.WriteFile("feature-a.txt", "feature a content\n")
	e.Git("add", "feature-a.txt")
	e.Git("commit", "-m", "feature-a: add feature-a.txt")

	// Create feature-b (modifies shared.txt)
	e.MustRun("create", "feature-b")
	e.WriteFile(conflictFile, "feature-b modified this\n")
	e.Git("add", conflictFile)
	e.Git("commit", "-m", "feature-b: modify shared.txt")

	// Move main forward with conflicting change to shared.txt
	e.Git("checkout", "main")
	e.WriteFile(conflictFile, "main modified this differently\n")
	e.Git("add", conflictFile)
	e.Git("commit", "-m", "main: modify shared.txt")

	// Return to feature-a for cascade (cascade operates on current + descendants)
	e.Git("checkout", "feature-a")

	return conflictFile
}

// ResolveConflict marks conflict file as resolved.
func (e *TestEnv) ResolveConflict(conflictFile string) {
	e.t.Helper()
	e.WriteFile(conflictFile, "resolved content\n")
	e.Git("add", conflictFile)
}

// SimulateSomeoneElsePushed simulates another dev pushing to remote.
func (e *TestEnv) SimulateSomeoneElsePushed(branch string) string {
	e.t.Helper()

	if e.RemoteDir == "" {
		e.t.Fatal("requires remote")
	}

	otherDir := e.t.TempDir()

	cmd := exec.Command("git", "clone", e.RemoteDir, otherDir)
	if err := cmd.Run(); err != nil {
		e.t.Fatalf("clone failed: %v", err)
	}

	runIn := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = otherDir
		if err := cmd.Run(); err != nil {
			e.t.Fatalf("git %v failed: %v", args, err)
		}
	}

	runIn("config", "user.email", "other@example.com")
	runIn("config", "user.name", "Other Developer")
	runIn("checkout", branch)

	// Create file and commit
	cmd = exec.Command("bash", "-c", "echo 'other dev change' > other-dev.txt")
	cmd.Dir = otherDir
	cmd.Run()

	runIn("add", "other-dev.txt")
	runIn("commit", "-m", "commit from other developer")
	runIn("push", "origin", branch)

	cmd = exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = otherDir
	out, _ := cmd.Output()
	return string(out)
}

// SimulatePRMerged simulates a PR being merged (branch deleted on remote).
func (e *TestEnv) SimulatePRMerged(prBranch, targetBranch string) {
	e.t.Helper()

	if e.RemoteDir == "" {
		e.t.Fatal("requires remote")
	}

	prTip := e.GitRemote("rev-parse", prBranch)
	e.GitRemote("update-ref", "refs/heads/"+targetBranch, prTip)
	e.GitRemote("branch", "-D", prBranch)
}

// FetchOrigin fetches from remote.
func (e *TestEnv) FetchOrigin() {
	e.t.Helper()
	e.Git("fetch", "origin")
}

// GetStackConfig reads stack config from git config.
func (e *TestEnv) GetStackConfig(key string) string {
	e.t.Helper()
	cmd := exec.Command("git", "config", "--get", key)
	cmd.Dir = e.WorkDir
	out, _ := cmd.Output()
	return strings.TrimSpace(string(out))
}

// AssertStackParent verifies branch's stack parent.
func (e *TestEnv) AssertStackParent(branch, expectedParent string) {
	e.t.Helper()
	actual := e.GetStackConfig("branch." + branch + ".stackparent")
	if actual != expectedParent {
		e.t.Errorf("branch %q: expected parent %q, got %q", branch, expectedParent, actual)
	}
}
