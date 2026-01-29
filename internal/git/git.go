// internal/git/git.go
package git

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/cli/safeexec"
)

// ErrDirtyWorkTree is returned when the working tree has uncommitted changes.
var ErrDirtyWorkTree = errors.New("working tree has uncommitted changes")

var (
	gitPath     string
	gitPathOnce sync.Once
	gitPathErr  error
)

// resolveGitPath finds the git executable using safeexec to prevent PATH injection.
func resolveGitPath() (string, error) {
	gitPathOnce.Do(func() {
		gitPath, gitPathErr = safeexec.LookPath("git")
	})
	return gitPath, gitPathErr
}

// Git provides git operations for a repository.
type Git struct {
	repoPath string
}

// New creates a Git instance for the repository at the given path.
func New(repoPath string) *Git {
	return &Git{repoPath: repoPath}
}

// run executes a git command and returns stdout. Stderr is captured for error messages.
func (g *Git) run(args ...string) (string, error) {
	gitBin, err := resolveGitPath()
	if err != nil {
		return "", fmt.Errorf("failed to find git: %w", err)
	}

	fullArgs := append([]string{"-C", g.repoPath}, args...)
	cmd := exec.Command(gitBin, fullArgs...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return "", fmt.Errorf("git %s: %s", args[0], strings.TrimSpace(stderr.String()))
		}
		return "", fmt.Errorf("git %s: %w", args[0], err)
	}

	return strings.TrimSpace(stdout.String()), nil
}

// runInteractive executes a git command with stdout/stderr connected to the terminal.
func (g *Git) runInteractive(args ...string) error {
	gitBin, err := resolveGitPath()
	if err != nil {
		return fmt.Errorf("failed to find git: %w", err)
	}

	fullArgs := append([]string{"-C", g.repoPath}, args...)
	cmd := exec.Command(gitBin, fullArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// runSilent executes a git command and discards output, returning only success/failure.
func (g *Git) runSilent(args ...string) error {
	_, err := g.run(args...)
	return err
}

// CurrentBranch returns the name of the current branch.
func (g *Git) CurrentBranch() (string, error) {
	return g.run("rev-parse", "--abbrev-ref", "HEAD")
}

// BranchExists checks if a branch exists.
func (g *Git) BranchExists(branch string) bool {
	err := g.runSilent("rev-parse", "--verify", "refs/heads/"+branch)
	return err == nil
}

// CreateBranch creates a new branch at the current HEAD.
func (g *Git) CreateBranch(name string) error {
	return g.runSilent("branch", name)
}

// Checkout switches to the specified branch.
func (g *Git) Checkout(branch string) error {
	return g.runSilent("checkout", branch)
}

// CreateAndCheckout creates a new branch and switches to it.
func (g *Git) CreateAndCheckout(name string) error {
	return g.runSilent("checkout", "-b", name)
}

// IsDirty returns true if there are uncommitted changes (staged or unstaged).
func (g *Git) IsDirty() (bool, error) {
	out, err := g.run("status", "--porcelain")
	if err != nil {
		return false, err
	}
	return len(out) > 0, nil
}

// HasStagedChanges returns true if there are staged changes.
func (g *Git) HasStagedChanges() (bool, error) {
	err := g.runSilent("diff", "--cached", "--quiet")
	if err != nil {
		// Exit code 1 means there are differences
		return true, nil
	}
	return false, nil
}

// Commit creates a commit with the given message.
func (g *Git) Commit(message string) error {
	return g.runSilent("commit", "-m", message)
}

// Push force-pushes a branch to origin with lease.
func (g *Git) Push(branch string, force bool) error {
	args := []string{"push", "origin", branch}
	if force {
		args = append(args, "--force-with-lease")
	}
	return g.runInteractive(args...)
}

// GetMergeBase returns the merge base of two branches.
func (g *Git) GetMergeBase(a, b string) (string, error) {
	return g.run("merge-base", a, b)
}

// GetTip returns the commit SHA at the tip of a branch.
func (g *Git) GetTip(branch string) (string, error) {
	return g.run("rev-parse", branch)
}

// NeedsRebase returns true if branch needs to be rebased onto parent.
func (g *Git) NeedsRebase(branch, parent string) (bool, error) {
	mergeBase, err := g.GetMergeBase(branch, parent)
	if err != nil {
		return false, err
	}
	parentTip, err := g.GetTip(parent)
	if err != nil {
		return false, err
	}
	return mergeBase != parentTip, nil
}

// Rebase rebases the current branch onto target.
func (g *Git) Rebase(onto string) error {
	return g.runInteractive("rebase", onto)
}

// RebaseContinue continues an in-progress rebase.
func (g *Git) RebaseContinue() error {
	return g.runInteractive("rebase", "--continue")
}

// RebaseAbort aborts an in-progress rebase.
func (g *Git) RebaseAbort() error {
	return g.runSilent("rebase", "--abort")
}

// IsRebaseInProgress checks if a rebase is in progress.
func (g *Git) IsRebaseInProgress() bool {
	rebaseMerge := filepath.Join(g.repoPath, ".git", "rebase-merge")
	rebaseApply := filepath.Join(g.repoPath, ".git", "rebase-apply")
	_, err1 := os.Stat(rebaseMerge)
	_, err2 := os.Stat(rebaseApply)
	return err1 == nil || err2 == nil
}

// GetGitDir returns the .git directory path.
func (g *Git) GetGitDir() string {
	return filepath.Join(g.repoPath, ".git")
}

// Fetch fetches from origin.
func (g *Git) Fetch() error {
	return g.runInteractive("fetch", "origin")
}

// FastForward fast-forwards a branch to its remote tracking branch.
func (g *Git) FastForward(branch string) error {
	// First checkout the branch
	if err := g.Checkout(branch); err != nil {
		return err
	}
	// Then merge with fast-forward only
	return g.runSilent("merge", "--ff-only", "origin/"+branch)
}

// DeleteBranch deletes a local branch.
func (g *Git) DeleteBranch(branch string) error {
	return g.runSilent("branch", "-D", branch)
}

// Commit represents a git commit with its subject and body.
type Commit struct {
	Subject string // First line of the commit message
	Body    string // Everything after the first line (may be empty)
}

// GetCommits returns the commits from base..head (commits in head not in base).
// Returns commits in reverse chronological order (newest first).
func (g *Git) GetCommits(base, head string) ([]Commit, error) {
	// Use null byte separators for reliable parsing
	// Format: subject\x00body\x00\x00 (double null between commits)
	format := "%s%x00%b%x00%x00"
	out, err := g.run("log", "--format="+format, base+".."+head)
	if err != nil {
		return nil, err
	}

	if out == "" {
		return nil, nil
	}

	var commits []Commit
	// Split by double null (between commits)
	entries := strings.Split(out, "\x00\x00")
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		// Split by single null (between subject and body)
		parts := strings.SplitN(entry, "\x00", 2)
		subject := strings.TrimSpace(parts[0])
		var body string
		if len(parts) > 1 {
			body = strings.TrimSpace(parts[1])
		}
		if subject != "" {
			commits = append(commits, Commit{Subject: subject, Body: body})
		}
	}

	return commits, nil
}
