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

Requires [GitHub CLI][] (`gh`) installed and authenticated.

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
gh stack cascade
```

If you run into conflicts, resolve them and run `gh stack continue` to resume the cascade (or `gh stack abort` to cancel). Once complete, your local stacks will be in sync. _They won't yet be pushed to the remote repository._

#### Scenario 2: Changes in the local trunk

Maybe we pulled down `main` and it has new commits. We'll use the same strategy as above, but this time from the `main` branch:

```bash
gh stack cascade
```

> [!NOTE]
>
> Since `main` (the trunk) is the parent of every stack, `gh stack cascade` will naturally cascade _all_ stacks.

#### Scenario 3: Upstream changes

Say `feature-auth` has been merged into the remote `main`. We now need to cascade the changes, but also retarget `feature-auth-tests` to `main` from `feature-auth`. You'll want to run:

```bash
gh stack sync
```

This will:

1. Fetch from origin
2. Fast-forward the trunk
3. Detect merged PRs
4. Clean up merged branches
5. Retarget orphaned children to trunk
6. Cascade all branches

What it _won't_ do is push back up to the remote; see the [next section](#creating--updating-prs) for that.

### Creating & Updating PRs

To create PRs for the `feature-auth` and `feature-auth-tests` branches, execute this from the `feature-auth` branch:

```bash
gh stack submit
```

Whenever you need to push these branches again, or update the PRs, you can run `gh stack submit` again.

> [!TIP]
>
> `gh stack submit` does everything `gh stack cascade` does, and then some. Generally, if you want to make local mid-stack changes _without_ pushing to the remote, you'll want `gh stack cascade`; otherwise just use `gh stack submit`.

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
| `submit`   | Cascade, push, and create/update PRs in one command |
| `cascade`  | Rebase branch and descendants onto parents          |
| `continue` | Resume operation after conflict resolution          |
| `abort`    | Cancel in-progress operation                        |
| `sync`     | Full sync: fetch, cleanup merged PRs, cascade all   |
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

| Flag          | Description                           |
| ------------- | ------------------------------------- |
| `--all`       | Show all branches                     |
| `--porcelain` | Machine-readable tab-separated output |

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

| Flag            | Description                                     |
| --------------- | ----------------------------------------------- |
| `-m, --message` | Commit message for staged changes               |
| `--empty`       | Create branch without committing staged changes |

### adopt

Start tracking an existing branch by setting its parent.

By default, adopts the current branch. The parent must be either the trunk or another tracked branch.

#### adopt Usage

```bash
gh stack adopt <parent>
```

#### adopt Flags

| Flag       | Description                               |
| ---------- | ----------------------------------------- |
| `--branch` | Branch to adopt (default: current branch) |

### orphan

Stop tracking a branch by removing it from the stack tree.

If the branch has children, you must use `--force` to orphan both the branch and all its descendants.

#### orphan Usage

```bash
gh stack orphan [branch]
```

If no branch is specified, orphans the current branch.

#### orphan Flags

| Flag      | Description                 |
| --------- | --------------------------- |
| `--force` | Also orphan all descendants |

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

Cascade, push, and create/update PRs for current branch and descendants.

This is the primary workflow command. It performs three phases:

1. **Cascade**: Rebase current branch and descendants onto their parents
2. **Push**: Force-push all affected branches (using `--force-with-lease`)
3. **PR**: Create PRs for branches without them; update PR bases for existing PRs

PRs targeting non-trunk branches are created as drafts. When a PR's base changes to trunk (after its parent merges), you'll be prompted to mark it ready for review.

If a rebase conflict occurs, resolve it and run `gh stack continue`.

#### submit Flags

| Flag             | Description                                     |
| ---------------- | ----------------------------------------------- |
| `--dry-run`      | Show what would happen without doing it         |
| `--current-only` | Only submit the current branch, not descendants |
| `--update-only`  | Only update existing PRs, don't create new ones |

### cascade

Rebase the current branch and its descendants onto their parents.

Use this when you've made local changes and want to keep your stack in sync without pushing or creating PRs. For a full submit workflow, use `gh stack submit` instead.

If a rebase conflict occurs, resolve it and run `gh stack continue`.

#### cascade Flags

| Flag        | Description                                  |
| ----------- | -------------------------------------------- |
| `--only`    | Only cascade current branch, not descendants |
| `--dry-run` | Show what would be done                      |

### continue

Continue a cascade or submit operation after resolving rebase conflicts.

After resolving conflicts and staging the changes, run this command to resume the operation.

### abort

Abort a cascade or submit operation in progress.

This aborts any in-progress rebase and cleans up the operation state. Your branches will be left in their pre-operation state.

### sync

Full sync: fetch from origin, detect merged PRs, clean up merged branches, retarget orphaned children, and cascade all branches.

This is the command to run when upstream changes have occurred (e.g., a PR in your stack was merged). It handles the bookkeeping of updating your local stack to match remote state.

#### sync Flags

| Flag           | Description             |
| -------------- | ----------------------- |
| `--no-cascade` | Skip cascading branches |
| `--dry-run`    | Show what would be done |

### undo

Undo the last destructive operation (cascade, submit, or sync) by restoring branches to their pre-operation state.

Before any destructive operation, gh-stack automatically captures a snapshot of affected branches. If something goes wrong or you change your mind, `undo` restores:

- Branch refs (SHAs)
- Stack configuration (parent, PR number, fork point)
- Any auto-stashed uncommitted changes

Snapshots are stored in `.git/stack-undo/` and archived to `.git/stack-undo/done/` after successful undo. Snapshots are automatically pruned to keep at most 50 pending and 50 archived, with the oldest removed first. No manual cleanup is required.

> [!NOTE]
>
> `undo` only affects local state. It cannot undo remote changes like force-pushes. If you've already pushed, you may need to force-push again after undoing.

#### undo Flags

| Flag        | Description                              |
| ----------- | ---------------------------------------- |
| `--force`   | Skip confirmation prompt                 |
| `--dry-run` | Show what would be restored without doing it |

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

[Graphite](https://graphite.dev/) is a SaaS product with a polished CLI and web dashboard. It requires an account and stores stack metadata on their servers. **gh-stack** stores everything locally in `.git/config`—no account, no remote dependency.

### vs. spr

[spr](https://github.com/ejoffe/spr) enforces a strict "one commit = one PR" model. You work on a single branch, and each commit automatically becomes a separate PR. You cannot merge PRs through GitHub's UI—you must use `spr merge`.

**gh-stack** uses a traditional "one branch = one PR" model. You control what goes into each PR, create PRs when you're ready, and merge through GitHub normally. More flexibility, less automation.

### vs. git-town

[git-town](https://github.com/git-town/git-town) is a general-purpose Git workflow tool that automates branch creation, synchronization, and cleanup across many workflows (Git Flow, GitHub Flow, trunk-based development). Stacked changes are one feature among many.

**gh-stack** focuses exclusively on stacked PRs. If you want a comprehensive Git workflow tool, use git-town. If you want a lightweight tool just for managing PR stacks on GitHub, use **gh-stack**.

### vs. git-branchless

[git-branchless](https://github.com/arxanas/git-branchless) is a powerful suite that enhances Git with undo functionality, interactive commit graph editing, and patch-stack workflows. It's designed for power users and optimized for massive repositories.

**gh-stack** is narrower in scope: it tracks parent-child relationships between branches and helps you manage the resulting PRs. It doesn't modify how Git works—it just adds stack awareness on top.

## Development

To build from source, you'll need Go 1.25+.

```bash
gh repo clone boneskull/gh-stack
cd gh-stack
make build        # Build binary to ./gh-stack
make test         # Run tests
make lint         # Run linter
make gh-install   # Install as gh extension locally
```

## Acknowledgements

- Inspired by [Graphite][].

## License

Copyright © 2026 [Christopher "boneskull" Hiller][boneskull]. Licensed under [Apache-2.0](LICENSE).

[GitHub CLI]: https://cli.github.com/
[boneskull]: https://github.com/boneskull
[graphite]: https://graphite.dev/
