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

## Usage

### Initialize a Repository

```bash
gh stack init
```

This sets your current branch (typically `main`) as the trunk.

### Create a Stacked Branch

```bash
gh stack create feature-auth
```

Creates `feature-auth` branched from your current position and tracks it as a child.

> ![TIP]
>
> `gh stack create` is sugar for this:
>
> ```bash
> git checkout -b feature-auth
> gh stack adopt feature-auth
> ```

### View Your Stack

```bash
gh stack log
```

```text
main
└── feature-auth
    └── feature-auth-tests
```

### Create PRs for Your Stack

```bash
gh stack pr
```

Creates a PR targeting the parent branch. If a PR already exists, updates its base.

### Push Your Stack

```bash
gh stack push
```

Force-pushes (with lease) all branches from trunk to your current branch, updating PR bases as needed.

### Rebase After Parent Changes

```bash
gh stack cascade
```

Rebases the current branch onto its parent, then cascades to all descendants. If conflicts occur:

```bash
# Resolve conflicts, then:
gh stack continue

# Or abort:
gh stack abort
```

### Sync Everything

```bash
gh stack sync
```

Fetches from origin, fast-forwards trunk, detects merged PRs, cleans up merged branches, retargets orphaned children to trunk, and cascades all branches.

## Commands

| Command    | Description                                           |
| ---------- | ----------------------------------------------------- |
| `init`     | Initialize stack tracking with trunk branch           |
| `log`      | Display branch tree                                   |
| `create`   | Create new branch stacked on current                  |
| `adopt`    | Start tracking an existing branch                     |
| `orphan`   | Stop tracking a branch                                |
| `link`     | Associate PR number with branch                       |
| `unlink`   | Remove PR association                                 |
| `pr`       | Create or update PR targeting parent                  |
| `push`     | Force-push stack with `--force-with-lease`            |
| `cascade`  | Rebase branch and descendants onto parents            |
| `continue` | Resume cascade after conflict resolution              |
| `abort`    | Cancel cascade operation                              |
| `sync`     | Full sync: fetch, cleanup merged PRs, cascade all     |

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

To build from source, you'll need Go 1.22+.

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
