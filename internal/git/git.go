// internal/git/git.go
package git

import (
	"errors"
	"os/exec"
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
