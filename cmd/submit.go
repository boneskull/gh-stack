// cmd/submit.go
package cmd

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/git"
	"github.com/boneskull/gh-stack/internal/github"
	"github.com/boneskull/gh-stack/internal/prompt"
	"github.com/boneskull/gh-stack/internal/state"
	"github.com/boneskull/gh-stack/internal/style"
	"github.com/boneskull/gh-stack/internal/tree"
	"github.com/cli/go-gh/v2/pkg/browser"
	"github.com/spf13/cobra"
)

var submitCmd = &cobra.Command{
	Use:   "submit",
	Short: "Restack, push, and create/update PRs for current branch and descendants",
	Long: `Submit rebases the current branch and its descendants onto their parents,
pushes all affected branches, and creates or updates pull requests.

This is the typical workflow command after making changes in a stack:
1. Restack: rebase current branch + descendants onto their parents
2. Push: force-push all affected branches (with --force-with-lease)
3. PR: create PRs for branches without them, update PR bases for those that have them

If a rebase conflict occurs, resolve it and run 'gh stack continue'.`,
	RunE: runSubmit,
}

var (
	submitDryRunFlag      bool
	submitCurrentOnlyFlag bool
	submitUpdateOnlyFlag  bool
	submitPushOnlyFlag    bool
	submitYesFlag         bool
	submitWebFlag         bool
)

func init() {
	submitCmd.Flags().BoolVar(&submitDryRunFlag, "dry-run", false, "show what would be done without doing it")
	submitCmd.Flags().BoolVar(&submitCurrentOnlyFlag, "current-only", false, "only submit current branch, not descendants")
	submitCmd.Flags().BoolVar(&submitUpdateOnlyFlag, "update-only", false, "only update existing PRs, don't create new ones")
	submitCmd.Flags().BoolVar(&submitPushOnlyFlag, "push-only", false, "skip PR creation/update, only restack and push")
	submitCmd.Flags().BoolVarP(&submitYesFlag, "yes", "y", false, "skip interactive prompts and use auto-generated title/description for PRs")
	submitCmd.Flags().BoolVarP(&submitWebFlag, "web", "w", false, "open created/updated PRs in web browser")
	rootCmd.AddCommand(submitCmd)
}

func runSubmit(cmd *cobra.Command, args []string) error {
	s := style.New()

	// Validate flag combinations
	if submitPushOnlyFlag && submitUpdateOnlyFlag {
		return fmt.Errorf("--push-only and --update-only cannot be used together: --push-only skips all PR operations")
	}
	if submitPushOnlyFlag && submitWebFlag {
		return fmt.Errorf("--push-only and --web cannot be used together: --push-only skips all PR operations")
	}

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
		stashRef, saveErr = saveUndoSnapshot(g, cfg, branches, nil, "submit", "gh stack submit", s)
		if saveErr != nil {
			fmt.Printf("%s could not save undo state: %v\n", s.WarningIcon(), saveErr)
		}
	}

	// Phase 1: Restack
	fmt.Println(s.Bold("=== Phase 1: Restack ==="))
	if cascadeErr := doCascadeWithState(g, cfg, branches, submitDryRunFlag, state.OperationSubmit, submitUpdateOnlyFlag, submitWebFlag, submitPushOnlyFlag, branchNames, stashRef, s); cascadeErr != nil {
		// Stash is saved in state for conflicts; restore on other errors
		if cascadeErr != ErrConflict && stashRef != "" {
			fmt.Println("Restoring auto-stashed changes...")
			if popErr := g.StashPop(stashRef); popErr != nil {
				fmt.Printf("%s could not restore stashed changes (commit %s): %v\n", s.WarningIcon(), git.AbbrevSHA(stashRef), popErr)
			}
		}
		return cascadeErr
	}

	// Phases 2 & 3
	err = doSubmitPushAndPR(g, cfg, root, branches, submitDryRunFlag, submitUpdateOnlyFlag, submitWebFlag, submitPushOnlyFlag, s)

	// Restore auto-stashed changes after operation completes
	if stashRef != "" {
		fmt.Println("Restoring auto-stashed changes...")
		if popErr := g.StashPop(stashRef); popErr != nil {
			fmt.Printf("%s could not restore stashed changes (commit %s): %v\n", s.WarningIcon(), git.AbbrevSHA(stashRef), popErr)
		}
	}

	return err
}

// doSubmitPushAndPR handles push and PR creation/update phases.
// This is called after cascade succeeds (or from continue after conflict resolution).
func doSubmitPushAndPR(g *git.Git, cfg *config.Config, root *tree.Node, branches []*tree.Node, dryRun, updateOnly, openWeb, pushOnly bool, s *style.Style) error {
	// Phase 2: Push all branches
	fmt.Println(s.Bold("\n=== Phase 2: Push ==="))
	for _, b := range branches {
		if dryRun {
			fmt.Printf("%s Would push %s -> origin/%s (forced)\n", s.Muted("dry-run:"), s.Branch(b.Name), s.Branch(b.Name))
		} else {
			fmt.Printf("Pushing %s -> origin/%s (forced)... ", s.Branch(b.Name), s.Branch(b.Name))
			if err := g.Push(b.Name, true); err != nil {
				fmt.Println(s.Error("failed"))
				return fmt.Errorf("failed to push %s: %w", b.Name, err)
			}
			fmt.Println(s.Success("ok"))
		}
	}

	// Phase 3: Create/update PRs
	if pushOnly {
		fmt.Println(s.Bold("\n=== Phase 3: PRs ==="))
		fmt.Println(s.Muted("Skipped (--push-only)"))
		return nil
	}
	return doSubmitPRs(g, cfg, root, branches, dryRun, updateOnly, openWeb, s)
}

// doSubmitPRs handles PR creation/update for all branches.
func doSubmitPRs(g *git.Git, cfg *config.Config, root *tree.Node, branches []*tree.Node, dryRun, updateOnly, openWeb bool, s *style.Style) error {
	fmt.Println(s.Bold("\n=== Phase 3: PRs ==="))

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
				fmt.Printf("%s Would update PR #%d base to %s\n", s.Muted("dry-run:"), existingPR, s.Branch(parent))
			} else {
				fmt.Printf("Updating PR #%d for %s (base: %s)... ", existingPR, s.Branch(b.Name), s.Branch(parent))
				if err := ghClient.UpdatePRBase(existingPR, parent); err != nil {
					fmt.Println(s.Error("failed"))
					fmt.Printf("%s failed to update PR #%d base: %v\n", s.WarningIcon(), existingPR, err)
				} else {
					fmt.Println(s.Success("ok"))
					if openWeb {
						prURLs = append(prURLs, ghClient.PRURL(existingPR))
					}
				}
				// Update stack comment
				if err := ghClient.GenerateAndPostStackComment(root, b.Name, trunk, existingPR); err != nil {
					fmt.Printf("%s failed to update stack comment for PR #%d: %v\n", s.WarningIcon(), existingPR, err)
				}

				// If PR is a draft and now targets trunk, offer to publish
				maybeMarkPRReady(ghClient, existingPR, b.Name, parent, trunk, s)
			}
		} else if !updateOnly {
			// Create new PR
			if dryRun {
				fmt.Printf("%s Would create PR for %s (base: %s)\n", s.Muted("dry-run:"), s.Branch(b.Name), s.Branch(parent))
			} else {
				prNum, adopted, err := createPRForBranch(g, ghClient, cfg, root, b.Name, parent, trunk, s)
				if err != nil {
					fmt.Printf("%s failed to create PR for %s: %v\n", s.WarningIcon(), s.Branch(b.Name), err)
				} else if adopted {
					fmt.Printf("%s Adopted PR #%d for %s (%s)\n", s.SuccessIcon(), prNum, s.Branch(b.Name), ghClient.PRURL(prNum))
					if openWeb {
						prURLs = append(prURLs, ghClient.PRURL(prNum))
					}
				} else {
					fmt.Printf("%s Created PR #%d for %s (%s)\n", s.SuccessIcon(), prNum, s.Branch(b.Name), ghClient.PRURL(prNum))
					if openWeb {
						prURLs = append(prURLs, ghClient.PRURL(prNum))
					}
				}
			}
		} else {
			fmt.Printf("Skipping %s %s\n", s.Branch(b.Name), s.Muted("(no existing PR, --update-only)"))
		}
	}

	// Open PRs in browser if requested
	if openWeb && len(prURLs) > 0 {
		b := browser.New("", os.Stdout, os.Stderr)
		for _, url := range prURLs {
			if err := b.Browse(url); err != nil {
				fmt.Fprintf(os.Stderr, "%s could not open browser for %s: %v\n", s.WarningIcon(), url, err)
			}
		}
	}

	return nil
}

// createPRForBranch creates a PR for the given branch and stores the PR number.
// If a PR already exists for the branch, it adopts the existing PR instead.
// Returns (prNumber, adopted, error) where adopted is true if we adopted an existing PR.
func createPRForBranch(g *git.Git, ghClient *github.Client, cfg *config.Config, root *tree.Node, branch, base, trunk string, s *style.Style) (int, bool, error) {
	// Determine if draft (not targeting trunk = middle of stack)
	draft := base != trunk

	// Generate default title from first commit message (falls back to branch name)
	defaultTitle := generateDefaultTitle(g, base, branch)

	// Generate PR body from commits
	defaultBody, bodyErr := generatePRBody(g, base, branch)
	if bodyErr != nil {
		// Non-fatal: just skip auto-body
		fmt.Printf("%s could not generate PR body: %v\n", s.WarningIcon(), bodyErr)
		defaultBody = ""
	}

	// Get title and body (prompt if interactive and --yes not set)
	title, body, err := promptForPRDetails(branch, defaultTitle, defaultBody, s)
	if err != nil {
		return 0, false, fmt.Errorf("failed to get PR details: %w", err)
	}

	pr, err := ghClient.CreateSubmitPR(branch, base, title, body, draft)
	if err != nil {
		// Check if PR already exists - if so, adopt it
		if strings.Contains(err.Error(), "pull request already exists") {
			prNum, adoptErr := adoptExistingPR(ghClient, cfg, root, branch, base, trunk, s)
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
		fmt.Printf("%s failed to add stack comment to PR #%d: %v\n", s.WarningIcon(), pr.Number, err)
	}

	return pr.Number, false, nil
}

// generateDefaultTitle creates a PR title from the first commit message.
// Falls back to branch name if no commits are available.
func generateDefaultTitle(g *git.Git, base, branch string) string {
	commits, err := g.GetCommits(base, branch)
	if err != nil || len(commits) == 0 {
		// Fall back to branch name
		return generateTitleFromBranch(branch)
	}
	// Use the first commit's subject.
	// NOTE: GetCommits is defined to return commits in reverse chronological order (newest first),
	// so the oldest commit in the range [base..branch] is the last element of the slice.
	// See internal/git.GetCommits for the documented ordering guarantee.
	// Trim whitespace to avoid malformed PR titles.
	subject := strings.TrimSpace(commits[len(commits)-1].Subject)
	if subject == "" {
		// If the subject is empty or whitespace-only, fall back to branch name
		return generateTitleFromBranch(branch)
	}
	return subject
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
func promptForPRDetails(branch, defaultTitle, defaultBody string, s *style.Style) (title, body string, err error) {
	// Skip prompts if --yes flag is set
	if submitYesFlag {
		return defaultTitle, defaultBody, nil
	}

	// Skip prompts if not interactive
	if !prompt.IsInteractive() {
		return defaultTitle, defaultBody, nil
	}

	fmt.Printf("\n--- Creating PR for %s %s ---\n", s.Branch(branch), s.Muted("(use --yes to skip prompts)"))

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
		fmt.Println(s.Muted("\nGenerated PR description:"))
		fmt.Println(s.Muted("---"))
		// Show first few lines or truncate if too long
		lines := strings.Split(defaultBody, "\n")
		if len(lines) > 10 {
			for _, line := range lines[:10] {
				fmt.Println(line)
			}
			fmt.Printf(s.Muted("... (%d more lines)\n"), len(lines)-10)
		} else {
			fmt.Println(defaultBody)
		}
		fmt.Println(s.Muted("---"))
	}

	editBody, err := prompt.Confirm("Edit description in editor?", false)
	if err != nil {
		return "", "", err
	}

	if editBody {
		body, err = prompt.EditInEditor(defaultBody)
		if err != nil {
			fmt.Printf("%s editor failed, using generated description: %v\n", s.WarningIcon(), err)
			body = defaultBody
		}
	} else {
		body = defaultBody
	}

	fmt.Println()
	return title, body, nil
}

// adoptExistingPR finds an existing PR for the branch and adopts it into the stack.
func adoptExistingPR(ghClient *github.Client, cfg *config.Config, root *tree.Node, branch, base, trunk string, s *style.Style) (int, error) {
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
			fmt.Printf("%s failed to update base: %v\n", s.WarningIcon(), err)
		}
	}

	// Add/update stack navigation comment
	if err := ghClient.GenerateAndPostStackComment(root, branch, trunk, existingPR.Number); err != nil {
		fmt.Printf("%s failed to update stack comment: %v\n", s.WarningIcon(), err)
	}

	// If adopted PR is a draft and targets trunk, offer to publish
	if existingPR.Draft && base == trunk {
		promptMarkPRReady(ghClient, existingPR.Number, branch, trunk, s)
	}

	return existingPR.Number, nil
}

// maybeMarkPRReady checks if a PR is a draft targeting trunk and offers to publish it.
// This handles the case where a PR was created as a draft (middle of stack) but now
// targets trunk because its parent was merged.
func maybeMarkPRReady(ghClient *github.Client, prNumber int, branch, base, trunk string, s *style.Style) {
	// Only relevant if PR now targets trunk
	if base != trunk {
		return
	}

	// Check if PR is a draft
	pr, err := ghClient.GetPR(prNumber)
	if err != nil || !pr.Draft {
		return
	}

	promptMarkPRReady(ghClient, prNumber, branch, trunk, s)
}

// promptMarkPRReady prompts to publish a draft PR and marks it ready if confirmed.
// Called when we already know the PR is a draft targeting trunk.
func promptMarkPRReady(ghClient *github.Client, prNumber int, branch, trunk string, s *style.Style) {
	fmt.Printf("PR #%d (%s) is a draft and now targets %s.\n", prNumber, s.Branch(branch), s.Branch(trunk))

	// Skip prompt if --yes flag is set or non-interactive
	shouldMarkReady := true
	if !submitYesFlag && prompt.IsInteractive() {
		shouldMarkReady, _ = prompt.Confirm("Mark as ready for review?", true) //nolint:errcheck // default is fine
	}

	if shouldMarkReady {
		if readyErr := ghClient.MarkPRReady(prNumber); readyErr != nil {
			fmt.Printf("%s failed to mark PR ready: %v\n", s.WarningIcon(), readyErr)
		} else {
			fmt.Printf("%s PR #%d marked as ready for review.\n", s.SuccessIcon(), prNumber)
		}
	}
}

// generatePRBody creates a PR description from the commits between base and head.
// For a single commit: returns the commit body.
// For multiple commits: returns each commit as a markdown section.
//
// Commit message bodies are unwrapped so that hard line breaks within paragraphs
// (typical of the ~72-column git convention) are removed. This produces better
// rendering in GitHub's PR description, which treats single newlines as <br> tags.
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
		return unwrapParagraphs(commits[0].Body), nil
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
			sb.WriteString(unwrapParagraphs(commit.Body))
			sb.WriteString("\n")
		}
	}

	return sb.String(), nil
}

// unwrapParagraphs removes hard line breaks within plain-text paragraphs while
// preserving intentional structure: blank lines, markdown block-level syntax
// (headers, lists, blockquotes, horizontal rules), and code blocks (both fenced
// and indented). This converts the ~72-column convention used in commit messages
// into flowing text suitable for GitHub's markdown renderer.
//
// If HTML tags are found in prose (outside code blocks and inline code spans),
// the entire text is returned as-is — anyone writing raw HTML in a commit message
// is doing something intentional with formatting.

// htmlTagRe matches anything that looks like an HTML tag (e.g. <div>, </span>, <br/>).
var htmlTagRe = regexp.MustCompile(`</?[a-zA-Z][a-zA-Z0-9]*[\s/>]`)

// inlineCodeRe matches backtick-enclosed inline code spans so we can strip them
// before checking for HTML. Otherwise `<token>` in code would trigger a false positive.
var inlineCodeRe = regexp.MustCompile("`[^`]+`")

// fenceMarker returns the fence prefix ("```" or "~~~") if the line opens or
// closes a fenced code block, or "" otherwise.
func fenceMarker(trimmedLine string) string {
	if strings.HasPrefix(trimmedLine, "```") {
		return "```"
	}
	if strings.HasPrefix(trimmedLine, "~~~") {
		return "~~~"
	}
	return ""
}

// containsHTMLOutsideCode scans the text for HTML tags that appear in prose,
// ignoring content inside fenced code blocks, indented code blocks, and inline
// code spans. Returns true if HTML is found in any prose line.
func containsHTMLOutsideCode(text string) bool {
	lines := strings.Split(text, "\n")
	var openFence string // tracks the opening fence marker ("```" or "~~~"), empty when outside

	for _, line := range lines {
		trimmed := strings.TrimRight(line, " \t")
		marker := fenceMarker(trimmed)

		// Track fenced code blocks — only the matching marker can close a block
		if openFence == "" && marker != "" {
			openFence = marker
			continue
		}
		if openFence != "" {
			if marker == openFence {
				openFence = ""
			}
			continue
		}

		// Skip indented code blocks (4+ spaces or tab)
		if strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t") {
			continue
		}

		// Strip inline code spans, then check for HTML
		stripped := inlineCodeRe.ReplaceAllString(line, "")
		if htmlTagRe.MatchString(stripped) {
			return true
		}
	}

	return false
}

func unwrapParagraphs(text string) string {
	if text == "" {
		return ""
	}

	// Bail if the text contains HTML tags in prose — don't mess with it.
	if containsHTMLOutsideCode(text) {
		return text
	}

	lines := strings.Split(text, "\n")
	var result []string
	var paragraph []string
	var openFence string // tracks the opening fence marker ("```" or "~~~"), empty when outside

	flushParagraph := func() {
		if len(paragraph) > 0 {
			result = append(result, strings.Join(paragraph, " "))
			paragraph = nil
		}
	}

	for _, line := range lines {
		trimmed := strings.TrimRight(line, " \t")
		marker := fenceMarker(trimmed)

		// Track fenced code blocks — only the matching marker can close a block
		if openFence == "" && marker != "" {
			flushParagraph()
			result = append(result, line)
			openFence = marker
			continue
		}
		if openFence != "" {
			result = append(result, line)
			if marker == openFence {
				openFence = ""
			}
			continue
		}

		// Blank line = paragraph break
		if trimmed == "" {
			flushParagraph()
			result = append(result, "")
			continue
		}

		// Preserve lines that are markdown block-level elements
		if isBlockElement(trimmed) {
			flushParagraph()
			result = append(result, line)
			continue
		}

		// Indented code block (4+ spaces or tab)
		if strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t") {
			flushParagraph()
			result = append(result, line)
			continue
		}

		// Otherwise it's a paragraph line — accumulate it
		paragraph = append(paragraph, trimmed)
	}

	flushParagraph()

	return strings.Join(result, "\n")
}

// isBlockElement returns true if the line starts with markdown block-level syntax
// that should not be joined with adjacent lines.
func isBlockElement(line string) bool {
	// Headers
	if strings.HasPrefix(line, "#") {
		return true
	}
	// Unordered lists
	if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") || strings.HasPrefix(line, "+ ") ||
		line == "-" || line == "*" || line == "+" {
		return true
	}
	// Ordered lists (e.g. "1. ", "12. ")
	for i, ch := range line {
		if ch >= '0' && ch <= '9' {
			continue
		}
		if ch == '.' && i > 0 && i+1 < len(line) && line[i+1] == ' ' {
			return true
		}
		break
	}
	// Blockquotes
	if strings.HasPrefix(line, ">") {
		return true
	}
	// Horizontal rules (---, ***, ___)
	if isHorizontalRule(line) {
		return true
	}
	// Pipe tables
	if strings.HasPrefix(line, "|") {
		return true
	}
	return false
}

// isHorizontalRule checks for markdown horizontal rules: three or more
// -, *, or _ characters (with optional spaces).
func isHorizontalRule(line string) bool {
	stripped := strings.ReplaceAll(line, " ", "")
	if len(stripped) < 3 {
		return false
	}
	ch := stripped[0]
	if ch != '-' && ch != '*' && ch != '_' {
		return false
	}
	for _, c := range stripped {
		if byte(c) != ch {
			return false
		}
	}
	return true
}
