# gh-stack: Design Document

A GitHub CLI extension for managing stacked pull requests.

## Overview

`gh-stack` tracks parent-child relationships between branches, enabling workflows where PRs target other PRs. When a parent branch is rebased or merged, the tool helps "cascade" those changes to descendants and retarget orphaned branches.

## Data Model

All metadata is stored in `.git/config` under custom keys.

```ini
[stack]
    trunk = main

[branch "feature-a"]
    stackParent = main
    stackPR = 1234

[branch "feature-b"]
    stackParent = feature-a
    stackPR = 1235

[branch "feature-c"]
    stackParent = feature-a
    # No PR yet
```

This represents:
```
main
└── feature-a (#1234)
    ├── feature-b (#1235)
    └── feature-c
```

### Fields

| Key | Description |
|-----|-------------|
| `stack.trunk` | The trunk branch name (e.g., `main`, `master`) |
| `branch.<name>.stackParent` | The branch this branch is stacked on |
| `branch.<name>.stackPR` | The GitHub PR number (set after PR creation) |

### Invariants

1. `trunk` has no `stackParent`
2. Every tracked branch has exactly one `stackParent`
3. The parent graph is acyclic (it's a tree rooted at trunk)
4. A branch's `stackParent` must either be `trunk` or another tracked branch

## Commands

### `gh stack init`

Initialize stack tracking in the repository.

```bash
gh stack init [--trunk <branch>]
```

**Behavior:**
1. If `--trunk` provided, use that; otherwise prompt or default to `main`/`master`
2. Write `stack.trunk` to `.git/config`
3. If trunk doesn't exist locally, error

**Errors:**
- Repository not a git repo
- Trunk branch doesn't exist

---

### `gh stack create <name>`

Create a new branch stacked on the current branch.

```bash
gh stack create <name> [-m <message>] [--empty]
```

**Behavior:**
1. Validate current branch is trunk or tracked
2. Create new branch at current HEAD
3. If staged changes exist (and not `--empty`), commit with message
4. Set `branch.<name>.stackParent` to current branch
5. Check out the new branch

**Errors:**
- Current branch is not tracked (suggest `gh stack adopt`)
- Branch name already exists
- No staged changes and no `--empty` flag (prompt user)

---

### `gh stack adopt [<branch>]`

Start tracking an existing branch.

```bash
gh stack adopt [<branch>] [--parent <parent>]
```

**Behavior:**
1. Default to current branch if `<branch>` not specified
2. If `--parent` given, use that; otherwise prompt with interactive picker
3. Validate parent is trunk or tracked
4. Set `branch.<name>.stackParent`

**Errors:**
- Branch doesn't exist
- Parent is not tracked
- Would create a cycle

---

### `gh stack orphan [<branch>]`

Stop tracking a branch (remove from the tree).

```bash
gh stack orphan [<branch>] [--force]
```

**Behavior:**
1. Default to current branch
2. If branch has children, error unless `--force`
3. If `--force`, also orphan all descendants
4. Remove `stackParent` and `stackPR` from config

**Errors:**
- Branch has children (without `--force`)

---

### `gh stack log`

Display the branch tree.

```bash
gh stack log [--all] [--porcelain]
```

**Behavior:**
1. Print tree structure with branch names, PR numbers, and status
2. Mark current branch with `*`
3. `--porcelain` outputs machine-readable format

**Example output:**
```
main
└── feature-a (#1234 ✓)
    ├── feature-b (#1235 ⋯)
    └── * feature-c
```

Legend: `✓` = approved, `⋯` = pending review, no symbol = no PR

---

### `gh stack cascade`

Rebase current branch onto its parent, then recursively cascade to descendants.

```bash
gh stack cascade [--only] [--dry-run]
```

**Behavior:**
1. `--only`: only cascade current branch, not descendants
2. For current branch: rebase onto parent if needed
3. For each child: recursively cascade
4. On conflict: pause, print instructions, save state for `continue`

**Algorithm:**
```
cascade(branch):
    parent = stackParent(branch)
    if needs_rebase(branch, parent):
        rebase(branch, onto=parent)  # may conflict
    for child in children(branch):
        cascade(child)

needs_rebase(branch, parent):
    return merge_base(branch, parent) != tip(parent)
```

**State file** (on conflict): `.git/STACK_CASCADE_STATE`
```json
{
  "current": "feature-b",
  "pending": ["feature-c", "feature-d"],
  "original_head": "abc123"
}
```

**Errors:**
- Current branch not tracked
- Rebase conflict (pauses, doesn't error)

---

### `gh stack continue`

Continue a cascade after resolving conflicts.

```bash
gh stack continue
```

**Behavior:**
1. Complete the in-progress rebase (`git rebase --continue`)
2. Resume cascading remaining branches from state file
3. Clean up state file when done

**Errors:**
- No cascade in progress
- Rebase not yet resolved (unstaged conflicts)

---

### `gh stack abort`

Abort a cascade in progress.

```bash
gh stack abort
```

**Behavior:**
1. Abort the in-progress rebase
2. Restore original branch positions (if we saved them)
3. Clean up state file

---

### `gh stack sync`

Fetch from origin, detect merged PRs, retarget orphaned branches, cascade all.

```bash
gh stack sync [--no-cascade] [--dry-run]
```

**Behavior:**
1. `git fetch origin`
2. Fast-forward trunk to `origin/trunk`
3. For each tracked branch with a PR:
   - Query GitHub: is PR merged?
   - If merged: retarget children to trunk, mark branch for deletion
4. Validate all parents still exist (handle broken links)
5. Cascade all branches (unless `--no-cascade`)
6. Prompt to delete merged branches

**Algorithm:**
```
sync():
    fetch()
    fast_forward(trunk)
    
    merged = []
    for branch in tracked_branches():
        if pr := get_pr(branch):
            if is_merged(pr):
                merged.append(branch)
    
    for branch in merged:
        for child in children(branch):
            set_parent(child, trunk)
        prompt_delete(branch)
    
    validate_parents()  # check for broken links
    cascade_all()
```

**Errors:**
- Network error querying GitHub
- Broken parent link (see "Broken Parent Links" below)

---

### `gh stack push`

Force-push branches from trunk to current branch, updating PR base branches as needed.

```bash
gh stack push [--dry-run]
```

**Behavior:**
1. Collect all branches from trunk to current (the "downstack")
2. For each branch with a PR, check if the PR's base branch matches `stackParent`
   - If mismatched, update via `gh pr edit --base <stackParent>`
3. Force-push each to origin (with `--force-with-lease`)
4. Print summary

**Example:**
```bash
$ gh stack push
Updating PR #1235 base: feature-a -> main
Pushing 2 branches...
  feature-a -> origin/feature-a (forced)
  feature-b -> origin/feature-b (forced)
```

**Errors:**
- Remote reject (e.g., protected branch)
- Lease failure (remote changed unexpectedly) — print warning, suggest `--force`
- GitHub API error updating PR base

---

### `gh stack pr`

Create or update a PR for the current branch. (Thin wrapper around `gh pr create`)

```bash
gh stack pr [--base <branch>]
```

**Behavior:**
1. Determine base: use `stackParent` unless `--base` overrides
2. Check if PR already exists for this branch
3. If no PR: `gh pr create --base <parent>`, capture PR number, store in config
4. If PR exists: `gh pr edit` to update base if needed

**Note:** This is mostly a convenience wrapper. Users can also just use `gh pr create --base <parent>` directly and then `gh stack link <pr-number>`.

---

### `gh stack link <pr-number>`

Associate an existing PR with the current branch.

```bash
gh stack link <pr-number>
```

**Behavior:**
1. Validate PR exists and is for this branch
2. Store PR number in `branch.<name>.stackPR`

---

### `gh stack unlink`

Remove PR association from current branch.

```bash
gh stack unlink
```

---

## Error Handling

### Broken Parent Links

If a branch's `stackParent` no longer exists (user deleted it outside of `gh-stack`):

```
$ gh stack cascade
error: branch 'feature-b' has parent 'feature-a', but 'feature-a' does not exist

The parent branch may have been deleted outside of gh-stack.
To fix this, either:
  1. Recreate the parent branch, or
  2. Rebase 'feature-b' onto a new parent and run:
       gh stack orphan feature-b
       gh stack adopt feature-b --parent <new-parent>
```

**Detection:** During any operation that walks the tree, validate each `stackParent` exists.

### Cascade Conflicts

On rebase conflict:

```
$ gh stack cascade
Cascading feature-a... ok
Cascading feature-b... CONFLICT

Resolve conflicts and run 'gh stack continue', or 'gh stack abort' to cancel.
Remaining branches: feature-c, feature-d
```

### Dirty Working Tree

Before any operation that might modify branches:

```
$ gh stack cascade
error: working tree has uncommitted changes

Commit or stash your changes before cascading.
```

## Go Package Structure

```
gh-stack/
├── main.go              # Entry point, command dispatch
├── cmd/
│   ├── init.go
│   ├── create.go
│   ├── adopt.go
│   ├── orphan.go
│   ├── log.go
│   ├── cascade.go
│   ├── continue.go
│   ├── abort.go
│   ├── sync.go
│   ├── push.go
│   ├── pr.go
│   ├── link.go
│   └── unlink.go
├── internal/
│   ├── config/
│   │   └── config.go    # Read/write .git/config
│   ├── git/
│   │   └── git.go       # Git operations (rebase, fetch, etc.)
│   ├── tree/
│   │   └── tree.go      # Tree traversal, validation
│   ├── github/
│   │   └── github.go    # PR queries via gh CLI
│   └── state/
│       └── state.go     # Cascade state persistence
└── go.mod
```

## Future Considerations (Out of Scope for MVP)

- **Interactive rebase within stack**: reorder branches
- **Split branch**: extract commits into a new child
- **Fold branch**: merge a branch into its parent
- **Multiple trunks**: support `develop`, `release/*`, etc.
- **Team collaboration**: fetch a coworker's stack
- **Undo**: revert the last gh-stack operation

## Design Decisions

1. **PR base branch updates**: Handled during `gh stack push`, not `sync`. This keeps `sync` as a local-only operation (aside from fetch) and batches remote updates together.

2. **Config location**: Use `.git/config` with a library like `go-git` or `go-ini` for parsing. Git-native approach.

3. **Broken parent recovery**: Keep it simple—show an error with clear instructions. Don't attempt auto-detection of candidate parents (too risky for user to make wrong choice).
