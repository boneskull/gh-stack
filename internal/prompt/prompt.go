// internal/prompt/prompt.go
package prompt

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/cli/go-gh/v2/pkg/prompter"
	"github.com/cli/go-gh/v2/pkg/term"
	"github.com/cli/safeexec"
)

// termState caches terminal state for the session.
var termState term.Term

func init() {
	termState = term.FromEnv()
}

// IsInteractive returns true if stdout is connected to a terminal.
// Respects GH_FORCE_TTY, NO_COLOR, CLICOLOR, and CLICOLOR_FORCE environment variables
// for consistency with the gh CLI.
func IsInteractive() bool {
	return termState.IsTerminalOutput()
}

// newPrompter creates a prompter instance for interactive input.
func newPrompter() *prompter.Prompter {
	return prompter.New(os.Stdin, os.Stdout, os.Stderr)
}

// Input prompts the user for a single line of input with a default value.
// If the user enters nothing (just presses Enter), the default is returned.
// If not in an interactive terminal, the default is returned without prompting.
func Input(prompt, defaultValue string) (string, error) {
	if !IsInteractive() {
		return defaultValue, nil
	}

	p := newPrompter()
	result, err := p.Input(prompt, defaultValue)
	if err != nil {
		return defaultValue, fmt.Errorf("failed to read input: %w", err)
	}
	return result, nil
}

// Confirm prompts the user for a yes/no confirmation.
// Returns the defaultValue if not in an interactive terminal.
func Confirm(prompt string, defaultValue bool) (bool, error) {
	if !IsInteractive() {
		return defaultValue, nil
	}

	p := newPrompter()
	result, err := p.Confirm(prompt, defaultValue)
	if err != nil {
		return defaultValue, fmt.Errorf("failed to read input: %w", err)
	}
	return result, nil
}

// EditInEditor opens the given text in the user's preferred editor and returns
// the edited content. Uses $VISUAL, then $EDITOR, then falls back to "vi".
// If stdin is not a TTY, returns the original text without editing.
func EditInEditor(text string) (string, error) {
	if !IsInteractive() {
		return text, nil
	}

	// Find editor
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}

	// Create temp file
	tmpFile, err := os.CreateTemp("", "gh-stack-*.md")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath) //nolint:errcheck // best-effort cleanup

	// Write initial content
	if _, writeErr := tmpFile.WriteString(text); writeErr != nil {
		tmpFile.Close() //nolint:errcheck // already returning error
		return "", fmt.Errorf("failed to write temp file: %w", writeErr)
	}
	tmpFile.Close() //nolint:errcheck // file written successfully, close errors are ignorable

	// Resolve editor path securely
	editorPath, lookupErr := safeexec.LookPath(editor)
	if lookupErr != nil {
		// Try common editors as fallback
		for _, fallback := range []string{"vim", "nano", "vi"} {
			if p, e := safeexec.LookPath(fallback); e == nil {
				editorPath = p
				break
			}
		}
		if editorPath == "" {
			return "", fmt.Errorf("no editor found (set $EDITOR): %w", lookupErr)
		}
	}

	// Run editor
	cmd := exec.Command(editorPath, tmpPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if runErr := cmd.Run(); runErr != nil {
		return "", fmt.Errorf("editor exited with error: %w", runErr)
	}

	// Read edited content
	edited, err := os.ReadFile(tmpPath)
	if err != nil {
		return "", fmt.Errorf("failed to read edited file: %w", err)
	}

	// Normalize line endings and trim trailing whitespace
	result := strings.ReplaceAll(string(edited), "\r\n", "\n")
	result = strings.TrimRight(result, "\n\t ")

	return result, nil
}

// Select prompts the user to choose from a list of options using arrow keys.
// Returns the index of the selected option (0-based).
// If not in an interactive terminal, returns the defaultIndex.
func Select(prompt string, options []string, defaultIndex int) (int, error) {
	if len(options) == 0 {
		return 0, fmt.Errorf("no options provided")
	}

	// Clamp defaultIndex to valid range
	if defaultIndex < 0 || defaultIndex >= len(options) {
		defaultIndex = 0
	}

	if !IsInteractive() {
		return defaultIndex, nil
	}

	// Convert defaultIndex to default value string for prompter
	defaultValue := options[defaultIndex]

	p := newPrompter()
	result, err := p.Select(prompt, defaultValue, options)
	if err != nil {
		return defaultIndex, fmt.Errorf("failed to read selection: %w", err)
	}
	return result, nil
}
