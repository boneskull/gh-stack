# Design: Auto-push in `gh stack pr`

**Date:** 2026-01-28
**Status:** Approved

## Summary

Modify `gh stack pr` to automatically push the current branch to the remote (with tracking) before creating or updating a PR. This removes the manual `git push` step currently required.

## Motivation

Currently, `gh stack pr` assumes the branch already exists on the remote. If it doesn't, the GitHub API call fails. Users must remember to push before running `gh stack pr`, which adds friction to the workflow.

## Design

### Flow

```
gh stack pr
    |
    +-> Push branch to origin (force-with-lease, set upstream)
    |       |
    |       +-> Success -> Continue to PR operation
    |       +-> Failure -> Abort with clear error message
    |
    +-> Create or update PR (existing logic)
```

### Key Behaviors

- Always pushes before any PR operation (create or update)
- Uses `git push -u --force-with-lease origin <branch>`
- Fails the entire command if push fails
- No new flags--this becomes the default behavior

### Why Force-with-lease

Stacked PR workflows involve frequent rebasing. Regular `git push` would fail after every rebase. Force-with-lease allows the push while still protecting against overwriting commits you haven't seen locally.

## Implementation

### Changes to `internal/git/git.go`

Add a new method:

```go
// PushWithUpstream pushes a branch to origin and sets up tracking.
// Uses --force-with-lease for safety when rebasing.
func (g *Git) PushWithUpstream(branch string) error {
    return g.runInteractive("push", "-u", "--force-with-lease", "origin", branch)
}
```

### Changes to `cmd/pr.go`

Insert push call in `runPR()` after validating the branch is tracked (after line ~56), before checking if PR exists:

```go
// Push branch to remote (with tracking) before PR operation
fmt.Printf("Pushing %s to origin...\n", branch)
if err := g.PushWithUpstream(branch); err != nil {
    return fmt.Errorf("failed to push branch: %w", err)
}
```

### Files Changed

- `internal/git/git.go` - Add `PushWithUpstream` method
- `internal/git/git_test.go` - Add test for new method
- `cmd/pr.go` - Add push call before PR operations

## Error Handling

| Scenario | Behavior |
|----------|----------|
| No network | Command fails with git's error message |
| Diverged history (remote has unseen commits) | Force-with-lease rejects push, command fails |
| No permission | Command fails with git's error message |
| Branch already up-to-date | Push succeeds quickly, continues to PR |
| First push (no tracking) | `-u` sets up tracking, succeeds |

## Testing

### Unit Tests

Add test for `PushWithUpstream` in `git_test.go` verifying correct arguments are passed.

### Manual Testing Checklist

- [ ] `gh stack pr` on branch not yet pushed -> pushes, creates PR
- [ ] `gh stack pr` on branch with existing PR -> pushes, updates base
- [ ] `gh stack pr` when push fails -> clear error, no PR created

## Alternatives Considered

1. **Error with hint** - Fail with "Branch not pushed. Run `gh stack push` first." Rejected: adds friction without benefit.

2. **Interactive prompt** - Ask before pushing. Rejected: adds unnecessary step for common case.

3. **Regular push (no force)** - Would fail after every rebase. Rejected: doesn't fit stacked workflow.

4. **Push only when creating PR** - Leave updates alone. Rejected: inconsistent behavior, PRs wouldn't reflect local state.
