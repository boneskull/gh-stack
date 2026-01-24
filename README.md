# gh-stack

A GitHub CLI extension for managing stacked pull requests.

## Features

- **Local-first** — Stack metadata lives in `.git/config`, not a remote service
- **GitHub-native** — Works with the `gh` CLI you already use
- **Conflict-aware** — Cascade rebases with continue/abort recovery
- **Automatic sync** — Detects merged PRs, retargets orphaned branches, cascades all

## Quick Start

### Prerequisites

- Go 1.22+
- [GitHub CLI](https://cli.github.com/) (`gh`) installed and authenticated

### Installation

```bash
# Build from source
git clone https://github.com/boneskull/gh-stack.git
cd gh-stack
make build

# Install as gh extension
make gh-install
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

gh-stack stores metadata in your local `.git/config`:

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

## Acknowledgements

Inspired by [Graphite](https://graphite.dev/).

## License

Copyright © 2026 [Christopher "boneskull" Hiller](https://github.com/boneskull). Licensed under [Apache-2.0](LICENSE).
