package cmd

import (
	"fmt"
	"slices"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/detect"
	"github.com/boneskull/gh-stack/internal/git"
	"github.com/boneskull/gh-stack/internal/github"
	"github.com/boneskull/gh-stack/internal/prompt"
	"github.com/boneskull/gh-stack/internal/style"
	"github.com/boneskull/gh-stack/internal/tree"
)

// autoDetectAndAdopt finds untracked branches and adopts them using full detection
// (PR + merge-base). Prompts for ambiguous cases in interactive mode.
func autoDetectAndAdopt(cfg *config.Config, g *git.Git, gh *github.Client, s *style.Style) error {
	trunk, err := cfg.GetTrunk()
	if err != nil {
		return err
	}

	tracked, err := cfg.ListTrackedBranches()
	if err != nil {
		return err
	}

	candidates, err := detect.FindUntrackedCandidates(g, tracked, trunk)
	if err != nil {
		return err
	}

	if len(candidates) == 0 {
		return nil
	}

	// Build tree for cycle checking
	root, err := tree.Build(cfg)
	if err != nil {
		return err
	}

	// Loop until no progress: untracked chains (e.g., C based on untracked B)
	// may require multiple passes since a branch can only be detected once its
	// parent has been adopted into the tracked set.
	adopted := make(map[string]bool)
	for {
		progress := false
		for _, branch := range candidates {
			if adopted[branch] {
				continue
			}

			result, detectErr := detect.DetectParent(branch, tracked, trunk, g, gh)
			if detectErr != nil {
				fmt.Printf("%s could not detect parent for %s: %v\n",
					s.WarningIcon(), s.Branch(branch), detectErr)
				continue
			}

			var parent string
			switch result.Confidence {
			case detect.High, detect.Medium:
				parent = result.Parent
			case detect.Ambiguous:
				if prompt.IsInteractive() && len(result.Candidates) > 0 {
					selected, selErr := prompt.Select(
						fmt.Sprintf("Select parent for %s:", branch),
						result.Candidates, 0)
					if selErr != nil {
						continue
					}
					parent = result.Candidates[selected]
				} else {
					continue
				}
			}

			// Cycle check: if branch is already an ancestor of the detected parent,
			// adopting it as a child would create a cycle.
			parentNode := tree.FindNode(root, parent)
			if parentNode != nil {
				ancestors := tree.GetAncestors(parentNode)
				if slices.ContainsFunc(ancestors, func(n *tree.Node) bool {
					return n.Name == branch
				}) {
					fmt.Printf("%s skipping %s: would create a cycle\n",
						s.WarningIcon(), s.Branch(branch))
					adopted[branch] = true
					continue
				}
			}

			// Commit adoption
			if setErr := cfg.SetParent(branch, parent); setErr != nil {
				fmt.Printf("%s failed to adopt %s: %v\n",
					s.WarningIcon(), s.Branch(branch), setErr)
				adopted[branch] = true
				continue
			}

			// Store fork point
			forkPoint, fpErr := g.GetMergeBase(branch, parent)
			if fpErr == nil {
				_ = cfg.SetForkPoint(branch, forkPoint) //nolint:errcheck // best effort
			}

			// Store PR number if detected via PR
			if result.PRNumber > 0 {
				_ = cfg.SetPR(branch, result.PRNumber) //nolint:errcheck // best effort
			}

			confidenceLabel := ""
			if result.Confidence == detect.High {
				confidenceLabel = " (via PR)"
			}
			fmt.Printf("%s Auto-adopted %s with parent %s%s\n",
				s.SuccessIcon(), s.Branch(branch), s.Branch(parent), confidenceLabel)

			// Update tracked list and rebuild tree for subsequent passes
			tracked = append(tracked, branch)
			adopted[branch] = true
			progress = true
			if newRoot, buildErr := tree.Build(cfg); buildErr == nil {
				root = newRoot
			}
		}

		if !progress {
			break
		}
	}

	// Print guidance for any branches that couldn't be resolved
	for _, branch := range candidates {
		if !adopted[branch] {
			fmt.Printf("%s could not determine parent for %s; run 'gh stack adopt <parent> --branch %s'\n",
				s.WarningIcon(), s.Branch(branch), branch)
		}
	}

	return nil
}
