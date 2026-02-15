// internal/config/config.go
package config

import (
	"bufio"
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/boneskull/gh-stack/internal/git"
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
	g *git.Git
}

// New creates a Config that routes all git-config calls through the given Git instance.
func New(g *git.Git) (*Config, error) {
	if _, err := g.GetResolvedGitDir(); err != nil {
		return nil, errors.New("not a git repository")
	}
	return &Config{g: g}, nil
}

// GetTrunk returns the configured trunk branch name.
func (c *Config) GetTrunk() (string, error) {
	out, err := c.g.ConfigGet("stack.trunk")
	if err != nil {
		return "", ErrNotInitialized
	}
	return out, nil
}

// SetTrunk sets the trunk branch name.
func (c *Config) SetTrunk(branch string) error {
	return c.g.ConfigSet("stack.trunk", branch)
}

// GetParent returns the parent branch for the given branch.
func (c *Config) GetParent(branch string) (string, error) {
	key := "branch." + branch + ".stackParent"
	out, err := c.g.ConfigGet(key)
	if err != nil {
		return "", ErrBranchNotTracked
	}
	return out, nil
}

// SetParent sets the parent branch for the given branch.
func (c *Config) SetParent(branch, parent string) error {
	key := "branch." + branch + ".stackParent"
	return c.g.ConfigSet(key, parent)
}

// RemoveParent removes the parent tracking for a branch.
func (c *Config) RemoveParent(branch string) error {
	key := "branch." + branch + ".stackParent"
	// --unset returns error if key doesn't exist, which is fine
	_ = c.g.ConfigUnset(key) //nolint:errcheck // unset returns error if key missing
	return nil
}

// GetPR returns the PR number for the given branch.
func (c *Config) GetPR(branch string) (int, error) {
	key := "branch." + branch + ".stackPR"
	out, err := c.g.ConfigGet(key)
	if err != nil {
		return 0, ErrNoPR
	}
	return strconv.Atoi(out)
}

// SetPR sets the PR number for the given branch.
func (c *Config) SetPR(branch string, pr int) error {
	key := "branch." + branch + ".stackPR"
	return c.g.ConfigSet(key, strconv.Itoa(pr))
}

// RemovePR removes the PR association for a branch.
func (c *Config) RemovePR(branch string) error {
	key := "branch." + branch + ".stackPR"
	// --unset returns error if key doesn't exist, which is fine
	_ = c.g.ConfigUnset(key) //nolint:errcheck // unset returns error if key missing
	return nil
}

// GetForkPoint returns the stored fork point SHA for a branch.
// The fork point is where the branch originally diverged from its parent.
func (c *Config) GetForkPoint(branch string) (string, error) {
	key := "branch." + branch + ".stackForkPoint"
	out, err := c.g.ConfigGet(key)
	if err != nil {
		return "", ErrNoForkPoint
	}
	return out, nil
}

// SetForkPoint stores the fork point SHA for a branch.
func (c *Config) SetForkPoint(branch, sha string) error {
	key := "branch." + branch + ".stackForkPoint"
	return c.g.ConfigSet(key, sha)
}

// SetForkPointWithComment stores the fork point SHA with an inline comment.
// Requires Git 2.45+. The comment appears after the value on the same line.
// Returns an error with stderr content on failure, so callers can distinguish
// "unknown option" (old git) from other failures.
func (c *Config) SetForkPointWithComment(branch, sha, comment string) error {
	key := "branch." + branch + ".stackForkPoint"
	return c.g.ConfigSetWithComment(key, sha, comment)
}

// RemoveForkPoint removes the stored fork point for a branch.
func (c *Config) RemoveForkPoint(branch string) error {
	key := "branch." + branch + ".stackForkPoint"
	_ = c.g.ConfigUnset(key) //nolint:errcheck // unset returns error if key missing
	return nil
}

// ListTrackedBranches returns all branches that have a stackParent set.
func (c *Config) ListTrackedBranches() ([]string, error) {
	// Note: git normalizes config keys to lowercase, so stackParent becomes stackparent
	out, err := c.g.ConfigGetRegexp(`^branch\..*\.stackparent$`)
	if err != nil {
		// No matches is not an error — git config exits non-zero when no keys match
		return []string{}, nil //nolint:nilerr // non-zero exit = no matching config keys
	}

	var branches []string
	re := regexp.MustCompile(`^branch\.(.+)\.stackparent\s+`)
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		if matches := re.FindStringSubmatch(line); len(matches) > 1 {
			branches = append(branches, matches[1])
		}
	}
	return branches, nil
}
