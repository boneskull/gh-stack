// Package style provides consistent terminal output styling for gh-stack.
// It follows GitHub CLI color conventions and respects NO_COLOR and TTY detection.
package style

import (
	"fmt"
	"os"
	"sync"

	"github.com/cli/go-gh/v2/pkg/term"
	"github.com/mgutz/ansi"
)

// Style provides methods for styling terminal output.
// All methods return plain text when colors are disabled.
type Style struct {
	enabled bool
}

var (
	// Color functions using basic ANSI colors for maximum compatibility.
	green   = ansi.ColorFunc("green")
	red     = ansi.ColorFunc("red")
	yellow  = ansi.ColorFunc("yellow")
	cyan    = ansi.ColorFunc("cyan")
	magenta = ansi.ColorFunc("magenta")
	gray    = ansi.ColorFunc("black+h") // bright black = gray
	bold    = ansi.ColorFunc("default+b")

	// Cached terminal state.
	termState     term.Term
	termStateOnce sync.Once
)

func getTermState() term.Term {
	termStateOnce.Do(func() {
		termState = term.FromEnv()
	})
	return termState
}

// isColorEnabled returns true if color output should be used.
// Respects NO_COLOR, CLICOLOR, CLICOLOR_FORCE, and GH_FORCE_TTY.
func isColorEnabled() bool {
	// NO_COLOR takes precedence (https://no-color.org/)
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return false
	}

	// Use go-gh's term package which respects GH_FORCE_TTY, CLICOLOR, etc.
	return getTermState().IsColorEnabled()
}

// New creates a new Style instance.
// Colors are automatically enabled/disabled based on terminal capabilities.
func New() *Style {
	return &Style{enabled: isColorEnabled()}
}

// NewWithColor creates a Style with explicit color setting.
// Useful for testing or forcing color on/off.
func NewWithColor(enabled bool) *Style {
	return &Style{enabled: enabled}
}

// Enabled returns whether colors are enabled.
func (s *Style) Enabled() bool {
	return s.enabled
}

// Success styles text for success messages (green).
func (s *Style) Success(t string) string {
	if !s.enabled {
		return t
	}
	return green(t)
}

// Successf styles formatted text for success messages (green).
func (s *Style) Successf(format string, args ...any) string {
	return s.Success(fmt.Sprintf(format, args...))
}

// Warning styles text for warning messages (yellow).
func (s *Style) Warning(t string) string {
	if !s.enabled {
		return t
	}
	return yellow(t)
}

// Warningf styles formatted text for warning messages (yellow).
func (s *Style) Warningf(format string, args ...any) string {
	return s.Warning(fmt.Sprintf(format, args...))
}

// Error styles text for error messages (red).
func (s *Style) Error(t string) string {
	if !s.enabled {
		return t
	}
	return red(t)
}

// Errorf styles formatted text for error messages (red).
func (s *Style) Errorf(format string, args ...any) string {
	return s.Error(fmt.Sprintf(format, args...))
}

// Branch styles branch names (cyan).
func (s *Style) Branch(t string) string {
	if !s.enabled {
		return t
	}
	return cyan(t)
}

// Branchf styles formatted branch names (cyan).
func (s *Style) Branchf(format string, args ...any) string {
	return s.Branch(fmt.Sprintf(format, args...))
}

// Merged styles merged PR references (magenta).
func (s *Style) Merged(t string) string {
	if !s.enabled {
		return t
	}
	return magenta(t)
}

// Mergedf styles formatted merged PR references (magenta).
func (s *Style) Mergedf(format string, args ...any) string {
	return s.Merged(fmt.Sprintf(format, args...))
}

// Muted styles secondary/hint text (gray).
func (s *Style) Muted(t string) string {
	if !s.enabled {
		return t
	}
	return gray(t)
}

// Mutedf styles formatted secondary/hint text (gray).
func (s *Style) Mutedf(format string, args ...any) string {
	return s.Muted(fmt.Sprintf(format, args...))
}

// Bold styles text with bold weight.
func (s *Style) Bold(t string) string {
	if !s.enabled {
		return t
	}
	return bold(t)
}

// Boldf styles formatted text with bold weight.
func (s *Style) Boldf(format string, args ...any) string {
	return s.Bold(fmt.Sprintf(format, args...))
}

// SuccessIcon returns the success icon (✓) in green.
func (s *Style) SuccessIcon() string {
	return s.Success("✓")
}

// WarningIcon returns the warning icon (!) in yellow.
func (s *Style) WarningIcon() string {
	return s.Warning("!")
}

// FailureIcon returns the failure icon (✗) in red.
func (s *Style) FailureIcon() string {
	return s.Error("✗")
}

// SuccessMessage formats a complete success message with icon.
// Example: "✓ Operation complete"
func (s *Style) SuccessMessage(msg string) string {
	return fmt.Sprintf("%s %s", s.SuccessIcon(), s.Success(msg))
}

// WarningMessage formats a complete warning message with icon.
// Example: "! Something might be wrong"
func (s *Style) WarningMessage(msg string) string {
	return fmt.Sprintf("%s %s", s.WarningIcon(), s.Warning(msg))
}

// FailureMessage formats a complete failure message with icon.
// Example: "✗ Operation failed"
func (s *Style) FailureMessage(msg string) string {
	return fmt.Sprintf("%s %s", s.FailureIcon(), s.Error(msg))
}
