// cmd/doctor.go
package cmd

import (
	"fmt"
	"os"
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

	var issueCount int
	for _, branch := range branches {
		result := checkBranch(g, cfg, s, branch, doctorFixFlag)
		if !printResult(s, result) {
			issueCount++
		}
	}

	fmt.Println()
	printSummary(s, issueCount, doctorFixFlag)

	return nil
}

// printResult prints the outcome of a single branch check.
// Returns true if the branch is healthy (or was fixed), false if issues remain.
func printResult(s *style.Style, r branchResult) bool {
	if r.healthy {
		fmt.Printf("%s %s %s\n", s.SuccessIcon(), s.Branch(r.name), s.Muted("(healthy)"))
		return true
	}
	if r.fixed {
		fmt.Printf("%s %s: %s\n", s.SuccessIcon(), s.Branch(r.name), r.fixMsg)
		return true
	}
	fmt.Printf("%s %s\n", s.FailureIcon(), s.Branch(r.name))
	for _, issue := range r.issues {
		fmt.Printf("    %s\n", issue)
	}
	return false
}

// printSummary prints the final summary line after all branches have been checked.
func printSummary(s *style.Style, issueCount int, fix bool) {
	if issueCount == 0 {
		fmt.Println(s.SuccessMessage("All branches healthy."))
		return
	}
	noun := "issue"
	if issueCount > 1 {
		noun = "issues"
	}
	fmt.Printf("%d %s found.", issueCount, noun)
	if !fix {
		fmt.Printf(" Run 'gh stack doctor --fix' to repair.")
	}
	fmt.Println()
}

func checkBranch(g *git.Git, cfg *config.Config, s *style.Style, branch string, fix bool) branchResult {
	result := branchResult{name: branch}

	issues := health.CheckBranch(g, cfg, branch)
	if len(issues) == 0 {
		result.healthy = true
		return result
	}

	if !fix {
		result.issues = formatIssues(s, issues)
		return result
	}

	return fixBranch(g, cfg, s, branch, issues)
}

// formatIssues converts health issues into styled display strings.
func formatIssues(s *style.Style, issues []health.Issue) []string {
	out := make([]string, 0, len(issues))
	for _, iss := range issues {
		switch iss.Kind {
		case health.KindBranchMissing, health.KindParentMissing, health.KindNoForkPoint:
			out = append(out, s.Error(iss.Message))
		default:
			out = append(out, iss.Message)
		}
	}
	return out
}

// fixBranch attempts to repair the first issue found by health.CheckBranch.
func fixBranch(g *git.Git, cfg *config.Config, s *style.Style, branch string, issues []health.Issue) branchResult {
	result := branchResult{name: branch}

	kind := issues[0].Kind
	if !issues[0].Fixable && kind != health.KindDrift {
		result.issues = formatIssues(s, issues)
		return result
	}

	parent, _ := cfg.GetParent(branch)     //nolint:errcheck // health check validated parent exists
	storedFP, _ := cfg.GetForkPoint(branch) //nolint:errcheck // may be empty for KindNoForkPoint

	switch kind {
	case health.KindNoForkPoint:
		return fixMissingForkPoint(g, cfg, branch, parent)
	case health.KindForkPointMissing, health.KindForkPointNotAncestor:
		return fixStaleForkPoint(g, cfg, s, branch, parent, storedFP, issues[0].Message)
	case health.KindDrift:
		return fixDrift(g, cfg, s, branch, parent, storedFP)
	default:
		for _, iss := range issues {
			result.issues = append(result.issues, iss.Message)
		}
		return result
	}
}

// fixMissingForkPoint computes and sets a fork point when none is stored.
func fixMissingForkPoint(g *git.Git, cfg *config.Config, branch, parent string) branchResult {
	result := branchResult{name: branch}
	newFP, err := computeForkPoint(g, parent, branch)
	if err != nil {
		result.issues = append(result.issues, fmt.Sprintf("No fork point stored and could not compute one: %v", err))
		return result
	}
	if err := cfg.SetForkPoint(branch, newFP); err != nil {
		result.issues = append(result.issues, fmt.Sprintf("Failed to set fork point: %v", err))
		return result
	}
	result.fixed = true
	result.fixMsg = fmt.Sprintf("set fork point to %s", git.AbbrevSHA(newFP))
	return result
}

// fixStaleForkPoint recomputes and replaces a fork point that is missing or not an ancestor.
func fixStaleForkPoint(g *git.Git, cfg *config.Config, s *style.Style, branch, parent, storedFP, context string) branchResult {
	result := branchResult{name: branch}
	newFP, err := computeForkPoint(g, parent, branch)
	if err != nil {
		result.issues = append(result.issues, fmt.Sprintf("%s and could not compute replacement: %v", context, err))
		return result
	}
	if err := setForkPointWithComment(s, cfg, branch, storedFP, newFP); err != nil {
		result.issues = append(result.issues, fmt.Sprintf("Failed to set fork point: %v", err))
		return result
	}
	result.fixed = true
	result.fixMsg = fmt.Sprintf("updated fork point %s \u2192 %s", git.AbbrevSHA(storedFP), git.AbbrevSHA(newFP))
	return result
}

// fixDrift replaces a stored fork point that doesn't match the computed merge-base.
func fixDrift(g *git.Git, cfg *config.Config, s *style.Style, branch, parent, storedFP string) branchResult {
	result := branchResult{name: branch}
	mergeBase, err := g.GetMergeBase(parent, branch)
	if err != nil {
		result.issues = append(result.issues, fmt.Sprintf("Failed to compute merge-base for fix: %v", err))
		return result
	}
	if err := setForkPointWithComment(s, cfg, branch, storedFP, mergeBase); err != nil {
		result.issues = append(result.issues, fmt.Sprintf("Failed to set fork point: %v", err))
		return result
	}
	result.fixed = true
	result.fixMsg = fmt.Sprintf("updated fork point %s \u2192 %s", git.AbbrevSHA(storedFP), git.AbbrevSHA(mergeBase))
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

// setForkPointWithComment updates the fork point and records the old value
// as an inline git config comment for provenance. Falls back to a plain
// SetForkPoint if the git version doesn't support --comment (requires 2.45+).
func setForkPointWithComment(s *style.Style, cfg *config.Config, branch, oldSHA, newSHA string) error {
	comment := fmt.Sprintf("replaces %s (%s)",
		git.AbbrevSHA(oldSHA),
		time.Now().UTC().Format(time.RFC3339),
	)
	err := cfg.SetForkPointWithComment(branch, newSHA, comment)
	if err == nil {
		return nil
	}

	// --comment likely unsupported; fall back to plain set and warn once.
	if setErr := cfg.SetForkPoint(branch, newSHA); setErr != nil {
		return setErr
	}
	warnOldGit(s)
	return nil
}

var oldGitWarned bool

// warnOldGit prints a one-time warning that the user's git version is too old
// for inline config comments.
func warnOldGit(s *style.Style) {
	if oldGitWarned {
		return
	}
	oldGitWarned = true
	fmt.Printf("%s git config --comment not supported; fix provenance will be missing.\n", s.WarningIcon())
	fmt.Println(s.Muted("  gh-stack works best with git 2.45 or newer."))
}
