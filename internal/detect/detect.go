// Package detect provides automatic parent branch detection for untracked branches.
package detect

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/boneskull/gh-stack/internal/git"
	"github.com/boneskull/gh-stack/internal/github"
)

// Confidence represents how certain the detection is.
type Confidence int

const (
	// Ambiguous means multiple candidates tied or no signal was found.
	Ambiguous Confidence = iota
	// Medium means a unique merge-base winner was found.
	Medium
	// High means a PR base branch matched a tracked branch or trunk.
	High
)

// String returns a human-readable confidence label.
func (c Confidence) String() string {
	switch c {
	case High:
		return "high"
	case Medium:
		return "medium"
	case Ambiguous:
		return "ambiguous"
	default:
		return "unknown"
	}
}

// Result holds the outcome of a parent detection attempt.
type Result struct {
	Parent     string
	Confidence Confidence
	PRNumber   int      // non-zero if detected via PR
	Candidates []string // populated when Ambiguous, for prompting
}

// candidate holds a scored potential parent.
type candidate struct {
	name     string
	distance int
}

// DetectParentLocal detects the parent branch using only local git data (no network).
// It computes merge-base distance between the untracked branch and each candidate
// (trunk + tracked branches), returning the closest unique match.
func DetectParentLocal(branch string, tracked []string, trunk string, g *git.Git) (*Result, error) {
	return rankCandidates(branch, tracked, trunk, g)
}

// DetectParent detects the parent branch using PR data first, falling back to
// local merge-base analysis. The GitHub client may be nil, in which case only
// local detection is used.
func DetectParent(branch string, tracked []string, trunk string, g *git.Git, gh *github.Client) (*Result, error) {
	// Step 1: Try PR-based detection if GitHub client is available
	if gh != nil {
		pr, prErr := gh.FindPRByHead(branch)
		if prErr == nil && pr != nil {
			base := pr.Base.Ref
			if isCandidate(base, tracked, trunk) {
				return &Result{
					Parent:     base,
					Confidence: High,
					PRNumber:   pr.Number,
				}, nil
			}
			// PR exists but base is not tracked -- fall through
		}
		// No PR or error -- fall through to merge-base
	}

	// Step 2: Fall back to merge-base ranking
	return rankCandidates(branch, tracked, trunk, g)
}

// rankCandidates computes merge-base distance for each candidate and returns
// the closest unique match.
func rankCandidates(branch string, tracked []string, trunk string, g *git.Git) (*Result, error) {
	// Build candidate set: trunk + all tracked branches
	seen := make(map[string]bool)
	var candidates []candidate

	allCandidates := append([]string{trunk}, tracked...)
	for _, name := range allCandidates {
		if seen[name] || name == branch {
			continue
		}
		seen[name] = true

		mergeBase, err := g.GetMergeBase(branch, name)
		if err != nil {
			continue // skip candidates we can't compare
		}

		distance, err := g.RevListCount(mergeBase, branch)
		if err != nil {
			continue
		}

		candidates = append(candidates, candidate{name: name, distance: distance})
	}

	if len(candidates) == 0 {
		return &Result{Confidence: Ambiguous}, nil
	}

	// Sort by distance ascending (closest fork = most likely parent)
	slices.SortFunc(candidates, func(a, b candidate) int {
		return cmp.Compare(a.distance, b.distance)
	})

	best := candidates[0]

	// Check for tie with second-best
	if len(candidates) > 1 && candidates[1].distance == best.distance {
		// Ambiguous -- collect all tied candidates for prompting
		var tied []string
		for _, c := range candidates {
			if c.distance == best.distance {
				tied = append(tied, c.name)
			}
		}
		return &Result{
			Confidence: Ambiguous,
			Candidates: tied,
		}, nil
	}

	return &Result{
		Parent:     best.name,
		Confidence: Medium,
	}, nil
}

// isCandidate returns true if the given branch name is trunk or in the tracked set.
func isCandidate(name string, tracked []string, trunk string) bool {
	return name == trunk || slices.Contains(tracked, name)
}

// FindUntrackedCandidates returns local branches that are not tracked and not trunk.
// These are candidates for auto-detection.
func FindUntrackedCandidates(g *git.Git, tracked []string, trunk string) ([]string, error) {
	all, err := g.ListLocalBranches()
	if err != nil {
		return nil, fmt.Errorf("list local branches: %w", err)
	}

	trackedSet := make(map[string]bool)
	trackedSet[trunk] = true
	for _, b := range tracked {
		trackedSet[b] = true
	}

	var candidates []string
	for _, b := range all {
		if !trackedSet[b] {
			candidates = append(candidates, b)
		}
	}
	return candidates, nil
}
