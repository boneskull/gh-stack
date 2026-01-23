// internal/git/git.go
package git

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrDirtyWorkTree is returned when the working tree has uncommitted changes.
var ErrDirtyWorkTree = errors.New("working tree has uncommitted changes")

// Git provides git operations for a repository.
type Git struct {
	repoPath string
}

// New creates a Git instance for the repository at the given path.
func New(repoPath string) *Git {
	return &Git{repoPath: repoPath}
}

// CurrentBranch returns the name of the current branch.
func (g *Git) CurrentBranch() (string, error) {
	out, err := exec.Command("git", "-C", g.repoPath, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// BranchExists checks if a branch exists.
func (g *Git) BranchExists(branch string) bool {
	err := exec.Command("git", "-C", g.repoPath, "rev-parse", "--verify", "refs/heads/"+branch).Run()
	return err == nil
}

// CreateBranch creates a new branch at the current HEAD.
func (g *Git) CreateBranch(name string) error {
	return exec.Command("git", "-C", g.repoPath, "branch", name).Run()
}

// Checkout switches to the specified branch.
func (g *Git) Checkout(branch string) error {
	return exec.Command("git", "-C", g.repoPath, "checkout", branch).Run()
}

// CreateAndCheckout creates a new branch and switches to it.
func (g *Git) CreateAndCheckout(name string) error {
	return exec.Command("git", "-C", g.repoPath, "checkout", "-b", name).Run()
}

// IsDirty returns true if there are uncommitted changes (staged or unstaged).
func (g *Git) IsDirty() (bool, error) {
	out, err := exec.Command("git", "-C", g.repoPath, "status", "--porcelain").Output()
	if err != nil {
		return false, err
	}
	return len(strings.TrimSpace(string(out))) > 0, nil
}

// HasStagedChanges returns true if there are staged changes.
func (g *Git) HasStagedChanges() (bool, error) {
	err := exec.Command("git", "-C", g.repoPath, "diff", "--cached", "--quiet").Run()
	if err != nil {
		// Exit code 1 means there are differences
		return true, nil
	}
	return false, nil
}

// Commit creates a commit with the given message.
func (g *Git) Commit(message string) error {
	return exec.Command("git", "-C", g.repoPath, "commit", "-m", message).Run()
}

// Push force-pushes a branch to origin with lease.
func (g *Git) Push(branch string, force bool) error {
	args := []string{"-C", g.repoPath, "push", "origin", branch}
	if force {
		args = append(args, "--force-with-lease")
	}
	cmd := exec.Command("git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// GetMergeBase returns the merge base of two branches.
func (g *Git) GetMergeBase(a, b string) (string, error) {
	out, err := exec.Command("git", "-C", g.repoPath, "merge-base", a, b).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// GetTip returns the commit SHA at the tip of a branch.
func (g *Git) GetTip(branch string) (string, error) {
	out, err := exec.Command("git", "-C", g.repoPath, "rev-parse", branch).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
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
	cmd := exec.Command("git", "-C", g.repoPath, "rebase", onto)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// RebaseContinue continues an in-progress rebase.
func (g *Git) RebaseContinue() error {
	cmd := exec.Command("git", "-C", g.repoPath, "rebase", "--continue")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// RebaseAbort aborts an in-progress rebase.
func (g *Git) RebaseAbort() error {
	return exec.Command("git", "-C", g.repoPath, "rebase", "--abort").Run()
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
	cmd := exec.Command("git", "-C", g.repoPath, "fetch", "origin")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// FastForward fast-forwards a branch to its remote tracking branch.
func (g *Git) FastForward(branch string) error {
	// First checkout the branch
	if err := g.Checkout(branch); err != nil {
		return err
	}
	// Then merge with fast-forward only
	return exec.Command("git", "-C", g.repoPath, "merge", "--ff-only", "origin/"+branch).Run()
}

// DeleteBranch deletes a local branch.
func (g *Git) DeleteBranch(branch string) error {
	return exec.Command("git", "-C", g.repoPath, "branch", "-D", branch).Run()
}
