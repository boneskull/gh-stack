// cmd/doctor.go
package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/git"
	"github.com/boneskull/gh-stack/internal/health"
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

	issues := health.CheckBranch(g, cfg, branch)
	if len(issues) == 0 {
		result.healthy = true
		return result
	}

	// If not fixing, format issues with styling and return.
	if !fix {
		for _, iss := range issues {
			switch iss.Kind {
			case health.KindBranchMissing, health.KindParentMissing, health.KindNoForkPoint:
				result.issues = append(result.issues, s.Error(iss.Message))
			default:
				result.issues = append(result.issues, iss.Message)
			}
		}
		return result
	}

	// Attempt to fix: dispatch based on the issue kind.
	kind := issues[0].Kind
	if !issues[0].Fixable && kind != health.KindDrift {
		// Unfixable issues (missing branch, missing parent, check failures)
		for _, iss := range issues {
			result.issues = append(result.issues, s.Error(iss.Message))
		}
		return result
	}

	parent, _ := cfg.GetParent(branch)     //nolint:errcheck // health check validated parent exists
	storedFP, _ := cfg.GetForkPoint(branch) //nolint:errcheck // may be empty for KindNoForkPoint

	switch kind {
	case health.KindNoForkPoint:
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

	case health.KindForkPointMissing:
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

	case health.KindForkPointNotAncestor:
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

	case health.KindDrift:
		mergeBase, err := g.GetMergeBase(parent, branch)
		if err != nil {
			result.issues = append(result.issues, fmt.Sprintf("Failed to compute merge-base for fix: %v", err))
			return result
		}
		if setErr := setForkPointWithComment(g, cfg, branch, storedFP, mergeBase); setErr != nil {
			result.issues = append(result.issues, fmt.Sprintf("Failed to set fork point: %v", setErr))
			return result
		}
		result.fixed = true
		result.fixMsg = fmt.Sprintf("updated fork point %s \u2192 %s", git.AbbrevSHA(storedFP), git.AbbrevSHA(mergeBase))

	default:
		// Shouldn't happen, but surface issues if it does
		for _, iss := range issues {
			result.issues = append(result.issues, iss.Message)
		}
	}

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
	commentForkPointChange(g, branch, oldSHA)
	return nil
}

// commentForkPointChange inserts a comment above the stackForkPoint line in the git config
// recording the previous value and a timestamp. This preserves provenance so old fork
// points can be recovered if a fix goes wrong. Best-effort: errors are silently ignored.
func commentForkPointChange(g *git.Git, branch, oldSHA string) {
	configPath, err := g.GetConfigPath()
	if err != nil {
		return
	}
	info, err := os.Stat(configPath)
	if err != nil {
		return
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return
	}

	lines := strings.Split(string(data), "\n")
	sectionHeader := fmt.Sprintf("[branch %q]", branch)
	inSection := false
	modified := false
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
			modified = true
		}

		result = append(result, line)
	}

	if !modified {
		return
	}
	//nolint:errcheck // best-effort provenance
	_ = os.WriteFile(configPath, []byte(strings.Join(result, "\n")), info.Mode())
}
