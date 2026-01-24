# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build and Test Commands

```bash
make build          # Build binary to ./gh-stack
make test           # Run all tests with verbose output
make lint           # Run golangci-lint
make ci             # Run lint, test, build (what CI does)
make tools          # Install golangci-lint v2
make gh-install     # Install as gh extension locally
```

Run a single test:
```bash
go test -v -run TestFunctionName ./cmd
go test -v -run TestFunctionName ./internal/config
```

## Architecture

gh-stack is a GitHub CLI extension for managing stacked pull requests. It stores all metadata locally in `.git/config`.

### Package Structure

```
cmd/           CLI commands (Cobra)
internal/
  config/      Stack metadata in git config (trunk, parent, PR associations)
  git/         Git operations wrapper using safeexec
  github/      GitHub API via go-gh library
  state/       Cascade state persistence (.git/STACK_CASCADE_STATE)
  tree/        Branch tree construction and traversal
```

### Key Concepts

**Stack Metadata Storage**: All stack relationships are stored in `.git/config`:
- `stack.trunk` - The base branch (usually `main`)
- `branch.<name>.stackParent` - Parent branch for `<name>`
- `branch.<name>.stackPR` - Associated PR number

**Tree Model**: `internal/tree` builds an in-memory tree from config. Commands traverse this tree for operations like cascade (rebase descendants) and log (display hierarchy).

**Cascade State**: During multi-branch rebases, state is persisted to `.git/STACK_CASCADE_STATE` allowing continue/abort recovery after conflicts.

### Data Flow

1. Commands load `config.Config` from working directory
2. `tree.Build()` constructs branch hierarchy from config
3. Commands use `git.Git` for git operations, `github.Client` for API calls
4. Changes write back to git config via `config.Config` methods

### GitHub Integration

Uses `go-gh` library (not subprocess calls to `gh`):
- `api.DefaultRESTClient()` - Authenticated REST client using gh's credentials
- `repository.Current()` - Detect repo from git remotes

### Git Operations

`internal/git` uses `safeexec.LookPath("git")` to find git securely (prevents PATH injection on Windows). The path is cached with `sync.Once`.
