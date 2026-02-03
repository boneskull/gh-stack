// internal/prompt/prompt.go
package prompt

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/cli/safeexec"
	"github.com/mattn/go-isatty"
)

// IsInteractive returns true if stdin is connected to a terminal.
func IsInteractive() bool {
	return isatty.IsTerminal(os.Stdin.Fd()) || isatty.IsCygwinTerminal(os.Stdin.Fd())
}

// Input prompts the user for a single line of input with a default value.
// If the user enters nothing (just presses Enter), the default is returned.
// If stdin is not a TTY, the default is returned without prompting.
func Input(prompt, defaultValue string) (string, error) {
	if !IsInteractive() {
		return defaultValue, nil
	}

	if defaultValue != "" {
		fmt.Printf("%s [%s]: ", prompt, defaultValue)
	} else {
		fmt.Printf("%s: ", prompt)
	}

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read input: %w", err)
	}

	input = strings.TrimSpace(input)
	if input == "" {
		return defaultValue, nil
	}
	return input, nil
}

// Confirm prompts the user for a yes/no confirmation.
// Returns the defaultValue if stdin is not a TTY.
func Confirm(prompt string, defaultValue bool) (bool, error) {
	if !IsInteractive() {
		return defaultValue, nil
	}

	defaultStr := "y/N"
	if defaultValue {
		defaultStr = "Y/n"
	}

	fmt.Printf("%s [%s]: ", prompt, defaultStr)

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return defaultValue, fmt.Errorf("failed to read input: %w", err)
	}

	input = strings.TrimSpace(strings.ToLower(input))
	if input == "" {
		return defaultValue, nil
	}

	switch input {
	case "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	default:
		return defaultValue, nil
	}
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

// Select prompts the user to choose from a list of options.
// Returns the index of the selected option (0-based).
// If stdin is not a TTY, returns the defaultIndex.
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

	fmt.Println(prompt)
	for i, opt := range options {
		marker := "  "
		if i == defaultIndex {
			marker = "> "
		}
		fmt.Printf("%s%d. %s\n", marker, i+1, opt)
	}

	fmt.Printf("Choice [%d]: ", defaultIndex+1)

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return defaultIndex, fmt.Errorf("failed to read input: %w", err)
	}

	input = strings.TrimSpace(input)
	if input == "" {
		return defaultIndex, nil
	}

	var choice int
	if _, err := fmt.Sscanf(input, "%d", &choice); err != nil {
		return defaultIndex, nil
	}

	// Convert to 0-based index
	choice--
	if choice < 0 || choice >= len(options) {
		return defaultIndex, nil
	}

	return choice, nil
}
