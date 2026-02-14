// cmd/doctor.go
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/git"
	"github.com/boneskull/gh-stack/internal/style"
	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check stack health and detect stale fork points",
	Long:  `Diagnose (and optionally repair) stale fork points caused by manual rebases or resets.`,
	RunE:  runDoctor,
}

var doctorFixFlag bool

func init() {
	doctorCmd.Flags().BoolVar(&doctorFixFlag, "fix", false, "repair detected issues")
	rootCmd.AddCommand(doctorCmd)
}

// branchResult tracks the outcome of checking a single branch.
type branchResult struct {
	name    string
	healthy bool
	issues  []string
	fixed   bool
	fixMsg  string
}

func runDoctor(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	cfg, err := config.Load(cwd)
	if err != nil {
		return err
	}

	if _, err := cfg.GetTrunk(); err != nil {
		return err
	}

	g := git.New(cwd)
	s := style.New()

	branches, err := cfg.ListTrackedBranches()
	if err != nil {
		return err
	}

	if len(branches) == 0 {
		fmt.Println("No tracked branches found.")
		return nil
	}

	fmt.Println(s.Bold("Stack Health Report"))
	fmt.Println()

	var healthy, issues int
	for _, branch := range branches {
		result := checkBranch(g, cfg, s, branch, doctorFixFlag)
		if result.healthy {
			fmt.Printf("%s %s %s\n", s.SuccessIcon(), s.Branch(result.name), s.Muted("(healthy)"))
			healthy++
		} else if result.fixed {
			fmt.Printf("%s %s: %s\n", s.SuccessIcon(), s.Branch(result.name), result.fixMsg)
			healthy++
		} else {
			fmt.Printf("%s %s\n", s.FailureIcon(), s.Branch(result.name))
			for _, issue := range result.issues {
				fmt.Printf("    %s\n", issue)
			}
			issues++
		}
	}

	fmt.Println()
	if issues > 0 {
		noun := "issue"
		if issues > 1 {
			noun = "issues"
		}
		fmt.Printf("%d %s found.", issues, noun)
		if !doctorFixFlag {
			fmt.Printf(" Run 'gh stack doctor --fix' to repair.")
		}
		fmt.Println()
	} else {
		fmt.Println(s.SuccessMessage("All branches healthy."))
	}

	return nil
}

func checkBranch(g *git.Git, cfg *config.Config, s *style.Style, branch string, fix bool) branchResult {
	result := branchResult{name: branch}

	// Check 1: branch exists locally
	if !g.BranchExists(branch) {
		result.issues = append(result.issues, s.Error("Branch does not exist locally"))
		return result
	}

	// Check 2: parent exists locally
	parent, err := cfg.GetParent(branch)
	if err != nil {
		result.issues = append(result.issues, s.Error("No parent configured"))
		return result
	}

	trunk, _ := cfg.GetTrunk() //nolint:errcheck // already validated in runDoctor
	if parent != trunk && !g.BranchExists(parent) {
		result.issues = append(result.issues, s.Errorf("Parent branch %s does not exist locally", parent))
		return result
	}

	// Check 3: fork point is stored
	storedFP, err := cfg.GetForkPoint(branch)
	if err != nil {
		if fix {
			newFP, fixErr := computeForkPoint(g, parent, branch)
			if fixErr != nil {
				result.issues = append(result.issues, fmt.Sprintf("No fork point stored and could not compute one: %v", fixErr))
				return result
			}
			if setErr := cfg.SetForkPoint(branch, newFP); setErr != nil {
				result.issues = append(result.issues, fmt.Sprintf("Failed to set fork point: %v", setErr))
				return result
			}
			result.fixed = true
			result.fixMsg = fmt.Sprintf("set fork point to %s", git.AbbrevSHA(newFP))
			return result
		}
		result.issues = append(result.issues, "No fork point stored")
		return result
	}

	// Check 4: fork point commit exists
	if !g.CommitExists(storedFP) {
		if fix {
			newFP, fixErr := computeForkPoint(g, parent, branch)
			if fixErr != nil {
				result.issues = append(result.issues, fmt.Sprintf("Stored fork point %s does not exist and could not compute replacement: %v", git.AbbrevSHA(storedFP), fixErr))
				return result
			}
			if setErr := setForkPointWithComment(g, cfg, branch, storedFP, newFP); setErr != nil {
				result.issues = append(result.issues, fmt.Sprintf("Failed to set fork point: %v", setErr))
				return result
			}
			result.fixed = true
			result.fixMsg = fmt.Sprintf("updated fork point %s \u2192 %s", git.AbbrevSHA(storedFP), git.AbbrevSHA(newFP))
			return result
		}
		result.issues = append(result.issues, fmt.Sprintf("Stored fork point %s does not exist", git.AbbrevSHA(storedFP)))
		return result
	}

	// Check 5: fork point is ancestor of branch
	isAnc, err := g.IsAncestor(storedFP, branch)
	if err != nil {
		result.issues = append(result.issues, fmt.Sprintf("Failed to check if stored fork point %s is an ancestor of %s: %v", git.AbbrevSHA(storedFP), branch, err))
		return result
	}
	if !isAnc {
		if fix {
			newFP, fixErr := computeForkPoint(g, parent, branch)
			if fixErr != nil {
				result.issues = append(result.issues, fmt.Sprintf("Stored fork point %s is not an ancestor of %s and could not compute replacement: %v", git.AbbrevSHA(storedFP), branch, fixErr))
				return result
			}
			if setErr := setForkPointWithComment(g, cfg, branch, storedFP, newFP); setErr != nil {
				result.issues = append(result.issues, fmt.Sprintf("Failed to set fork point: %v", setErr))
				return result
			}
			result.fixed = true
			result.fixMsg = fmt.Sprintf("updated fork point %s \u2192 %s", git.AbbrevSHA(storedFP), git.AbbrevSHA(newFP))
			return result
		}
		result.issues = append(result.issues, fmt.Sprintf("Stored fork point %s is not an ancestor of %s", git.AbbrevSHA(storedFP), branch))
		return result
	}

	// Check 6: drift detection
	mergeBase, err := g.GetMergeBase(parent, branch)
	if err != nil {
		// Can't compute merge-base; skip drift check
		result.healthy = true
		return result
	}

	if storedFP != mergeBase {
		if fix {
			if setErr := setForkPointWithComment(g, cfg, branch, storedFP, mergeBase); setErr != nil {
				result.issues = append(result.issues, fmt.Sprintf("Failed to set fork point: %v", setErr))
				return result
			}
			result.fixed = true
			result.fixMsg = fmt.Sprintf("updated fork point %s \u2192 %s", git.AbbrevSHA(storedFP), git.AbbrevSHA(mergeBase))
			return result
		}
		result.issues = append(result.issues,
			fmt.Sprintf("Stored parent: %s", parent),
			fmt.Sprintf("Stored fork:   %s", git.AbbrevSHA(storedFP)),
			fmt.Sprintf("Actual fork:   %s", git.AbbrevSHA(mergeBase)),
			"Drift detected: stored fork point does not match merge-base",
		)
		return result
	}

	result.healthy = true
	return result
}

// computeForkPoint tries the reflog-aware fork-point first, falling back to merge-base.
func computeForkPoint(g *git.Git, parent, branch string) (string, error) {
	fp, err := g.GetForkPoint(parent, branch)
	if err == nil {
		return fp, nil
	}
	return g.GetMergeBase(parent, branch)
}

// setForkPointWithComment updates the fork point and inserts a comment preserving the old value.
// The comment is best-effort; if it fails, the fork point is still updated.
func setForkPointWithComment(g *git.Git, cfg *config.Config, branch, oldSHA, newSHA string) error {
	if err := cfg.SetForkPoint(branch, newSHA); err != nil {
		return err
	}
	commentForkPointChange(g.GetGitDir(), branch, oldSHA)
	return nil
}

// commentForkPointChange inserts a comment above the stackForkPoint line in .git/config
// recording the previous value and a timestamp. This preserves provenance so old fork
// points can be recovered if a fix goes wrong. Best-effort: errors are silently ignored.
func commentForkPointChange(gitDir, branch, oldSHA string) {
	configPath := filepath.Join(gitDir, "config")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return
	}

	lines := strings.Split(string(data), "\n")
	sectionHeader := fmt.Sprintf("[branch %q]", branch)
	inSection := false
	var result []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if trimmed == sectionHeader {
			inSection = true
			result = append(result, line)
			continue
		}

		// New section starts; we've left the target section
		if inSection && strings.HasPrefix(trimmed, "[") {
			inSection = false
		}

		// Insert comment before the stackForkPoint line
		if inSection && strings.HasPrefix(strings.ToLower(trimmed), "stackforkpoint") {
			indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			timestamp := time.Now().UTC().Format(time.RFC3339)
			result = append(result,
				fmt.Sprintf("%s# doctor fix %s replaces previous value of:", indent, timestamp),
				fmt.Sprintf("%s# %s", indent, oldSHA),
			)
		}

		result = append(result, line)
	}

	//nolint:errcheck // best-effort provenance
	_ = os.WriteFile(configPath, []byte(strings.Join(result, "\n")), 0644)
}
