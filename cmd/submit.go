// cmd/submit.go
package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/git"
	"github.com/boneskull/gh-stack/internal/github"
	"github.com/boneskull/gh-stack/internal/prompt"
	"github.com/boneskull/gh-stack/internal/state"
	"github.com/boneskull/gh-stack/internal/tree"
	"github.com/cli/go-gh/v2/pkg/browser"
	"github.com/spf13/cobra"
)

var submitCmd = &cobra.Command{
	Use:   "submit",
	Short: "Cascade, push, and create/update PRs for current branch and descendants",
	Long: `Submit rebases the current branch and its descendants onto their parents,
pushes all affected branches, and creates or updates pull requests.

This is the typical workflow command after making changes in a stack:
1. Cascade: rebase current branch + descendants onto their parents
2. Push: force-push all affected branches (with --force-with-lease)
3. PR: create PRs for branches without them, update PR bases for those that have them

If a rebase conflict occurs, resolve it and run 'gh stack continue'.`,
	RunE: runSubmit,
}

var (
	submitDryRunFlag      bool
	submitCurrentOnlyFlag bool
	submitUpdateOnlyFlag  bool
	submitYesFlag         bool
	submitWebFlag         bool
)

func init() {
	submitCmd.Flags().BoolVar(&submitDryRunFlag, "dry-run", false, "show what would be done without doing it")
	submitCmd.Flags().BoolVar(&submitCurrentOnlyFlag, "current-only", false, "only submit current branch, not descendants")
	submitCmd.Flags().BoolVar(&submitUpdateOnlyFlag, "update-only", false, "only update existing PRs, don't create new ones")
	submitCmd.Flags().BoolVarP(&submitYesFlag, "yes", "y", false, "skip interactive prompts and use auto-generated title/description for PRs")
	submitCmd.Flags().BoolVarP(&submitWebFlag, "web", "w", false, "open created/updated PRs in web browser")
	rootCmd.AddCommand(submitCmd)
}

func runSubmit(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	cfg, err := config.Load(cwd)
	if err != nil {
		return err
	}

	g := git.New(cwd)

	// Check if operation already in progress
	if state.Exists(g.GetGitDir()) {
		return fmt.Errorf("operation already in progress; use 'gh stack continue' or 'gh stack abort'")
	}

	currentBranch, err := g.CurrentBranch()
	if err != nil {
		return err
	}

	// Build tree
	root, err := tree.Build(cfg)
	if err != nil {
		return err
	}

	trunk, err := cfg.GetTrunk()
	if err != nil {
		return err
	}

	node := tree.FindNode(root, currentBranch)
	if node == nil {
		return fmt.Errorf("branch %q is not tracked in the stack\n\nTo add it, run:\n  gh stack adopt %s    # to stack on %s\n  gh stack adopt -p <parent>    # to stack on a different branch", currentBranch, trunk, trunk)
	}

	// Collect branches to submit (current + descendants, but never trunk)
	var branches []*tree.Node
	if currentBranch == trunk {
		// On trunk: only submit descendants, not trunk itself
		if submitCurrentOnlyFlag {
			return fmt.Errorf("cannot submit trunk branch %q; switch to a stack branch or remove --current-only", trunk)
		}
		branches = tree.GetDescendants(node)
		if len(branches) == 0 {
			return fmt.Errorf("no stack branches to submit; trunk %q has no descendants", trunk)
		}
	} else {
		// On a stack branch: submit it and optionally its descendants
		branches = append(branches, node)
		if !submitCurrentOnlyFlag {
			branches = append(branches, tree.GetDescendants(node)...)
		}
	}

	// Build the complete branch name list for state persistence
	branchNames := make([]string, len(branches))
	for i, b := range branches {
		branchNames[i] = b.Name
	}

	// Save undo snapshot (unless dry-run)
	var stashRef string
	if !submitDryRunFlag {
		var saveErr error
		stashRef, saveErr = saveUndoSnapshot(g, cfg, branches, nil, "submit", "gh stack submit")
		if saveErr != nil {
			fmt.Printf("Warning: could not save undo state: %v\n", saveErr)
		}
	}

	// Phase 1: Cascade
	fmt.Println("=== Phase 1: Cascade ===")
	if cascadeErr := doCascadeWithState(g, cfg, branches, submitDryRunFlag, state.OperationSubmit, submitUpdateOnlyFlag, submitWebFlag, branchNames, stashRef); cascadeErr != nil {
		// Stash is saved in state for conflicts; restore on other errors
		if cascadeErr != ErrConflict && stashRef != "" {
			fmt.Println("Restoring auto-stashed changes...")
			if popErr := g.StashPop(stashRef); popErr != nil {
				fmt.Printf("Warning: could not restore stashed changes (commit %s): %v\n", git.AbbrevSHA(stashRef), popErr)
			}
		}
		return cascadeErr
	}

	// Phases 2 & 3
	err = doSubmitPushAndPR(g, cfg, root, branches, submitDryRunFlag, submitUpdateOnlyFlag, submitWebFlag)

	// Restore auto-stashed changes after operation completes
	if stashRef != "" {
		fmt.Println("Restoring auto-stashed changes...")
		if popErr := g.StashPop(stashRef); popErr != nil {
			fmt.Printf("Warning: could not restore stashed changes (commit %s): %v\n", git.AbbrevSHA(stashRef), popErr)
		}
	}

	return err
}

// doSubmitPushAndPR handles push and PR creation/update phases.
// This is called after cascade succeeds (or from continue after conflict resolution).
func doSubmitPushAndPR(g *git.Git, cfg *config.Config, root *tree.Node, branches []*tree.Node, dryRun, updateOnly, openWeb bool) error {
	// Phase 2: Push all branches
	fmt.Println("\n=== Phase 2: Push ===")
	for _, b := range branches {
		if dryRun {
			fmt.Printf("Would push %s -> origin/%s (forced)\n", b.Name, b.Name)
		} else {
			fmt.Printf("Pushing %s -> origin/%s (forced)... ", b.Name, b.Name)
			if err := g.Push(b.Name, true); err != nil {
				fmt.Println("failed")
				return fmt.Errorf("failed to push %s: %w", b.Name, err)
			}
			fmt.Println("ok")
		}
	}

	// Phase 3: Create/update PRs
	return doSubmitPRs(g, cfg, root, branches, dryRun, updateOnly, openWeb)
}

// doSubmitPRs handles PR creation/update for all branches.
func doSubmitPRs(g *git.Git, cfg *config.Config, root *tree.Node, branches []*tree.Node, dryRun, updateOnly, openWeb bool) error {
	fmt.Println("\n=== Phase 3: PRs ===")

	trunk, err := cfg.GetTrunk()
	if err != nil {
		return err
	}

	// In dry-run mode, we don't need a GitHub client
	var ghClient *github.Client
	if !dryRun {
		var clientErr error
		ghClient, clientErr = github.NewClient()
		if clientErr != nil {
			return clientErr
		}
	}

	// Collect PR URLs for --web flag
	var prURLs []string

	for _, b := range branches {
		parent, _ := cfg.GetParent(b.Name) //nolint:errcheck // empty is fine
		if parent == "" {
			parent = trunk
		}

		existingPR, _ := cfg.GetPR(b.Name) //nolint:errcheck // 0 is fine

		if existingPR > 0 {
			// Update existing PR
			if dryRun {
				fmt.Printf("Would update PR #%d base to %q\n", existingPR, parent)
			} else {
				fmt.Printf("Updating PR #%d for %s (base: %s)... ", existingPR, b.Name, parent)
				if err := ghClient.UpdatePRBase(existingPR, parent); err != nil {
					fmt.Println("failed")
					fmt.Printf("Warning: failed to update PR #%d base: %v\n", existingPR, err)
				} else {
					fmt.Println("ok")
					if openWeb {
						prURLs = append(prURLs, ghClient.PRURL(existingPR))
					}
				}
				// Update stack comment
				if err := ghClient.GenerateAndPostStackComment(root, b.Name, trunk, existingPR); err != nil {
					fmt.Printf("Warning: failed to update stack comment for PR #%d: %v\n", existingPR, err)
				}
			}
		} else if !updateOnly {
			// Create new PR
			if dryRun {
				fmt.Printf("Would create PR for %s (base: %s)\n", b.Name, parent)
			} else {
				prNum, adopted, err := createPRForBranch(g, ghClient, cfg, root, b.Name, parent, trunk)
				if err != nil {
					fmt.Printf("Warning: failed to create PR for %s: %v\n", b.Name, err)
				} else if adopted {
					fmt.Printf("Adopted PR #%d for %s (%s)\n", prNum, b.Name, ghClient.PRURL(prNum))
					if openWeb {
						prURLs = append(prURLs, ghClient.PRURL(prNum))
					}
				} else {
					fmt.Printf("Created PR #%d for %s (%s)\n", prNum, b.Name, ghClient.PRURL(prNum))
					if openWeb {
						prURLs = append(prURLs, ghClient.PRURL(prNum))
					}
				}
			}
		} else {
			fmt.Printf("Skipping %s (no existing PR, --update-only)\n", b.Name)
		}
	}

	// Open PRs in browser if requested
	if openWeb && len(prURLs) > 0 {
		b := browser.New("", os.Stdout, os.Stderr)
		for _, url := range prURLs {
			if err := b.Browse(url); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not open browser for %s: %v\n", url, err)
			}
		}
	}

	return nil
}

// createPRForBranch creates a PR for the given branch and stores the PR number.
// If a PR already exists for the branch, it adopts the existing PR instead.
// Returns (prNumber, adopted, error) where adopted is true if we adopted an existing PR.
func createPRForBranch(g *git.Git, ghClient *github.Client, cfg *config.Config, root *tree.Node, branch, base, trunk string) (int, bool, error) {
	// Determine if draft (not targeting trunk = middle of stack)
	draft := base != trunk

	// Generate default title from branch name
	defaultTitle := generateTitleFromBranch(branch)

	// Generate PR body from commits
	defaultBody, bodyErr := generatePRBody(g, base, branch)
	if bodyErr != nil {
		// Non-fatal: just skip auto-body
		fmt.Printf("Warning: could not generate PR body: %v\n", bodyErr)
		defaultBody = ""
	}

	// Get title and body (prompt if interactive and --yes not set)
	title, body, err := promptForPRDetails(branch, defaultTitle, defaultBody)
	if err != nil {
		return 0, false, fmt.Errorf("failed to get PR details: %w", err)
	}

	pr, err := ghClient.CreateSubmitPR(branch, base, title, body, draft)
	if err != nil {
		// Check if PR already exists - if so, adopt it
		if strings.Contains(err.Error(), "pull request already exists") {
			prNum, adoptErr := adoptExistingPR(ghClient, cfg, root, branch, base, trunk)
			return prNum, true, adoptErr
		}
		return 0, false, err
	}

	// Store PR number in config
	if err := cfg.SetPR(branch, pr.Number); err != nil {
		return pr.Number, false, fmt.Errorf("PR created but failed to store number: %w", err)
	}

	// Update the tree node's PR number so stack comments render correctly
	if node := tree.FindNode(root, branch); node != nil {
		node.PR = pr.Number
	}

	// Add stack navigation comment
	if err := ghClient.GenerateAndPostStackComment(root, branch, trunk, pr.Number); err != nil {
		fmt.Printf("Warning: failed to add stack comment to PR #%d: %v\n", pr.Number, err)
	}

	return pr.Number, false, nil
}

// generateTitleFromBranch creates a PR title from a branch name.
// Replaces - and _ with spaces and converts to title case.
func generateTitleFromBranch(branch string) string {
	title := strings.ReplaceAll(branch, "-", " ")
	title = strings.ReplaceAll(title, "_", " ")
	return toTitleCase(title)
}

// toTitleCase converts a string to title case (first letter of each word capitalized).
func toTitleCase(s string) string {
	words := strings.Fields(s)
	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(word[:1]) + strings.ToLower(word[1:])
		}
	}
	return strings.Join(words, " ")
}

// promptForPRDetails prompts the user for PR title and body.
// If --yes flag is set or stdin is not a TTY, returns the defaults without prompting.
func promptForPRDetails(branch, defaultTitle, defaultBody string) (title, body string, err error) {
	// Skip prompts if --yes flag is set
	if submitYesFlag {
		return defaultTitle, defaultBody, nil
	}

	// Skip prompts if not interactive
	if !prompt.IsInteractive() {
		return defaultTitle, defaultBody, nil
	}

	fmt.Printf("\n--- Creating PR for %s (use --yes to skip prompts) ---\n", branch)

	// Prompt for title
	title, err = prompt.Input("PR title", defaultTitle)
	if err != nil {
		return "", "", err
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return "", "", fmt.Errorf("PR title cannot be empty")
	}

	// Show the generated body and ask if user wants to edit
	if defaultBody != "" {
		fmt.Println("\nGenerated PR description:")
		fmt.Println("---")
		// Show first few lines or truncate if too long
		lines := strings.Split(defaultBody, "\n")
		if len(lines) > 10 {
			for _, line := range lines[:10] {
				fmt.Println(line)
			}
			fmt.Printf("... (%d more lines)\n", len(lines)-10)
		} else {
			fmt.Println(defaultBody)
		}
		fmt.Println("---")
	}

	editBody, err := prompt.Confirm("Edit description in editor?", false)
	if err != nil {
		return "", "", err
	}

	if editBody {
		body, err = prompt.EditInEditor(defaultBody)
		if err != nil {
			fmt.Printf("Warning: editor failed, using generated description: %v\n", err)
			body = defaultBody
		}
	} else {
		body = defaultBody
	}

	fmt.Println()
	return title, body, nil
}

// adoptExistingPR finds an existing PR for the branch and adopts it into the stack.
func adoptExistingPR(ghClient *github.Client, cfg *config.Config, root *tree.Node, branch, base, trunk string) (int, error) {
	existingPR, err := ghClient.FindPRByHead(branch)
	if err != nil {
		return 0, fmt.Errorf("failed to find existing PR: %w", err)
	}
	if existingPR == nil {
		return 0, fmt.Errorf("PR creation failed but no existing PR found for branch %q", branch)
	}

	// Store PR number in config
	if err := cfg.SetPR(branch, existingPR.Number); err != nil {
		return existingPR.Number, fmt.Errorf("failed to store PR number: %w", err)
	}

	// Update the tree node's PR number so stack comments render correctly
	if node := tree.FindNode(root, branch); node != nil {
		node.PR = existingPR.Number
	}

	// Update PR base to match stack parent
	if existingPR.Base.Ref != base {
		if err := ghClient.UpdatePRBase(existingPR.Number, base); err != nil {
			fmt.Printf("Warning: failed to update base: %v\n", err)
		}
	}

	// Add/update stack navigation comment
	if err := ghClient.GenerateAndPostStackComment(root, branch, trunk, existingPR.Number); err != nil {
		fmt.Printf("Warning: failed to update stack comment: %v\n", err)
	}

	return existingPR.Number, nil
}

// generatePRBody creates a PR description from the commits between base and head.
// For a single commit: returns the commit body.
// For multiple commits: returns each commit as a markdown section.
func generatePRBody(g *git.Git, base, head string) (string, error) {
	commits, err := g.GetCommits(base, head)
	if err != nil {
		return "", err
	}

	if len(commits) == 0 {
		return "", nil
	}

	if len(commits) == 1 {
		// Single commit: just use the body
		return commits[0].Body, nil
	}

	// Multiple commits: format as markdown sections
	var sb strings.Builder
	for i, commit := range commits {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("### ")
		sb.WriteString(commit.Subject)
		sb.WriteString("\n")
		if commit.Body != "" {
			sb.WriteString("\n")
			sb.WriteString(commit.Body)
			sb.WriteString("\n")
		}
	}

	return sb.String(), nil
}
