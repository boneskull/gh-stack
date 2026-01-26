# E2E Tests

End-to-end tests for gh-stack that compile and run the actual binary against isolated git repositories.

## Running

```bash
# Run all E2E tests
make e2e

# Run specific test
go test -v ./e2e/ -run TestCascade

# Run with race detection
go test -race -v ./e2e/...

# Run unit tests only (faster)
make test-unit

# Run everything
make test
```

## Architecture

### Files

- `main_test.go` - TestMain compiles binary once before all tests
- `helpers_test.go` - TestEnv struct, Result type, core helpers
- `scenario_helpers_test.go` - Conflict and remote simulation helpers
- `smoke_test.go` - Basic smoke test
- `init_create_test.go` - Init and create command tests
- `cascade_test.go` - Cascade tests including conflict recovery
- `adopt_orphan_test.go` - Adopt and orphan command tests
- `push_test.go` - Push command tests
- `error_cases_test.go` - Error handling tests
- `chaos_manual_git_test.go` - Manual git operation resilience
- `chaos_remote_test.go` - Remote sync scenario tests

### How It Works

1. **TestMain** compiles `gh-stack` binary once to a temp directory
2. Each test creates an isolated git repo via `NewTestEnv(t)` using `t.TempDir()`
3. Tests execute the binary via subprocess and verify behavior
4. Temp directories are automatically cleaned up by Go's testing framework

### TestEnv

`TestEnv` provides:
- `Run(args...)` - Execute gh-stack, returns Result (doesn't fail on error)
- `MustRun(args...)` - Execute gh-stack, fails test on non-zero exit
- `Git(args...)` - Execute git commands directly
- `WriteFile(name, content)` - Create files in the test repo
- `CreateCommit(msg)` - Create a commit with a unique file
- `Assert*()` - Various assertions (branch, ancestry, rebase state)

### Test Categories

| Category | Description |
|----------|-------------|
| Happy path | Normal workflows (init, create, cascade, push) |
| Error recovery | Conflicts, abort/continue, dirty working tree |
| Chaos tests | Manual git operations, remote sync edge cases |

## Writing Tests

```go
func TestMyScenario(t *testing.T) {
    // Create isolated git repo
    env := NewTestEnv(t)  // or NewTestEnvWithRemote(t) for push tests

    // Initialize stack
    env.MustRun("init")

    // Use env.Run() for commands that might fail
    result := env.Run("some-command")
    if result.Failed() {
        // handle or check error
    }

    // Use env.MustRun() for commands that must succeed
    env.MustRun("create", "feature-1")

    // Use env.Git() for direct git operations
    env.Git("checkout", "main")

    // Use assertions
    env.AssertBranch("feature-1")
    env.AssertStackParent("feature-1", "main")
    env.AssertAncestor("main", "feature-1")
}
```

## CI

E2E tests run as part of the standard test suite in GitHub Actions. The CI workflow configures git identity for the tests:

```yaml
- name: Configure git for E2E tests
  run: |
    git config --global user.email "ci@github.com"
    git config --global user.name "CI"
    git config --global init.defaultBranch main
```
