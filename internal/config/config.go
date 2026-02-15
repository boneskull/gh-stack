// internal/config/config.go
package config

import (
	"bufio"
	"bytes"
	"errors"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// ErrNotInitialized is returned when stack tracking is not initialized.
var ErrNotInitialized = errors.New("stack not initialized: run 'gh stack init' first")

// ErrBranchNotTracked is returned when a branch is not tracked.
var ErrBranchNotTracked = errors.New("branch not tracked")

// ErrNoPR is returned when a branch has no associated PR.
var ErrNoPR = errors.New("no PR associated with branch")

// ErrNoForkPoint is returned when a branch has no stored fork point.
var ErrNoForkPoint = errors.New("no fork point stored for branch")

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
	_ = exec.Command("git", "-C", c.repoPath, "config", "--unset", key).Run() //nolint:errcheck // unset returns error if key missing
	return nil
}

// GetPR returns the PR number for the given branch.
func (c *Config) GetPR(branch string) (int, error) {
	key := "branch." + branch + ".stackPR"
	out, err := exec.Command("git", "-C", c.repoPath, "config", "--get", key).Output()
	if err != nil {
		return 0, ErrNoPR
	}
	return strconv.Atoi(strings.TrimSpace(string(out)))
}

// SetPR sets the PR number for the given branch.
func (c *Config) SetPR(branch string, pr int) error {
	key := "branch." + branch + ".stackPR"
	return exec.Command("git", "-C", c.repoPath, "config", key, strconv.Itoa(pr)).Run()
}

// RemovePR removes the PR association for a branch.
func (c *Config) RemovePR(branch string) error {
	key := "branch." + branch + ".stackPR"
	// --unset returns error if key doesn't exist, which is fine
	_ = exec.Command("git", "-C", c.repoPath, "config", "--unset", key).Run() //nolint:errcheck // unset returns error if key missing
	return nil
}

// GetForkPoint returns the stored fork point SHA for a branch.
// The fork point is where the branch originally diverged from its parent.
func (c *Config) GetForkPoint(branch string) (string, error) {
	key := "branch." + branch + ".stackForkPoint"
	out, err := exec.Command("git", "-C", c.repoPath, "config", "--get", key).Output()
	if err != nil {
		return "", ErrNoForkPoint
	}
	return strings.TrimSpace(string(out)), nil
}

// SetForkPoint stores the fork point SHA for a branch.
func (c *Config) SetForkPoint(branch, sha string) error {
	key := "branch." + branch + ".stackForkPoint"
	return exec.Command("git", "-C", c.repoPath, "config", key, sha).Run()
}

// SetForkPointWithComment stores the fork point SHA with an inline comment.
// Requires Git 2.45+. The comment appears after the value on the same line.
// Returns an error with stderr content on failure, so callers can distinguish
// "unknown option" (old git) from other failures.
func (c *Config) SetForkPointWithComment(branch, sha, comment string) error {
	key := "branch." + branch + ".stackForkPoint"
	cmd := exec.Command("git", "-C", c.repoPath, "config", "--comment", comment, key, sha)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return errors.New(strings.TrimSpace(stderr.String()))
		}
		return err
	}
	return nil
}

// RemoveForkPoint removes the stored fork point for a branch.
func (c *Config) RemoveForkPoint(branch string) error {
	key := "branch." + branch + ".stackForkPoint"
	_ = exec.Command("git", "-C", c.repoPath, "config", "--unset", key).Run() //nolint:errcheck // unset returns error if key missing
	return nil
}

// ListTrackedBranches returns all branches that have a stackParent set.
func (c *Config) ListTrackedBranches() ([]string, error) {
	// Note: git normalizes config keys to lowercase, so stackParent becomes stackparent
	out, err := exec.Command("git", "-C", c.repoPath, "config", "--get-regexp", "^branch\\..*\\.stackparent$").Output()
	if err != nil {
		// No matches is not an error — git config exits non-zero when no keys match
		return []string{}, nil //nolint:nilerr // non-zero exit = no matching config keys
	}

	var branches []string
	re := regexp.MustCompile(`^branch\.(.+)\.stackparent\s+`)
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		if matches := re.FindStringSubmatch(line); len(matches) > 1 {
			branches = append(branches, matches[1])
		}
	}
	return branches, nil
}
