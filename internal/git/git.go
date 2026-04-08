// internal/git/git.go
package git

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/cli/safeexec"
)

// ErrDirtyWorkTree is returned when the working tree has uncommitted changes.
var ErrDirtyWorkTree = errors.New("working tree has uncommitted changes")

var (
	gitPath     string
	gitPathOnce sync.Once
	errGitPath  error
)

// resolveGitPath finds the git executable using safeexec to prevent PATH injection.
func resolveGitPath() (string, error) {
	gitPathOnce.Do(func() {
		gitPath, errGitPath = safeexec.LookPath("git")
	})
	return gitPath, errGitPath
}

// Git provides git operations for a repository.
type Git struct {
	repoPath string
}

// New creates a Git instance for the repository at the given path.
func New(repoPath string) *Git {
	return &Git{repoPath: repoPath}
}

// run executes a git command and returns stdout. Stderr is captured for error messages.
func (g *Git) run(args ...string) (string, error) {
	gitBin, err := resolveGitPath()
	if err != nil {
		return "", fmt.Errorf("failed to find git: %w", err)
	}

	fullArgs := append([]string{"-C", g.repoPath}, args...)
	cmd := exec.Command(gitBin, fullArgs...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return "", fmt.Errorf("git %s: %s", args[0], strings.TrimSpace(stderr.String()))
		}
		return "", fmt.Errorf("git %s: %w", args[0], err)
	}

	return strings.TrimSpace(stdout.String()), nil
}

// runInteractive executes a git command with stdout/stderr connected to the terminal.
func (g *Git) runInteractive(args ...string) error {
	gitBin, err := resolveGitPath()
	if err != nil {
		return fmt.Errorf("failed to find git: %w", err)
	}

	fullArgs := append([]string{"-C", g.repoPath}, args...)
	cmd := exec.Command(gitBin, fullArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// runSilent executes a git command and discards output, returning only success/failure.
func (g *Git) runSilent(args ...string) error {
	_, err := g.run(args...)
	return err
}

// CurrentBranch returns the name of the current branch.
func (g *Git) CurrentBranch() (string, error) {
	return g.run("rev-parse", "--abbrev-ref", "HEAD")
}

// BranchExists checks if a branch exists.
func (g *Git) BranchExists(branch string) bool {
	err := g.runSilent("rev-parse", "--verify", "refs/heads/"+branch)
	return err == nil
}

// CreateBranch creates a new branch at the current HEAD.
func (g *Git) CreateBranch(name string) error {
	return g.runSilent("branch", name)
}

// Checkout switches to the specified branch.
func (g *Git) Checkout(branch string) error {
	return g.runSilent("checkout", branch)
}

// CreateAndCheckout creates a new branch and switches to it.
func (g *Git) CreateAndCheckout(name string) error {
	return g.runSilent("checkout", "-b", name)
}

// IsDirty returns true if there are uncommitted changes (staged or unstaged).
func (g *Git) IsDirty() (bool, error) {
	out, err := g.run("status", "--porcelain")
	if err != nil {
		return false, err
	}
	return len(out) > 0, nil
}

// HasStagedChanges returns true if there are staged changes.
func (g *Git) HasStagedChanges() (bool, error) {
	err := g.runSilent("diff", "--cached", "--quiet")
	if err != nil {
		// Exit code 1 means there are differences — this is expected, not a real error
		return true, nil //nolint:nilerr // non-zero exit = staged changes exist
	}
	return false, nil
}

// Commit creates a commit with the given message.
func (g *Git) Commit(message string) error {
	return g.runSilent("commit", "-m", message)
}

// Push force-pushes a branch to origin with lease.
func (g *Git) Push(branch string, force bool) error {
	args := []string{"push", "origin", branch}
	if force {
		args = append(args, "--force-with-lease")
	}
	return g.runInteractive(args...)
}

// GetMergeBase returns the merge base of two branches.
func (g *Git) GetMergeBase(a, b string) (string, error) {
	return g.run("merge-base", a, b)
}

// GetTip returns the commit SHA at the tip of a branch.
func (g *Git) GetTip(branch string) (string, error) {
	return g.run("rev-parse", branch)
}

// NeedsRebase returns true if branch needs to be rebased onto parent.
func (g *Git) NeedsRebase(branch, parent string) (bool, error) {
	mergeBase, err := g.GetMergeBase(branch, parent)
	if err != nil {
		return false, err
	}
	parentTip, err := g.GetTip(parent)
	if err != nil {
		return false, err
	}
	return mergeBase != parentTip, nil
}

// Rebase rebases the current branch onto target.
func (g *Git) Rebase(onto string) error {
	return g.runInteractive("rebase", onto)
}

// CommitExists checks if a commit SHA exists in the repository.
func (g *Git) CommitExists(sha string) bool {
	err := g.runSilent("cat-file", "-e", sha+"^{commit}")
	return err == nil
}

// HasUnmergedCommits returns true if the branch has commits not yet in upstream.
// Uses git cherry to detect by diff content, which works for cherry-picks
// where the commit SHAs differ but the content is the same.
// Note: This does NOT detect squash merges where multiple commits are combined.
// Use IsContentMerged for squash merge detection.
func (g *Git) HasUnmergedCommits(branch, upstream string) (bool, error) {
	// git cherry -v upstream branch
	// Returns lines prefixed with + (unmerged) or - (merged/equivalent)
	out, err := g.run("cherry", "-v", upstream, branch)
	if err != nil {
		return false, err
	}

	// Empty output means no commits unique to branch
	if out == "" {
		return false, nil
	}

	// If any line starts with +, there are unmerged commits
	for line := range strings.SplitSeq(out, "\n") {
		if strings.HasPrefix(line, "+ ") {
			return true, nil
		}
	}
	return false, nil
}

// IsContentMerged returns true if the branch has no content differences from upstream.
// This detects squash merges where the tree content is identical even though
// the commit history differs. Returns true if `git diff upstream branch` is empty.
func (g *Git) IsContentMerged(branch, upstream string) (bool, error) {
	// git diff upstream branch --quiet exits 0 if no diff, 1 if diff exists
	err := g.runSilent("diff", "--quiet", upstream, branch)
	if err != nil {
		// Exit code 1 means there are differences — this is expected, not a real error
		return false, nil //nolint:nilerr // non-zero exit = content differs
	}
	// Exit code 0 means no differences - content is merged
	return true, nil
}

// RemoteBranchExists checks if a branch exists on the remote (origin).
func (g *Git) RemoteBranchExists(branch string) bool {
	err := g.runSilent("ls-remote", "--exit-code", "--heads", "origin", branch)
	return err == nil
}

// ListRemoteBranches returns the set of branch names that exist on origin,
// based on locally-cached remote-tracking refs (no network call).
// This is accurate as long as a fetch or push has been done recently.
func (g *Git) ListRemoteBranches() (map[string]bool, error) {
	out, err := g.run("for-each-ref", "--format=%(refname:strip=3)", "refs/remotes/origin/")
	if err != nil {
		return nil, err
	}
	branches := make(map[string]bool)
	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && line != "HEAD" {
			branches[line] = true
		}
	}
	return branches, nil
}

// RebaseOnto rebases a branch onto a new base, replaying only commits after oldBase.
// Checks out the branch first, then runs: git rebase --onto <newBase> <oldBase>
// Useful when a parent branch was squash-merged and we need to replay only
// the commits unique to the child branch.
func (g *Git) RebaseOnto(newBase, oldBase, branch string) error {
	// Checkout the branch to rebase (git rebase operates on HEAD)
	if err := g.Checkout(branch); err != nil {
		return err
	}
	return g.runInteractive("rebase", "--onto", newBase, oldBase)
}

// RebaseContinue continues an in-progress rebase.
func (g *Git) RebaseContinue() error {
	return g.runInteractive("-c", "core.editor=true", "rebase", "--continue")
}

// RebaseAbort aborts an in-progress rebase.
func (g *Git) RebaseAbort() error {
	return g.runSilent("rebase", "--abort")
}

// IsRebaseInProgress checks if a rebase is in progress.
// Uses GetResolvedGitDir so it works in linked worktrees where .git is a file,
// not a directory.
func (g *Git) IsRebaseInProgress() bool {
	gitDir, err := g.GetResolvedGitDir()
	if err != nil {
		// Fall back to the old approach if we can't resolve the git dir
		gitDir = filepath.Join(g.repoPath, ".git")
	}
	rebaseMerge := filepath.Join(gitDir, "rebase-merge")
	rebaseApply := filepath.Join(gitDir, "rebase-apply")
	_, err1 := os.Stat(rebaseMerge)
	_, err2 := os.Stat(rebaseApply)
	return err1 == nil || err2 == nil
}

// GetResolvedGitDir returns the actual git directory for this repository.
// In a main worktree this is typically <repo>/.git.
// In a linked worktree this resolves to the per-worktree dir
// (e.g. <main-repo>/.git/worktrees/<name>), which is where rebase state lives.
func (g *Git) GetResolvedGitDir() (string, error) {
	out, err := g.run("rev-parse", "--git-dir")
	if err != nil {
		return "", err
	}
	// git rev-parse --git-dir may return a relative path when using -C;
	// resolve it relative to the repo path.
	if !filepath.IsAbs(out) {
		out = filepath.Join(g.repoPath, out)
	}
	return filepath.Clean(out), nil
}

// ListWorktrees parses `git worktree list --porcelain` and returns a map of
// branch name to worktree path for all linked worktrees (the main worktree
// is excluded).
func (g *Git) ListWorktrees() (map[string]string, error) {
	out, err := g.run("worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}

	result := make(map[string]string)
	var currentPath string
	first := true // skip the first entry (main worktree)

	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimSpace(line)

		if wtPath, ok := strings.CutPrefix(line, "worktree "); ok {
			if first {
				// Skip the main worktree entry entirely.
				first = false
				currentPath = ""
			} else {
				currentPath = wtPath
			}
			continue
		}

		if strings.HasPrefix(line, "branch ") && currentPath != "" {
			ref := strings.TrimPrefix(line, "branch ")
			// Strip refs/heads/ prefix to get the branch name
			branch := strings.TrimPrefix(ref, "refs/heads/")
			result[branch] = currentPath
			currentPath = ""
			continue
		}

		// Empty line separates worktree entries
		if line == "" {
			currentPath = ""
		}
	}

	return result, nil
}

// RebaseHere rebases the already-checked-out HEAD onto the given branch.
// This is semantically identical to Rebase but named explicitly to clarify
// that no checkout is needed -- the caller is operating inside a worktree
// that already has the target branch checked out.
func (g *Git) RebaseHere(onto string) error {
	return g.runInteractive("rebase", onto)
}

// RebaseOntoHere runs `git rebase --onto <newBase> <oldBase>` on the
// already-checked-out HEAD. Needed for fork-point rebases in worktrees
// where we cannot (and don't need to) checkout first.
func (g *Git) RebaseOntoHere(newBase, oldBase string) error {
	return g.runInteractive("rebase", "--onto", newBase, oldBase)
}

// GetGitDir returns the .git directory path.
func (g *Git) GetGitDir() string {
	return filepath.Join(g.repoPath, ".git")
}

// Fetch fetches from origin.
func (g *Git) Fetch() error {
	return g.runInteractive("fetch", "origin")
}

// FastForward fast-forwards a branch to its remote tracking branch.
func (g *Git) FastForward(branch string) error {
	// First checkout the branch
	if err := g.Checkout(branch); err != nil {
		return err
	}
	// Then merge with fast-forward only
	return g.runSilent("merge", "--ff-only", "origin/"+branch)
}

// DeleteBranch deletes a local branch.
func (g *Git) DeleteBranch(branch string) error {
	return g.runSilent("branch", "-D", branch)
}

// SetBranchRef sets a branch ref to point to a specific SHA.
// This is equivalent to `git branch -f <branch> <sha>`.
func (g *Git) SetBranchRef(branch, sha string) error {
	return g.runSilent("branch", "-f", branch, sha)
}

// CreateBranchAt creates a new branch at a specific SHA.
// This is equivalent to `git branch <name> <sha>`.
func (g *Git) CreateBranchAt(name, sha string) error {
	return g.runSilent("branch", name, sha)
}

// Stash creates a stash with the given message and returns the stash commit hash.
// Returns an empty string if there was nothing to stash.
// Includes untracked files (-u) to capture all working tree changes.
// The returned hash is stable and can be used to restore this specific stash
// even if other stashes are created later.
func (g *Git) Stash(message string) (string, error) {
	// Check if there's anything to stash first
	dirty, err := g.IsDirty()
	if err != nil {
		return "", err
	}
	if !dirty {
		return "", nil
	}

	// Create the stash with -u to include untracked files
	if stashErr := g.runSilent("stash", "push", "-u", "-m", message); stashErr != nil {
		return "", stashErr
	}

	// Resolve the created stash to a stable identifier (its commit hash)
	// This is more reliable than stash@{0} which can shift if user creates more stashes
	out, parseErr := g.run("rev-parse", "stash@{0}")
	if parseErr != nil {
		return "", parseErr
	}
	return out, nil
}

// StashPop restores a specific stash entry when a reference is provided.
// If ref is empty, it pops the most recent stash entry (matching `git stash pop`).
// When a commit hash is provided (from Stash()), uses apply+drop since pop only accepts stash refs.
// Returns an error if there are conflicts or no matching stash entry.
func (g *Git) StashPop(ref string) error {
	if ref == "" {
		return g.runInteractive("stash", "pop")
	}

	// git stash pop only accepts stash references (stash@{n}), not commit hashes.
	// Use apply instead which accepts commit hashes.
	if err := g.runInteractive("stash", "apply", ref); err != nil {
		return err
	}

	// Find and drop the stash entry by matching its commit hash
	stashRef, err := g.findStashByHash(ref)
	if err != nil {
		// Apply succeeded but we couldn't find stash to drop - not fatal
		return nil //nolint:nilerr // apply already succeeded, drop failure is best-effort
	}
	if stashRef != "" {
		// Silently drop; errors aren't critical since apply already succeeded
		_ = g.runSilent("stash", "drop", stashRef) //nolint:errcheck // best effort cleanup
	}
	return nil
}

// findStashByHash finds the stash@{n} reference for a given commit hash.
// Returns empty string if not found.
func (g *Git) findStashByHash(hash string) (string, error) {
	// List stashes with their hashes
	out, err := g.run("stash", "list", "--format=%H %gd")
	if err != nil {
		return "", err
	}
	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) == 2 && parts[0] == hash {
			return parts[1], nil
		}
	}
	return "", nil
}

// StashList returns true if there are any stash entries.
func (g *Git) StashList() (bool, error) {
	out, err := g.run("stash", "list")
	if err != nil {
		return false, err
	}
	return len(out) > 0, nil
}

// Commit represents a git commit with its subject and body.
type Commit struct {
	Subject string // First line of the commit message
	Body    string // Everything after the first line (may be empty)
}

// GetCommits returns the commits from base..head (commits in head not in base).
// Returns commits in reverse chronological order (newest first).
func (g *Git) GetCommits(base, head string) ([]Commit, error) {
	// Use null byte separators for reliable parsing
	// Format: subject\x00body\x00\x00 (double null between commits)
	format := "%s%x00%b%x00%x00"
	out, err := g.run("log", "--format="+format, base+".."+head)
	if err != nil {
		return nil, err
	}

	if out == "" {
		return nil, nil
	}

	var commits []Commit
	// Split by double null (between commits)
	for entry := range strings.SplitSeq(out, "\x00\x00") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		// Split by single null (between subject and body)
		parts := strings.SplitN(entry, "\x00", 2)
		subject := strings.TrimSpace(parts[0])
		var body string
		if len(parts) > 1 {
			body = strings.TrimSpace(parts[1])
		}
		if subject != "" {
			commits = append(commits, Commit{Subject: subject, Body: body})
		}
	}

	return commits, nil
}

// AbbrevSHA safely abbreviates a SHA to 7 characters.
// Returns the full string if it's shorter than 7 characters.
func AbbrevSHA(sha string) string {
	if len(sha) <= 7 {
		return sha
	}
	return sha[:7]
}
