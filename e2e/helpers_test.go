// e2e/helpers_test.go
package e2e_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestEnv provides a test environment with an isolated git repository.
type TestEnv struct {
	t          *testing.T
	WorkDir    string
	RemoteDir  string
	BinaryPath string
}

// Result captures command execution output.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

func (r *Result) Success() bool {
	return r.ExitCode == 0
}

func (r *Result) Failed() bool {
	return r.ExitCode != 0
}

func (r *Result) ContainsStdout(s string) bool {
	return strings.Contains(r.Stdout, s)
}

func (r *Result) ContainsStderr(s string) bool {
	return strings.Contains(r.Stderr, s)
}

// NewTestEnv creates a new test environment with an initialized git repo.
func NewTestEnv(t *testing.T) *TestEnv {
	t.Helper()
	workDir := t.TempDir()

	env := &TestEnv{
		t:          t,
		WorkDir:    workDir,
		BinaryPath: binaryPath,
	}

	env.Git("init")
	env.Git("config", "user.email", "test@example.com")
	env.Git("config", "user.name", "Test User")

	env.WriteFile("README.md", "# Test Repository\n")
	env.Git("add", ".")
	env.Git("commit", "-m", "initial commit")

	return env
}

// NewTestEnvWithRemote creates test env with a simulated remote.
func NewTestEnvWithRemote(t *testing.T) *TestEnv {
	t.Helper()
	env := NewTestEnv(t)

	remoteDir := t.TempDir()
	env.RemoteDir = remoteDir

	cmd := exec.Command("git", "clone", "--bare", env.WorkDir, remoteDir)
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to create bare remote: %v", err)
	}

	env.Git("remote", "add", "origin", remoteDir)
	env.Git("push", "-u", "origin", "main")

	return env
}

// Run executes gh-stack with args, returns Result (doesn't fail on non-zero).
func (e *TestEnv) Run(args ...string) *Result {
	e.t.Helper()

	var stdout, stderr bytes.Buffer
	cmd := exec.Command(e.BinaryPath, args...)
	cmd.Dir = e.WorkDir
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	result := &Result{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: 0,
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			e.t.Fatalf("failed to run command: %v", err)
		}
	}

	return result
}

// MustRun executes gh-stack and fails the test on non-zero exit.
func (e *TestEnv) MustRun(args ...string) *Result {
	e.t.Helper()
	result := e.Run(args...)
	if result.Failed() {
		e.t.Fatalf("gh-stack %s failed (exit %d):\nstdout: %s\nstderr: %s",
			strings.Join(args, " "), result.ExitCode, result.Stdout, result.Stderr)
	}
	return result
}

// Git executes a git command in the work directory.
func (e *TestEnv) Git(args ...string) string {
	e.t.Helper()

	var stdout, stderr bytes.Buffer
	cmd := exec.Command("git", args...)
	cmd.Dir = e.WorkDir
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		e.t.Fatalf("git %s failed: %v\nstderr: %s", strings.Join(args, " "), err, stderr.String())
	}

	return strings.TrimSpace(stdout.String())
}

// GitRemote executes git in the remote (bare) repo.
func (e *TestEnv) GitRemote(args ...string) string {
	e.t.Helper()

	if e.RemoteDir == "" {
		e.t.Fatal("GitRemote called but no remote configured")
	}

	var stdout, stderr bytes.Buffer
	cmd := exec.Command("git", args...)
	cmd.Dir = e.RemoteDir
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		e.t.Fatalf("git (remote) %s failed: %v\nstderr: %s", strings.Join(args, " "), err, stderr.String())
	}

	return strings.TrimSpace(stdout.String())
}

// WriteFile writes content to a file in the work directory.
func (e *TestEnv) WriteFile(name, content string) {
	e.t.Helper()
	path := filepath.Join(e.WorkDir, name)

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		e.t.Fatalf("failed to create directory %s: %v", dir, err)
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		e.t.Fatalf("failed to write file %s: %v", name, err)
	}
}

// CreateCommit creates a commit with a unique file.
func (e *TestEnv) CreateCommit(msg string) string {
	e.t.Helper()
	filename := strings.ReplaceAll(msg, " ", "-") + ".txt"
	e.WriteFile(filename, msg+"\n")
	e.Git("add", filename)
	e.Git("commit", "-m", msg)
	return e.Git("rev-parse", "HEAD")
}

// CreateCommitOnFile modifies a specific file and commits.
func (e *TestEnv) CreateCommitOnFile(filename, content, msg string) string {
	e.t.Helper()
	e.WriteFile(filename, content)
	e.Git("add", filename)
	e.Git("commit", "-m", msg)
	return e.Git("rev-parse", "HEAD")
}

// CurrentBranch returns the current branch name.
func (e *TestEnv) CurrentBranch() string {
	e.t.Helper()
	return e.Git("rev-parse", "--abbrev-ref", "HEAD")
}

// BranchTip returns the SHA of a branch's tip.
func (e *TestEnv) BranchTip(branch string) string {
	e.t.Helper()
	return e.Git("rev-parse", branch)
}

// AssertBranch fails if current branch doesn't match.
func (e *TestEnv) AssertBranch(expected string) {
	e.t.Helper()
	actual := e.CurrentBranch()
	if actual != expected {
		e.t.Errorf("expected branch %q, got %q", expected, actual)
	}
}

// AssertBranchExists fails if branch doesn't exist.
func (e *TestEnv) AssertBranchExists(branch string) {
	e.t.Helper()
	cmd := exec.Command("git", "rev-parse", "--verify", "refs/heads/"+branch)
	cmd.Dir = e.WorkDir
	if err := cmd.Run(); err != nil {
		e.t.Errorf("expected branch %q to exist", branch)
	}
}

// AssertClean fails if working tree is dirty.
func (e *TestEnv) AssertClean() {
	e.t.Helper()
	status := e.Git("status", "--porcelain")
	if status != "" {
		e.t.Errorf("expected clean working tree, got:\n%s", status)
	}
}

// AssertRebaseInProgress fails if no rebase is active.
func (e *TestEnv) AssertRebaseInProgress() {
	e.t.Helper()
	rebaseMerge := filepath.Join(e.WorkDir, ".git", "rebase-merge")
	rebaseApply := filepath.Join(e.WorkDir, ".git", "rebase-apply")

	_, err1 := os.Stat(rebaseMerge)
	_, err2 := os.Stat(rebaseApply)

	if err1 != nil && err2 != nil {
		e.t.Error("expected rebase in progress")
	}
}

// AssertNoRebaseInProgress fails if rebase is active.
func (e *TestEnv) AssertNoRebaseInProgress() {
	e.t.Helper()
	rebaseMerge := filepath.Join(e.WorkDir, ".git", "rebase-merge")
	rebaseApply := filepath.Join(e.WorkDir, ".git", "rebase-apply")

	_, err1 := os.Stat(rebaseMerge)
	_, err2 := os.Stat(rebaseApply)

	if err1 == nil || err2 == nil {
		e.t.Error("expected no rebase in progress")
	}
}

// AssertAncestor fails if ancestor is not an ancestor of descendant.
func (e *TestEnv) AssertAncestor(ancestor, descendant string) {
	e.t.Helper()
	cmd := exec.Command("git", "merge-base", "--is-ancestor", ancestor, descendant)
	cmd.Dir = e.WorkDir
	if err := cmd.Run(); err != nil {
		e.t.Errorf("expected %q to be ancestor of %q", ancestor, descendant)
	}
}
