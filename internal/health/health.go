// internal/health/health.go
package health

import (
	"fmt"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/git"
)

// IssueKind identifies which health check produced an issue.
type IssueKind int

const (
	// KindBranchMissing means the branch does not exist locally.
	KindBranchMissing IssueKind = iota + 1
	// KindParentMissing means the parent branch does not exist locally.
	KindParentMissing
	// KindNoForkPoint means no fork point is stored in config.
	KindNoForkPoint
	// KindForkPointMissing means the stored fork point commit doesn't exist.
	KindForkPointMissing
	// KindForkPointNotAncestor means the stored fork point is not an ancestor of the branch.
	KindForkPointNotAncestor
	// KindDrift means the stored fork point doesn't match the computed merge-base.
	KindDrift
	// KindCheckFailed means a check itself errored (e.g., merge-base computation failed).
	KindCheckFailed
)

// Issue describes a branch health problem.
type Issue struct {
	Kind    IssueKind // which check produced this issue
	Message string    // plain text, no ANSI styling
	Fixable bool      // can `doctor --fix` repair this?
}

// CheckBranch runs lightweight health checks on a tracked branch.
// Returns nil if healthy. Checks run in order and bail on first blocker:
//  1. Branch exists locally
//  2. Parent exists locally (including trunk)
//  3. Fork point is stored in config
//  4. Fork point commit exists in repo
//  5. Fork point is ancestor of branch
//  6. Stored fork point matches merge-base (drift detection)
func CheckBranch(g *git.Git, cfg *config.Config, branch string) []Issue {
	// Check 1: branch exists locally
	if !g.BranchExists(branch) {
		return []Issue{{Kind: KindBranchMissing, Message: "Branch does not exist locally", Fixable: false}}
	}

	// Check 2: parent exists locally
	parent, err := cfg.GetParent(branch)
	if err != nil {
		return []Issue{{Kind: KindParentMissing, Message: "No parent configured", Fixable: false}}
	}

	trunk, _ := cfg.GetTrunk() //nolint:errcheck // caller should validate trunk exists
	if parent == trunk {
		if !g.BranchExists(trunk) {
			return []Issue{{Kind: KindParentMissing, Message: fmt.Sprintf("Trunk branch %s does not exist locally", trunk), Fixable: false}}
		}
	} else if !g.BranchExists(parent) {
		return []Issue{{Kind: KindParentMissing, Message: fmt.Sprintf("Parent branch %s does not exist locally", parent), Fixable: false}}
	}

	// Check 3: fork point is stored
	storedFP, err := cfg.GetForkPoint(branch)
	if err != nil {
		return []Issue{{Kind: KindNoForkPoint, Message: "No fork point stored", Fixable: true}}
	}

	// Check 4: fork point commit exists
	if !g.CommitExists(storedFP) {
		return []Issue{{Kind: KindForkPointMissing, Message: fmt.Sprintf("Stored fork point %s does not exist", git.AbbrevSHA(storedFP)), Fixable: true}}
	}

	// Check 5: fork point is ancestor of branch
	isAnc, err := g.IsAncestor(storedFP, branch)
	if err != nil {
		return []Issue{{Kind: KindCheckFailed, Message: fmt.Sprintf("Failed to check if stored fork point %s is an ancestor of %s: %v", git.AbbrevSHA(storedFP), branch, err), Fixable: false}}
	}
	if !isAnc {
		return []Issue{{Kind: KindForkPointNotAncestor, Message: fmt.Sprintf("Stored fork point %s is not an ancestor of %s", git.AbbrevSHA(storedFP), branch), Fixable: true}}
	}

	// Check 6: drift detection
	mergeBase, err := g.GetMergeBase(parent, branch)
	if err != nil {
		return []Issue{{Kind: KindCheckFailed, Message: fmt.Sprintf("Failed to compute merge-base for %s and %s: %v", parent, branch, err), Fixable: false}}
	}

	if storedFP != mergeBase {
		return []Issue{
			{Kind: KindDrift, Message: fmt.Sprintf("Stored parent: %s", parent), Fixable: false},
			{Kind: KindDrift, Message: fmt.Sprintf("Stored fork:   %s", git.AbbrevSHA(storedFP)), Fixable: false},
			{Kind: KindDrift, Message: fmt.Sprintf("Actual fork:   %s", git.AbbrevSHA(mergeBase)), Fixable: false},
			{Kind: KindDrift, Message: "Drift detected: stored fork point does not match merge-base", Fixable: true},
		}
	}

	return nil
}
