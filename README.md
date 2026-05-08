# gh-stack

A [GitHub CLI][] extension for managing stacked pull requests.

## What Are Stacked PRs?

Large pull requests are hard to review. They're intimidating, take forever, and often get rubber-stamped instead of actually reviewed. The obvious fix is to break your work into smaller PRs—but what if PR #2 depends on PR #1? Do you just... wait?

**Stacked PRs** let you keep working. Instead of waiting for your first PR to merge before starting the next, you branch off your in-flight work:

```text
main
└── add-auth          ← PR #1: adds authentication
    └── add-auth-ui   ← PR #2: adds the login form (depends on #1)
        └── add-tests ← PR #3: tests for all of it (depends on #2)
```

Each branch gets its own focused PR. Reviewers see small, digestible changes. You keep your flow.

The catch? Managing these stacks by hand is tedious. When `main` updates, you need to rebase the whole chain in order. When PR #1 merges, you need to retarget PR #2 to point at `main`. Miss a step and you've got a mess.

**gh-stack automates all of that.**

## Installation

Requires:

- [GitHub CLI][] (`gh`) installed and authenticated
- Git 2.38 or newer (for `git rebase --update-refs` support; macOS Command Line Tools and current Linux distributions all ship a compatible version)

```bash
gh extension install boneskull/gh-stack
```

## Quick Start

### Initialize Your Repository

Check out your "trunk" branch (typically `main` or `master`), then initialize stack tracking:

```bash
gh stack init
```

### Create a Stacked Branch

Create a new stacked branch named `feature-auth`:

```bash
gh stack create feature-auth
```

### Create Another Stacked Branch

From the `feature-auth` branch, create a new stacked branch named `feature-auth-tests`:

```bash
gh stack create feature-auth-tests
```

### View Your Stack

From the `feature-auth-tests` branch, let's see an overview of the stack:

```bash
gh stack log
```

```text
main
└── feature-auth
    └── * feature-auth-tests
```

### Keep a Local Stack In Sync

#### Scenario 1: Changes in a local stacked branch

Say we've made changes in `feature-auth`. To keep the stack in sync, we will need to rebase `feature-auth-tests` onto `feature-auth`. From branch `feature-auth`, execute:

```bash
gh stack restack
```

If you run into conflicts, resolve them and run `gh stack continue` to resume the restack (or `gh stack abort` to cancel). Once complete, your local stacks will be in sync. _They won't yet be pushed to the remote repository._

#### Scenario 2: Changes in the local trunk

Maybe we pulled down `main` and it has new commits. We'll use the same strategy as above, but this time from the `main` branch:

```bash
gh stack restack
```

> [!NOTE]
>
> Since `main` (the trunk) is the parent of every stack, `gh stack restack` will naturally restack _all_ stacks.

#### Scenario 3: Upstream changes

Say `feature-auth` has been merged into the remote `main`. We now need to restack the changes, but also retarget `feature-auth-tests` to `main` from `feature-auth`. You'll want to run:

```bash
gh stack sync
```

This will:

1. Fetch from origin
2. Fast-forward the trunk
3. Detect merged PRs
4. Clean up merged branches
5. Retarget orphaned children to trunk
6. Restack all branches

What it _won't_ do is push back up to the remote; see the [next section](#creating--updating-prs) for that.

### Creating & Updating PRs

To create PRs for the entire stack, run from any branch:

```bash
gh stack submit
```

This pushes every branch in the stack (in parent-to-child order) and creates or updates their PRs. You can run it again whenever you need to push changes or update PRs.

To submit only the current branch and its descendants (the old default), use `--from`:

```bash
gh stack submit --from
```

> [!TIP]
>
> `gh stack submit` does everything `gh stack restack` does, and then some. Generally, if you want to make local mid-stack changes _without_ pushing to the remote, you'll want `gh stack restack`; otherwise just use `gh stack submit`.

## Commands

| Command    | Description                                         |
| ---------- | --------------------------------------------------- |
| `init`     | Initialize stack tracking with trunk branch         |
| `log`      | Display branch tree                                 |
| `create`   | Create new branch stacked on current                |
| `adopt`    | Start tracking an existing branch                   |
| `orphan`   | Stop tracking a branch                              |
| `link`     | Associate PR number with branch                     |
| `unlink`   | Remove PR association                               |
| `submit`   | Restack, push, and create/update PRs in one command |
| `restack`  | Rebase branch and descendants onto parents          |
| `continue` | Resume operation after conflict resolution          |
| `abort`    | Cancel in-progress operation                        |
| `sync`     | Full sync: fetch, cleanup merged PRs, restack all   |
| `undo`     | Undo the last destructive operation                 |

## Command Reference

### init

Initialize stack tracking in the repository. This must be run once before using other commands.

By default, `init` auto-detects the trunk branch (`main` or `master`). If neither exists, you must specify one with `--trunk`.

#### init Flags

| Flag      | Description                                          |
| --------- | ---------------------------------------------------- |
| `--trunk` | Trunk branch name (default: auto-detect main/master) |

### log

Display the branch tree showing the stack hierarchy, current branch, and associated PR numbers.

#### log Flags

| Flag              | Description                           |
| ----------------- | ------------------------------------- |
| `-a, --all`       | Show all branches                     |
| `-p, --porcelain` | Machine-readable tab-separated output |

#### Porcelain Format

When using `--porcelain`, output is tab-separated with fields:

```text
BRANCH    PARENT    PR_NUMBER    IS_CURRENT    PR_URL
```

### create

Create a new branch stacked on the current branch.

If you have staged changes, you can commit them as part of creating the new branch by providing a commit message with `-m`. To create the branch without committing staged changes, use `--empty`.

#### create Usage

```bash
gh stack create <name>
```

#### create Flags

| Flag              | Description                                     |
| ----------------- | ----------------------------------------------- |
| `-m, --message`   | Commit message for staged changes               |
| `-C, --no-commit` | Create branch without committing staged changes |

### adopt

Start tracking an existing branch by setting its parent.

By default, adopts the current branch. The parent must be either the trunk or another tracked branch. If the branch is already tracked, `adopt` updates its parent to the specified branch (or does nothing if the parent is unchanged).

#### adopt Usage

```bash
gh stack adopt <parent>
```

#### adopt Flags

| Flag           | Description                               |
| -------------- | ----------------------------------------- |
| `-b, --branch` | Branch to adopt (default: current branch) |

### orphan

Stop tracking a branch by removing it from the stack tree.

If the branch has children, you must use `--force` to orphan both the branch and all its descendants.

#### orphan Usage

```bash
gh stack orphan [branch]
```

If no branch is specified, orphans the current branch.

#### orphan Flags

| Flag          | Description                 |
| ------------- | --------------------------- |
| `-f, --force` | Also orphan all descendants |

### link

Associate an existing GitHub PR number with the current branch.

This is useful when you've created a PR manually (outside of `gh stack submit`) and want gh-stack to track it.

#### link Usage

```bash
gh stack link <pr-number>
```

### unlink

Remove the PR association from the current branch.

The PR itself is not affected; this only removes the local tracking.

### submit

Restack, push, and create/update PRs for the entire stack.

This is the primary workflow command. By default it processes **every tracked branch** in parent-before-child order. It performs three phases:

1. **Restack**: Rebase affected branches onto their parents
2. **Push**: Force-push all affected branches (using `--force-with-lease`)
3. **PR**: Create PRs for branches without them; update PR bases for existing PRs

PRs targeting non-trunk branches are created as drafts. When a PR's base changes to trunk (after its parent merges), you'll be prompted to mark it ready for review.

Use `--from` to limit the scope to a subtree instead of the full stack. A bare `--from` (no value) starts from the current branch, preserving the pre-v0.x behavior.

If a rebase conflict occurs, resolve it and run `gh stack continue`.

#### submit Flags

| Flag                  | Description                                                            |
| --------------------- | ---------------------------------------------------------------------- |
| `-D, --dry-run`       | Show what would happen without doing it                                    |
| `-f, --from [branch]` | Submit from this branch toward leaves (bare `--from` = current branch)     |
| `-c, --current`       | Only submit the current branch, not descendants                            |
| `-u, --update`        | Only update existing PRs, don't create new ones                            |
| `-s, --skip-prs`      | Skip PR creation/update, only restack and push                             |
| `-y, --yes`           | Skip interactive prompts; use auto-generated PR title/description          |
| `--web`               | Open created/updated PRs in web browser                                    |
| `--no-update-refs`    | Do not pass `--update-refs` to git (preserves untracked bookmark branches) |

### restack

Rebase the current branch and its descendants onto their parents. Aliased as `cascade`.

Use this when you've made local changes and want to keep your stack in sync without pushing or creating PRs. For a full submit workflow, use `gh stack submit` instead.

If a rebase conflict occurs, resolve it and run `gh stack continue`.

#### restack Flags

| Flag                 | Description                                                                |
| -------------------- | -------------------------------------------------------------------------- |
| `-c, --current`      | Only restack current branch, not descendants                               |
| `-D, --dry-run`      | Show what would be done                                                    |
| `-w, --worktrees`    | Rebase branches checked out in linked worktrees in-place                   |
| `--no-update-refs`   | Do not pass `--update-refs` to git (preserves untracked bookmark branches) |

### continue

Continue a restack or submit operation after resolving rebase conflicts.

After resolving conflicts and staging the changes, run this command to resume the operation.

### abort

Abort a restack or submit operation in progress.

This aborts any in-progress rebase and cleans up the operation state. Your branches will be left in their pre-operation state.

### sync

Full sync: fetch from origin, detect merged PRs, clean up merged branches, retarget orphaned children, and restack all branches.

This is the command to run when upstream changes have occurred (e.g., a PR in your stack was merged). It handles the bookkeeping of updating your local stack to match remote state.

#### sync Flags

| Flag              | Description                                              |
| ----------------- | -------------------------------------------------------- |
| `--no-restack`      | Skip restacking branches (might not work well!)                            |
| `-D, --dry-run`     | Show what would be done                                                    |
| `-w, --worktrees`   | Rebase branches checked out in linked worktrees in-place                   |
| `--no-update-refs`  | Do not pass `--update-refs` to git (preserves untracked bookmark branches) |

### undo

Undo the last destructive operation (restack, submit, or sync) by restoring branches to their pre-operation state.

Before any destructive operation, gh-stack automatically captures a snapshot of affected branches. If something goes wrong or you change your mind, `undo` restores:

- Branch refs (SHAs)
- Stack configuration (parent, PR number, fork point)
- Any auto-stashed uncommitted changes

Snapshots are stored in `.git/stack-undo/` and archived to `.git/stack-undo/done/` after successful undo. Snapshots are automatically pruned to keep at most 50 pending and 50 archived, with the oldest removed first. No manual cleanup is required.

> [!NOTE]
>
> `undo` only affects local state. It cannot undo remote changes like force-pushes. If you've already pushed, you may need to force-push again after undoing.

#### undo Flags

| Flag            | Description                                  |
| --------------- | -------------------------------------------- |
| `-f, --force`   | Skip confirmation prompt                     |
| `-D, --dry-run` | Show what would be restored without doing it |

## Working with Git Worktrees

If you use [git worktrees](https://git-scm.com/docs/git-worktree) to check out multiple stack branches simultaneously, `git checkout` will refuse to switch to a branch that's already checked out in another worktree. This means `restack` and `sync` will fail when they try to check out those branches.

The `--worktrees` flag solves this. When set, **gh-stack** detects linked worktrees up front and rebases those branches directly in their worktree directories instead of checking them out:

```bash
# Restack with worktree-aware rebasing
gh stack restack --worktrees

# Sync with worktree-aware rebasing
gh stack sync --worktrees
```

If a rebase conflict occurs in a worktree branch, **gh-stack** will tell you which worktree directory to resolve it in. After resolving, run `gh stack continue` from the main repository as usual—**gh-stack** remembers which worktree the conflict lives in.

> [!NOTE]
>
> The `--worktrees` flag is opt-in. Without it, **gh-stack** behaves exactly as before. If none of your stack branches are checked out in linked worktrees, the flag is a harmless no-op.

> [!NOTE]
>
> When linked worktrees are detected, **gh-stack** automatically passes `--no-update-refs` to git. This prevents silent stack corruption: git's `--update-refs` silently skips any ref that is checked out in a linked worktree, which would leave the chain in a broken state. If no branches are checked out in linked worktrees (even with `--worktrees` set), **gh-stack** passes `--update-refs` normally so that any untracked bookmark branches pointing into the stack are kept in sync.

## How It Works

**gh-stack** stores metadata in your local `.git/config`:

```ini
[stack]
    trunk = main

[branch "feature-auth"]
    stackParent = main
    stackPR = 123

[branch "feature-auth-tests"]
    stackParent = feature-auth
    stackPR = 124
```

No remote service required. Your stack relationships stay with your repository.

## Comparison

### vs. Graphite

[Graphite](https://graphite.dev/) is a SaaS product with a polished CLI (`gt`) and web dashboard. It requires an account and stores stack metadata on their servers.

Graphite's CLI is significantly more feature-rich:

| Feature                               | Graphite (`gt`) | `gh-stack` |
| ------------------------------------- | :-------------: | :--------: |
| Create / track / untrack branches     |       ✅        |     ✅     |
| View stack hierarchy                  |       ✅        |     ✅     |
| Sync with trunk                       |       ✅        |     ✅     |
| Create & update PRs                   |       ✅        |     ✅     |
| Conflict recovery (continue/abort)    |       ✅        |     ✅     |
| Undo last operation                   |       ✅        |     ✅     |
| Stack navigation (up/down/top/bottom) |       ✅        |     ❌     |
| Move branch to new parent             |       ✅        |     ❌     |
| Fold / split / reorder branches       |       ✅        |     ❌     |
| Absorb changes into downstack         |       ✅        |     ❌     |
| Amend / squash commits                |       ✅        |     ❌     |
| Fetch teammate's stack                |       ✅        |     ❌     |
| Freeze branch to prevent edits        |       ✅        |     ❌     |
| Web dashboard & PR inbox              |       ✅        |     ❌     |
| AI code review                        |       ✅        |     ❌     |
| Merge queue                           |       ✅        |     ❌     |
| Works offline (no account required)   |       ❌        |     ✅     |
| Open source                           |       ❌        |     ✅     |

If you want the kitchen sink—stack navigation, branch surgery, a web UI, AI reviews, merge queues—Graphite is hard to beat. If you want a lightweight, open-source tool that handles the core stacking workflow without an account or remote dependency, that's what **gh-stack** is for.

### vs. spr

[spr](https://github.com/ejoffe/spr) enforces a strict "one commit = one PR" model. You work on a single branch, and each commit automatically becomes a separate PR. You cannot merge PRs through GitHub's UI—you must use `spr merge`.

**gh-stack** uses a traditional "one branch = one PR" model. You control what goes into each PR, create PRs when you're ready, and merge through GitHub normally. More flexibility, less automation.

### vs. git-town

[git-town](https://github.com/git-town/git-town) is a general-purpose Git workflow tool that automates branch creation, synchronization, and cleanup across many workflows (Git Flow, GitHub Flow, trunk-based development). Stacked changes are one feature among many.

**gh-stack** focuses exclusively on stacked PRs. If you want a comprehensive Git workflow tool, use git-town. If you want a lightweight tool just for managing PR stacks on GitHub, use **gh-stack**.

### vs. git-branchless

[git-branchless](https://github.com/arxanas/git-branchless) is a powerful suite that enhances Git with undo functionality, interactive commit graph editing, and patch-stack workflows. It's designed for power users and optimized for massive repositories.

**gh-stack** is narrower in scope: it tracks parent-child relationships between branches and helps you manage the resulting PRs. It doesn't modify how Git works—it just adds stack awareness on top.

### vs. git-spice

[git-spice](https://abhinav.github.io/git-spice/) is a stacking tool that stores all operational state as Git objects in a local ref (`refs/spice/data`). Every state mutation creates a new commit in that ref, giving you a full history of every change to your stack metadata—explorable with `git log --patch refs/spice/data`. It supports both GitHub and GitLab.

**gh-stack** stores stack metadata directly in `.git/config` as standard Git configuration keys, with operation recovery and undo handled by plain JSON files in `.git/`. This means:

- **Faster writes.** Setting a config key is a single `git config` subprocess call. Creating a Git object requires hashing, compressing, writing a blob, writing a tree, writing a commit, and updating the ref.
- **Easier debugging.** You can inspect and repair state with `git config --edit` or a text editor. No need for `git cat-file` or `git log` on an internal ref.
- **No state history.** **git-spice** gets a full audit log for free. **gh-stack** provides multi-level undo via separate snapshot files instead, which covers the common case (undoing the last operation) without the overhead.

## Project Scope

- **gh-stack** aims to be a minimal alternative to Graphite for those who do not need its full feature set
- **gh-stack** wants to support only the minimum set of operations needed to manage stacked PRs
- Being a [GitHub CLI][] extension, **gh-stack** will not support other Git hosting services

## Development

To build from source, you'll need Go 1.25+.

```bash
gh repo clone boneskull/gh-stack
cd gh-stack
make tools        # Install development tools
make build        # Build binary to ./gh-stack
make test         # Run tests
make lint         # Run linter
make gh-install   # Install as gh extension locally
```

See [ARCHITECTURE.md](ARCHITECTURE.md) for a detailed breakdown of **gh-stack**'s data storage approach.

## Acknowledgements

- Inspired by [Graphite][].

## Contributors

<!-- ALL-CONTRIBUTORS-LIST:START - Do not remove or modify this section -->
<!-- prettier-ignore-start -->
<!-- markdownlint-disable -->
<table>
  <tbody>
    <tr>
      <td align="center" valign="top" width="14.28%"><a href="https://github.com/mvanhorn"><img src="https://avatars.githubusercontent.com/u/455140?v=4?s=100" width="100px;" alt="Matt Van Horn"/><br /><sub><b>Matt Van Horn</b></sub></a><br /><a href="#code-mvanhorn" title="Code">💻</a></td>
    </tr>
  </tbody>
</table>

<!-- markdownlint-restore -->
<!-- prettier-ignore-end -->

<!-- ALL-CONTRIBUTORS-LIST:END -->

## License

Copyright © 2026 [Christopher "boneskull" Hiller][boneskull]. Licensed under [BlueOak-1.0.0](LICENSE.md).

[GitHub CLI]: https://cli.github.com/
[boneskull]: https://github.com/boneskull
[graphite]: https://graphite.dev/
