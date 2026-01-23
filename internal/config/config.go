// internal/config/config.go
package config

import (
	"errors"
	"os/exec"
	"strings"
)

// ErrNotInitialized is returned when stack tracking is not initialized.
var ErrNotInitialized = errors.New("stack not initialized: run 'gh stack init' first")

// ErrBranchNotTracked is returned when a branch is not tracked.
var ErrBranchNotTracked = errors.New("branch not tracked")

// Config provides access to stack metadata stored in .git/config.
type Config struct {
	repoPath string
}

// Load creates a Config for the repository at the given path.
func Load(repoPath string) (*Config, error) {
	if _, err := exec.Command("git", "-C", repoPath, "rev-parse", "--git-dir").Output(); err != nil {
		return nil, errors.New("not a git repository")
	}
	return &Config{repoPath: repoPath}, nil
}

// GetTrunk returns the configured trunk branch name.
func (c *Config) GetTrunk() (string, error) {
	out, err := exec.Command("git", "-C", c.repoPath, "config", "--get", "stack.trunk").Output()
	if err != nil {
		return "", ErrNotInitialized
	}
	return strings.TrimSpace(string(out)), nil
}

// SetTrunk sets the trunk branch name.
func (c *Config) SetTrunk(branch string) error {
	cmd := exec.Command("git", "-C", c.repoPath, "config", "stack.trunk", branch)
	return cmd.Run()
}

// GetParent returns the parent branch for the given branch.
func (c *Config) GetParent(branch string) (string, error) {
	key := "branch." + branch + ".stackParent"
	out, err := exec.Command("git", "-C", c.repoPath, "config", "--get", key).Output()
	if err != nil {
		return "", ErrBranchNotTracked
	}
	return strings.TrimSpace(string(out)), nil
}

// SetParent sets the parent branch for the given branch.
func (c *Config) SetParent(branch, parent string) error {
	key := "branch." + branch + ".stackParent"
	return exec.Command("git", "-C", c.repoPath, "config", key, parent).Run()
}

// RemoveParent removes the parent tracking for a branch.
func (c *Config) RemoveParent(branch string) error {
	key := "branch." + branch + ".stackParent"
	// --unset returns error if key doesn't exist, which is fine
	exec.Command("git", "-C", c.repoPath, "config", "--unset", key).Run()
	return nil
}
