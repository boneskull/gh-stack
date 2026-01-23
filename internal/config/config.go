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
