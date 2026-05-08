// cmd/submit.go
package cmd

import (
	"errors"
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
	Short: "Restack, push, and create/update PRs for the entire stack",
	Long: `Submit rebases, pushes, and creates or updates pull requests for all
branches in the stack.

By default, submit processes every tracked branch (the entire stack) in
parent-before-child order. Use --from to limit the scope to a subtree.

Phases:
1. Restack: rebase affected branches onto their parents
2. Push: force-push branches that will get a PR (or that are bases for one), with --force-with-lease
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
	submitFromFlag        string
)

// prAction describes what we will do for a branch in the PR phase after push.
type prAction int

const (
	prActionSkip prAction = iota
	prActionUpdate
	prActionAdopt
	prActionCreate
)

// prDecision is the outcome of planning one stack branch before any push.
type prDecision struct {
	node   *tree.Node
	parent string
	action prAction
	// prActionUpdate
	prNum int
	// prActionAdopt
	adoptPR *github.PR
	// prActionCreate
	title string
	body  string
	draft bool
	// pushAnyway is true when action is prActionSkip but a descendant branch
	// will get a PR, so the branch must still exist on the remote as a base.
	pushAnyway bool
	// skipReason is set when action is prActionSkip ("update" or "user").
	skipReason string
}

func init() {
	submitCmd.Flags().BoolVarP(&submitDryRunFlag, "dry-run", "D", false, "show what would be done without doing it")
	submitCmd.Flags().BoolVarP(&submitCurrentOnlyFlag, "current", "c", false, "only submit current branch, not descendants")
	submitCmd.Flags().BoolVarP(&submitUpdateOnlyFlag, "update", "u", false, "only update existing PRs, don't create new ones")
	submitCmd.Flags().BoolVarP(&submitPushOnlyFlag, "skip-prs", "s", false, "skip PR creation/update, only restack and push")
	submitCmd.Flags().BoolVarP(&submitYesFlag, "yes", "y", false, "skip interactive prompts and use auto-generated title/description for PRs")
	submitCmd.Flags().BoolVar(&submitWebFlag, "web", false, "open created/updated PRs in web browser")
	submitCmd.Flags().StringVarP(&submitFromFlag, "from", "f", "", "submit from this branch toward leaves (default: entire stack; bare --from = current branch)")
	submitCmd.Flags().Lookup("from").NoOptDefVal = "HEAD"
	rootCmd.AddCommand(submitCmd)
}

func runSubmit(cmd *cobra.Command, args []string) error {
	s := style.New()

	// Validate flag combinations
	if submitPushOnlyFlag && submitUpdateOnlyFlag {
		return errors.New("--skip-prs and --update cannot be used together: --skip-prs skips all PR operations")
	}
	if submitPushOnlyFlag && submitWebFlag {
		return errors.New("--skip-prs and --web cannot be used together: --skip-prs skips all PR operations")
	}
	if submitFromFlag != "" && submitCurrentOnlyFlag {
		return errors.New("--from and --current cannot be used together: --current limits the scope to the current branch")
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
		return errors.New("operation already in progress; use 'gh stack continue' or 'gh stack abort'")
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

	// Collect branches to submit.
	//
	// --current: only the current branch (no descendants, no ancestors).
	// --from (bare):  current branch + descendants (old default behavior).
	// --from=<branch>: that branch + descendants.
	// Default:         entire stack (all trunk descendants).
	var branches []*tree.Node
	if submitCurrentOnlyFlag {
		// --current: submit only the current checked-out branch
		if currentBranch == trunk {
			return fmt.Errorf("cannot submit trunk branch %q; switch to a stack branch or remove --current", trunk)
		}
		node := tree.FindNode(root, currentBranch)
		if node == nil {
			return fmt.Errorf("branch %q is not tracked in the stack\n\nTo add it, run:\n  gh stack adopt %s    # to stack on %s\n  gh stack adopt -p <parent>    # to stack on a different branch", currentBranch, trunk, trunk)
		}
		branches = append(branches, node)
	} else {
		// Determine the starting node for branch collection
		var startNode *tree.Node
		switch {
		case submitFromFlag == "HEAD":
			// --from without value: resolve to current branch (old behavior)
			startNode = tree.FindNode(root, currentBranch)
			if startNode == nil {
				return fmt.Errorf("branch %q is not tracked in the stack\n\nTo add it, run:\n  gh stack adopt %s    # to stack on %s\n  gh stack adopt -p <parent>    # to stack on a different branch", currentBranch, trunk, trunk)
			}
		case submitFromFlag != "" && submitFromFlag != trunk:
			// --from=<branch>: use specified branch
			startNode = tree.FindNode(root, submitFromFlag)
			if startNode == nil {
				return fmt.Errorf("branch %q is not tracked in the stack", submitFromFlag)
			}
		default:
			// Default (no --from, or --from=<trunk>): entire stack
			startNode = root
		}

		// Collect branches from start node (never include trunk itself)
		if startNode == root {
			branches = tree.GetDescendants(root)
			if len(branches) == 0 {
				return fmt.Errorf("no stack branches to submit; trunk %q has no descendants", trunk)
			}
		} else {
			branches = append(branches, startNode)
			branches = append(branches, tree.GetDescendants(startNode)...)
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
	if restackErr := doRestackWithState(g, cfg, branches, RestackOptions{
		DryRun:     submitDryRunFlag,
		Operation:  state.OperationSubmit,
		UpdateOnly: submitUpdateOnlyFlag,
		OpenWeb:    submitWebFlag,
		PushOnly:   submitPushOnlyFlag,
		Branches:   branchNames,
		StashRef:   stashRef,
	}, s); restackErr != nil {
		// Stash is saved in state for conflicts; restore on other errors
		if !errors.Is(restackErr, ErrConflict) && stashRef != "" {
			fmt.Println("Restoring auto-stashed changes...")
			if popErr := g.StashPop(stashRef); popErr != nil {
				fmt.Printf("%s could not restore stashed changes (commit %s): %v\n", s.WarningIcon(), git.AbbrevSHA(stashRef), popErr)
			}
		}
		return restackErr
	}

	// Phases 2 & 3
	err = doSubmitPushAndPR(g, cfg, root, branches, SubmitOptions{
		DryRun:     submitDryRunFlag,
		UpdateOnly: submitUpdateOnlyFlag,
		OpenWeb:    submitWebFlag,
		PushOnly:   submitPushOnlyFlag,
	}, s)

	// Restore auto-stashed changes after operation completes
	if stashRef != "" {
		fmt.Println("Restoring auto-stashed changes...")
		if popErr := g.StashPop(stashRef); popErr != nil {
			fmt.Printf("%s could not restore stashed changes (commit %s): %v\n", s.WarningIcon(), git.AbbrevSHA(stashRef), popErr)
		}
	}

	return err
}

// SubmitOptions configures the push and PR phases of submit.
type SubmitOptions struct {
	// DryRun prints what would be done without actually pushing or creating PRs.
	DryRun bool
	// UpdateOnly skips creating new PRs; only existing PRs are updated.
	UpdateOnly bool
	// OpenWeb opens created/updated PRs in the browser.
	OpenWeb bool
	// PushOnly skips the PR creation/update phase entirely.
	PushOnly bool
}

// doSubmitPushAndPR handles push and PR creation/update phases.
// This is called after restack succeeds (or from continue after conflict resolution).
func doSubmitPushAndPR(g *git.Git, cfg *config.Config, root *tree.Node, branches []*tree.Node, opts SubmitOptions, s *style.Style) error {
	var decisions []*prDecision
	var ghClient *github.Client
	if !opts.PushOnly && !opts.DryRun {
		var clientErr error
		ghClient, clientErr = github.NewClient()
		if clientErr != nil {
			return clientErr
		}
	}
	if !opts.PushOnly {
		trunk, err := cfg.GetTrunk()
		if err != nil {
			return err
		}
		decisions = planPRDecisions(g, cfg, ghClient, trunk, branches, opts.DryRun, opts.UpdateOnly, s)
		applyMustPushForSkippedAncestors(decisions)
	}

	decisionByName := make(map[string]*prDecision, len(decisions))
	for _, d := range decisions {
		decisionByName[d.node.Name] = d
	}

	// Phase 2: Push branches that will participate in PRs (or all if --skip-prs).
	fmt.Println(s.Bold("\n=== Phase 2: Push ==="))
	for _, b := range branches {
		var d *prDecision
		if !opts.PushOnly {
			d = decisionByName[b.Name]
		}
		shouldPush := opts.PushOnly || d == nil || d.action != prActionSkip || d.pushAnyway
		if !shouldPush {
			fmt.Printf("Skipping push for %s %s\n", s.Branch(b.Name), s.Muted("(no PR for this branch)"))
			continue
		}
		if opts.DryRun {
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

	if opts.PushOnly {
		fmt.Println(s.Bold("\n=== Phase 3: PRs ==="))
		fmt.Println(s.Muted("Skipped (--skip-prs)"))
		return nil
	}
	return executePRDecisions(g, cfg, root, decisions, ghClient, opts, s)
}

// planPRDecisions resolves what to do for each branch before any push.
// When dryRun is true, ghClient may be nil (no GitHub lookups for adopt).
func planPRDecisions(g *git.Git, cfg *config.Config, ghClient *github.Client, trunk string, branches []*tree.Node, dryRun, updateOnly bool, s *style.Style) []*prDecision {
	out := make([]*prDecision, 0, len(branches))
	for _, b := range branches {
		parent, _ := cfg.GetParent(b.Name) //nolint:errcheck // empty is fine
		if parent == "" {
			parent = trunk
		}
		existingPR, _ := cfg.GetPR(b.Name) //nolint:errcheck // 0 is fine
		switch {
		case existingPR > 0:
			out = append(out, &prDecision{node: b, parent: parent, action: prActionUpdate, prNum: existingPR})
		case updateOnly:
			out = append(out, &prDecision{node: b, parent: parent, action: prActionSkip, skipReason: "update"})
		case dryRun:
			out = append(out, planPRDecisionDryRun(g, b, parent, trunk, s))
		default:
			out = append(out, planPRDecisionInteractive(g, ghClient, b, parent, trunk, s))
		}
	}
	return out
}

func planPRDecisionDryRun(g *git.Git, b *tree.Node, parent, trunk string, s *style.Style) *prDecision {
	draft := parent != trunk
	defaultTitle := generateDefaultTitle(g, parent, b.Name)
	defaultBody, bodyErr := generatePRBody(g, parent, b.Name)
	if bodyErr != nil {
		fmt.Printf("%s could not generate PR body: %v\n", s.WarningIcon(), bodyErr)
		defaultBody = ""
	}
	return &prDecision{
		node:   b,
		parent: parent,
		action: prActionCreate,
		title:  defaultTitle,
		body:   defaultBody,
		draft:  draft,
	}
}

func planPRDecisionInteractive(g *git.Git, ghClient *github.Client, b *tree.Node, parent, trunk string, s *style.Style) *prDecision {
	branch := b.Name
	existingPR, err := ghClient.FindPRByHead(branch)
	if err != nil {
		fmt.Printf("%s could not check for existing PR: %v\n", s.WarningIcon(), err)
	} else if existingPR != nil {
		return &prDecision{node: b, parent: parent, action: prActionAdopt, adoptPR: existingPR}
	}

	draft := parent != trunk
	defaultTitle := generateDefaultTitle(g, parent, branch)
	defaultBody, bodyErr := generatePRBody(g, parent, branch)
	if bodyErr != nil {
		fmt.Printf("%s could not generate PR body: %v\n", s.WarningIcon(), bodyErr)
		defaultBody = ""
	}
	title, body, skipped, err := promptForPRDetails(branch, defaultTitle, defaultBody, s)
	if err != nil {
		fmt.Printf("%s failed to get PR details for %s: %v\n", s.WarningIcon(), s.Branch(branch), err)
		return &prDecision{node: b, parent: parent, action: prActionSkip, skipReason: "user"}
	}
	if skipped {
		return &prDecision{node: b, parent: parent, action: prActionSkip, skipReason: "user"}
	}
	return &prDecision{node: b, parent: parent, action: prActionCreate, title: title, body: body, draft: draft}
}

// applyMustPushForSkippedAncestors marks skipped branches that must still be
// pushed because a descendant will open or update a PR that uses them as base.
func applyMustPushForSkippedAncestors(decisions []*prDecision) {
	byName := make(map[string]*prDecision, len(decisions))
	for _, d := range decisions {
		byName[d.node.Name] = d
	}
	for _, d := range decisions {
		if d.action == prActionSkip {
			continue
		}
		for cur := d.node.Parent; cur != nil; cur = cur.Parent {
			if x := byName[cur.Name]; x != nil && x.action == prActionSkip {
				x.pushAnyway = true
			}
		}
	}
}

// executePRDecisions runs the PR phase from pre-planned decisions (after push).
// planningClient is the client used during planPRDecisions; it is nil in dry-run.
// When not dry-run, the same client is reused here (no second NewClient).
func executePRDecisions(g *git.Git, cfg *config.Config, root *tree.Node, decisions []*prDecision, planningClient *github.Client, opts SubmitOptions, s *style.Style) error {
	fmt.Println(s.Bold("\n=== Phase 3: PRs ==="))

	trunk, err := cfg.GetTrunk()
	if err != nil {
		return err
	}

	ghClient := planningClient
	if !opts.DryRun && ghClient == nil {
		var clientErr error
		ghClient, clientErr = github.NewClient()
		if clientErr != nil {
			return clientErr
		}
	}

	var remoteBranches map[string]bool
	if !opts.DryRun {
		var rbErr error
		remoteBranches, rbErr = g.ListRemoteBranches()
		if rbErr != nil {
			fmt.Printf("%s could not list remote branches, stack comments may reference local-only branches: %v\n", s.WarningIcon(), rbErr)
		}
	}

	pCtx := prContext{
		ghClient:       ghClient,
		cfg:            cfg,
		root:           root,
		trunk:          trunk,
		remoteBranches: remoteBranches,
		s:              s,
	}

	var prURLs []string

	for _, d := range decisions {
		b := d.node
		parent := d.parent

		switch d.action {
		case prActionSkip:
			if opts.DryRun {
				if d.skipReason == "update" {
					fmt.Printf("Skipping %s %s\n", s.Branch(b.Name), s.Muted("(no existing PR, --update)"))
				}
				continue
			}
			switch d.skipReason {
			case "update":
				fmt.Printf("Skipping %s %s\n", s.Branch(b.Name), s.Muted("(no existing PR, --update)"))
			case "user":
				fmt.Printf("Skipped PR for %s %s\n", s.Branch(b.Name), s.Muted("(skipped)"))
			}
		case prActionUpdate:
			if opts.DryRun {
				fmt.Printf("%s Would update PR #%d base to %s\n", s.Muted("dry-run:"), d.prNum, s.Branch(parent))
			} else {
				fmt.Printf("Updating %s for %s (base: %s)... ", s.Hyperlink(fmt.Sprintf("PR #%d", d.prNum), ghClient.PRURL(d.prNum)), s.Branch(b.Name), s.Branch(parent))
				if err := ghClient.UpdatePRBase(d.prNum, parent); err != nil {
					fmt.Println(s.Error("failed"))
					fmt.Printf("%s failed to update PR #%d base: %v\n", s.WarningIcon(), d.prNum, err)
				} else {
					fmt.Println(s.Success("ok"))
					// Check for trunk transition BEFORE persisting the new base, so
					// isTransitionToTrunk compares against the previous stored value.
					maybeMarkPRReady(ghClient, cfg, d.prNum, b.Name, parent, trunk, s)
					_ = cfg.SetPRBase(b.Name, parent) //nolint:errcheck // best effort — used for transition detection only
					if err := ghClient.GenerateAndPostStackComment(root, b.Name, trunk, d.prNum, remoteBranches); err != nil {
						fmt.Printf("%s failed to update stack comment for PR #%d: %v\n", s.WarningIcon(), d.prNum, err)
					}
					if opts.OpenWeb {
						prURLs = append(prURLs, ghClient.PRURL(d.prNum))
					}
				}
			}
		case prActionAdopt:
			if opts.DryRun {
				fmt.Printf("%s Would adopt PR #%d for %s (base: %s)\n", s.Muted("dry-run:"), d.adoptPR.Number, s.Branch(b.Name), s.Branch(parent))
			} else {
				prNum, adoptErr := adoptExistingPRDirect(pCtx, b.Name, parent, d.adoptPR)
				switch {
				case adoptErr != nil:
					fmt.Printf("%s failed to adopt PR for %s: %v\n", s.WarningIcon(), s.Branch(b.Name), adoptErr)
				default:
					fmt.Printf("%s Adopted PR #%d for %s (%s)\n", s.SuccessIcon(), prNum, s.Branch(b.Name), ghClient.PRURL(prNum))
					if opts.OpenWeb {
						prURLs = append(prURLs, ghClient.PRURL(prNum))
					}
				}
			}
		case prActionCreate:
			if opts.DryRun {
				fmt.Printf("%s Would create PR for %s (base: %s)\n", s.Muted("dry-run:"), s.Branch(b.Name), s.Branch(parent))
			} else {
				prNum, adopted, execErr := executePRCreate(pCtx, b.Name, parent, d.title, d.body, d.draft)
				switch {
				case execErr != nil:
					fmt.Printf("%s failed to create PR for %s: %v\n", s.WarningIcon(), s.Branch(b.Name), execErr)
				case adopted:
					fmt.Printf("%s Adopted PR #%d for %s (%s)\n", s.SuccessIcon(), prNum, s.Branch(b.Name), ghClient.PRURL(prNum))
					if opts.OpenWeb {
						prURLs = append(prURLs, ghClient.PRURL(prNum))
					}
				default:
					fmt.Printf("%s Created PR #%d for %s (%s)\n", s.SuccessIcon(), prNum, s.Branch(b.Name), ghClient.PRURL(prNum))
					if opts.OpenWeb {
						prURLs = append(prURLs, ghClient.PRURL(prNum))
					}
				}
			}
		}
	}

	if opts.OpenWeb && len(prURLs) > 0 {
		brw := browser.New("", os.Stdout, os.Stderr)
		for _, url := range prURLs {
			if err := brw.Browse(url); err != nil {
				fmt.Fprintf(os.Stderr, "%s could not open browser for %s: %v\n", s.WarningIcon(), url, err)
			}
		}
	}

	return nil
}

// prContext bundles the shared read-only context that is threaded through the
// PR creation and adoption helpers. Grouping these avoids repeating the same
// six parameters on every private function that participates in the submit
// workflow.
type prContext struct {
	ghClient       *github.Client
	cfg            *config.Config
	root           *tree.Node
	trunk          string
	remoteBranches map[string]bool
	s              *style.Style
}

// executePRCreate opens a PR with the given title/body (branch must be on the remote).
// Returns adopted true if an existing PR was adopted instead of creating.
func executePRCreate(pCtx prContext, branch, base, title, body string, draft bool) (prNum int, adopted bool, err error) {
	pr, err := pCtx.ghClient.CreateSubmitPR(branch, base, title, body, draft)
	if err != nil {
		if strings.Contains(err.Error(), "pull request already exists") {
			prNum, adoptErr := adoptExistingPR(pCtx, branch, base)
			return prNum, true, adoptErr
		}
		if isBaseBranchInvalidError(err) {
			return 0, false, fmt.Errorf(
				"base branch %q does not exist on the remote; push it first or run 'gh stack submit' to push the entire stack: %w",
				base, err)
		}
		return 0, false, err
	}

	if err := pCtx.cfg.SetPR(branch, pr.Number); err != nil {
		return pr.Number, false, fmt.Errorf("PR created but failed to store number: %w", err)
	}

	// Persist the base so future submit runs can detect trunk transitions.
	_ = pCtx.cfg.SetPRBase(branch, base) //nolint:errcheck // best effort

	if node := tree.FindNode(pCtx.root, branch); node != nil {
		node.PR = pr.Number
	}

	if err := pCtx.ghClient.GenerateAndPostStackComment(pCtx.root, branch, pCtx.trunk, pr.Number, pCtx.remoteBranches); err != nil {
		fmt.Printf("%s failed to add stack comment to PR #%d: %v\n", pCtx.s.WarningIcon(), pr.Number, err)
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
// If --yes flag is set or not in an interactive terminal (stdin/stdout not TTYs),
// returns the defaults without prompting.
// Returns (title, body, skipped, error) where skipped is true if user pressed ESC.
func promptForPRDetails(branch, defaultTitle, defaultBody string, s *style.Style) (title, body string, skipped bool, err error) {
	// Skip prompts if --yes flag is set
	if submitYesFlag {
		return defaultTitle, defaultBody, false, nil
	}

	// Skip prompts if not interactive
	if !prompt.IsInteractive() {
		return defaultTitle, defaultBody, false, nil
	}

	fmt.Printf("\n--- Creating PR for %s %s ---\n", s.Branch(branch), s.Muted("(ESC to skip, --yes to skip prompts)"))

	// Prompt for title with skip support
	title, skipped, err = prompt.InputWithSkip("PR title", "Press ESC to skip creating this PR", defaultTitle)
	if err != nil {
		return "", "", false, err
	}
	if skipped {
		return "", "", true, nil
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return "", "", false, errors.New("PR title cannot be empty")
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
		return "", "", false, err
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
	return title, body, false, nil
}

// adoptExistingPR finds an existing PR for the branch and adopts it into the stack.
func adoptExistingPR(pCtx prContext, branch, base string) (int, error) {
	existingPR, err := pCtx.ghClient.FindPRByHead(branch)
	if err != nil {
		return 0, fmt.Errorf("failed to find existing PR: %w", err)
	}
	if existingPR == nil {
		return 0, fmt.Errorf("PR creation failed but no existing PR found for branch %q", branch)
	}

	return adoptExistingPRDirect(pCtx, branch, base, existingPR)
}

// adoptExistingPRDirect adopts an already-fetched PR into the stack.
// This is the implementation shared by adoptExistingPR and the adopt path in executePRDecisions.
func adoptExistingPRDirect(pCtx prContext, branch, base string, existingPR *github.PR) (int, error) {
	// Store PR number in config
	if err := pCtx.cfg.SetPR(branch, existingPR.Number); err != nil {
		return existingPR.Number, fmt.Errorf("failed to store PR number: %w", err)
	}

	// Update the tree node's PR number so stack comments render correctly
	if node := tree.FindNode(pCtx.root, branch); node != nil {
		node.PR = existingPR.Number
	}

	// Update PR base to match stack parent
	if existingPR.Base.Ref != base {
		if err := pCtx.ghClient.UpdatePRBase(existingPR.Number, base); err != nil {
			fmt.Printf("%s failed to update base: %v\n", pCtx.s.WarningIcon(), err)
		}
	}

	// Persist the new base for transition detection on future submit runs.
	_ = pCtx.cfg.SetPRBase(branch, base) //nolint:errcheck // best effort

	// Add/update stack navigation comment
	if err := pCtx.ghClient.GenerateAndPostStackComment(pCtx.root, branch, pCtx.trunk, existingPR.Number, pCtx.remoteBranches); err != nil {
		fmt.Printf("%s failed to update stack comment: %v\n", pCtx.s.WarningIcon(), err)
	}

	// Prompt to publish if the PR is a draft and its base is transitioning to trunk.
	// We use the live pre-adoption base (existingPR.Base.Ref) here since stored base
	// metadata may not exist yet for a freshly adopted PR.
	if existingPR.Draft && base == pCtx.trunk && existingPR.Base.Ref != pCtx.trunk {
		promptMarkPRReady(pCtx.ghClient, existingPR.Number, branch, pCtx.trunk, pCtx.s)
	}

	return existingPR.Number, nil
}

// isTransitionToTrunk reports whether a PR's base branch is moving to trunk for
// the first time as tracked by gh-stack. It uses the stored last-known PR base
// (persisted after each successful submit) to detect the transition, avoiding
// any additional GitHub API calls.
//
// Returns true when:
//   - no stored base exists yet (first run after PR creation/adoption), or
//   - stored base is not trunk (base is changing from something else to trunk).
//
// Returns false when the stored base is already trunk, meaning the PR was
// already targeting trunk on the previous submit run.
func isTransitionToTrunk(cfg *config.Config, branch, trunk string) bool {
	storedBase, err := cfg.GetPRBase(branch)
	if err == nil && storedBase == trunk {
		return false
	}
	return true
}

// maybeMarkPRReady checks if a PR is a draft targeting trunk and offers to publish it.
// This handles the case where a PR was created as a draft (middle of stack) but now
// targets trunk because its parent was merged.
//
// The prompt fires only when the base branch is transitioning to trunk for the first
// time — i.e., the stored last-known base is not already trunk. This prevents the
// prompt from appearing on every submit run once the PR already targets trunk.
func maybeMarkPRReady(ghClient *github.Client, cfg *config.Config, prNumber int, branch, base, trunk string, s *style.Style) {
	// Only relevant if PR now targets trunk
	if base != trunk {
		return
	}

	// Check if PR is a draft
	pr, err := ghClient.GetPR(prNumber)
	if err != nil || !pr.Draft {
		return
	}

	// Only prompt when the base is transitioning to trunk this run.
	if !isTransitionToTrunk(cfg, branch, trunk) {
		return
	}

	promptMarkPRReady(ghClient, prNumber, branch, trunk, s)
}

// promptMarkPRReady prompts to publish a draft PR and marks it ready if confirmed.
// Called when we already know the PR is a draft targeting trunk.
func promptMarkPRReady(ghClient *github.Client, prNumber int, branch, trunk string, s *style.Style) {
	fmt.Printf("PR #%d (%s) is a draft and now targets %s.\n", prNumber, s.Branch(branch), s.Branch(trunk))

	// Skip prompt if --yes flag is set or non-interactive
	shouldMarkReady := false
	if !submitYesFlag && prompt.IsInteractive() {
		shouldMarkReady, _ = prompt.Confirm("Mark as ready for review?", false) //nolint:errcheck // default is fine
	}

	if shouldMarkReady {
		if readyErr := ghClient.MarkPRReady(prNumber); readyErr != nil {
			fmt.Printf("%s failed to mark PR ready: %v\n", s.WarningIcon(), readyErr)
		} else {
			fmt.Printf("%s PR #%d marked as ready for review.\n", s.SuccessIcon(), prNumber)
		}
	}
}

// isBaseBranchInvalidError returns true if the error indicates that the PR base
// branch does not exist on the remote (GitHub returns HTTP 422 with
// "PullRequest.base is invalid" in this case).
func isBaseBranchInvalidError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "PullRequest.base is invalid") ||
		strings.Contains(msg, "base is invalid")
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

// htmlTagRe matches anything that looks like an HTML tag, including custom
// elements with hyphens (e.g. <my-component>) and namespaced tags (e.g. <xml:tag>).
var htmlTagRe = regexp.MustCompile(`</?[a-zA-Z][-:a-zA-Z0-9]*[\s/>]`)

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

// unwrapParagraphs removes hard line breaks within plain-text paragraphs and
// list items while preserving intentional structure: blank lines, markdown
// block-level syntax (headers, blockquotes, horizontal rules), and code blocks
// (both fenced and indented). This converts the ~72-column convention used in
// commit messages into flowing text suitable for GitHub's markdown renderer.
//
// List items are treated like paragraphs for unwrapping: a hard-wrapped list
// item (with or without continuation indentation) is joined back into a single
// line. Each new list marker starts a fresh accumulation group, so consecutive
// items remain separate.
//
// If HTML tags are found in prose (outside code blocks and inline code spans),
// the entire text is returned as-is — anyone writing raw HTML in a commit message
// is doing something intentional with formatting.
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

		// List items start a new accumulation group so that hard-wrapped
		// continuations are joined back into the item, just like paragraphs.
		if isListItem(trimmed) {
			flushParagraph()
			paragraph = append(paragraph, trimmed)
			continue
		}

		// Non-list block elements (headers, blockquotes, rules, tables). Lists
		// are handled above via isListItem so they accumulate continuations.
		if isBlockElement(trimmed) {
			flushParagraph()
			result = append(result, line)
			continue
		}

		// Continuation of a list item: strip leading whitespace that may
		// come from markdown continuation indentation (e.g. 2-space indent
		// under a list marker). This must be checked before the indented
		// code block rule — nested list continuations can easily reach 4+
		// spaces (2 for nesting + 2 for continuation).
		if len(paragraph) > 0 && isListItem(paragraph[0]) {
			paragraph = append(paragraph, strings.TrimSpace(trimmed))
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

// isListItem returns true if the (possibly indented) line starts a markdown
// list item: unordered ("- ", "* ", "+ ") or ordered ("1. ", "12. ", etc.).
// Indented list items (nested lists) are also detected.
func isListItem(line string) bool {
	stripped := strings.TrimLeft(line, " \t")
	if stripped == "" {
		return false
	}
	// Unordered lists
	if strings.HasPrefix(stripped, "- ") || strings.HasPrefix(stripped, "* ") || strings.HasPrefix(stripped, "+ ") ||
		stripped == "-" || stripped == "*" || stripped == "+" {
		return true
	}
	// Ordered lists (e.g. "1. ", "12. ")
	for i, ch := range stripped {
		if ch >= '0' && ch <= '9' {
			continue
		}
		if ch == '.' && i > 0 && i+1 < len(stripped) && stripped[i+1] == ' ' {
			return true
		}
		break
	}
	return false
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
