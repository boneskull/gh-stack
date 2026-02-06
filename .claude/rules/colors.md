# Terminal Output Colors

This project follows the [GitHub CLI Primer color conventions](https://github.com/cli/cli/tree/trunk/docs/primer/foundations) for consistent, accessible terminal output.

## Color Scheme

Use only the 8 basic ANSI colors for maximum terminal compatibility:

| Color   | Usage                              | Style Method   |
| ------- | ---------------------------------- | -------------- |
| Green   | Success messages, open states      | `Success()`    |
| Red     | Errors, failures, conflicts        | `Error()`      |
| Yellow  | Warnings, pending states           | `Warning()`    |
| Cyan    | Branch names                       | `Branch()`     |
| Magenta | Merged PRs                         | `Merged()`     |
| Gray    | Secondary text, hints, muted info  | `Muted()`      |
| Bold    | Phase headers, emphasis            | `Bold()`       |

## Icons

Use these Unicode symbols to enhance (not replace) meaning:

| Icon | Meaning  | Style Method      |
| ---- | -------- | ----------------- |
| `✓`  | Success  | `SuccessIcon()`   |
| `✗`  | Failure  | `FailureIcon()`   |
| `!`  | Warning  | `WarningIcon()`   |

## Usage

Import the style package and create an instance:

```go
import "github.com/boneskull/gh-stack/internal/style"

s := style.New()

// Success messages
fmt.Printf("%s Sync complete!\n", s.SuccessIcon())
fmt.Println(s.SuccessMessage("Operation complete"))

// Warnings
fmt.Printf("%s could not fetch PR: %v\n", s.WarningIcon(), err)

// Branch names
fmt.Printf("Rebasing %s onto %s\n", s.Branch(current), s.Branch(parent))

// Phase headers
fmt.Println(s.Bold("=== Phase 1: Cascade ==="))

// Secondary/muted text
fmt.Println(s.Muted("Run 'git config ...' to fix."))
```

## Environment Variables

The style package respects standard terminal conventions:

- `NO_COLOR` - Disables all colors when set (any value)
- `CLICOLOR=0` - Disables colors
- `CLICOLOR_FORCE=1` - Forces colors even in non-TTY
- `GH_FORCE_TTY` - Forces TTY behavior (from go-gh)

## Scriptability

When output is piped (non-TTY), colors are automatically disabled. This ensures:

- Machine-readable output when piped to other commands
- State is communicated via text, not just color
- Compatibility with tools like `grep`, `awk`, `cut`

## Guidelines

1. **Color enhances, never communicates alone** - Always include text that conveys the meaning
2. **Be consistent** - Use the same color for the same semantic meaning
3. **Test without colors** - Run with `NO_COLOR=1` to verify output is still clear
4. **Prefer semantic methods** - Use `Success()` not raw green for success messages
