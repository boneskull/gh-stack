// cmd/shortcuts_test.go
package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// testCommands returns the real set of subcommands registered on rootCmd.
// All init() functions in the cmd package run before tests, so this always
// reflects the actual command set.
func testCommands() []*cobra.Command {
	return rootCmd.Commands()
}

func TestResolvePrefix_ExactMatch(t *testing.T) {
	cmds := testCommands()

	for _, cmd := range cmds {
		if cmd.Hidden {
			continue
		}
		name := cmd.Name()
		resolved, ambiguous := resolvePrefix(name, cmds)
		if resolved != name {
			t.Errorf("exact match %q: got resolved=%q, want %q", name, resolved, name)
		}
		if ambiguous != nil {
			t.Errorf("exact match %q: got ambiguous=%v, want nil", name, ambiguous)
		}
	}
}

func TestResolvePrefix_ExactMatchAlias(t *testing.T) {
	cmds := testCommands()

	for _, cmd := range cmds {
		if cmd.Hidden {
			continue
		}
		for _, alias := range cmd.Aliases {
			resolved, ambiguous := resolvePrefix(alias, cmds)
			if resolved != alias {
				t.Errorf("exact alias match %q: got resolved=%q, want %q", alias, resolved, alias)
			}
			if ambiguous != nil {
				t.Errorf("exact alias match %q: got ambiguous=%v, want nil", alias, ambiguous)
			}
		}
	}
}

func TestResolvePrefix_Unambiguous(t *testing.T) {
	cmds := testCommands()

	tests := []struct {
		prefix string
		want   string
	}{
		{"i", "init"},
		{"in", "init"},
		{"ini", "init"},
		{"o", "orphan"},
		{"or", "orphan"},
		{"r", "restack"},
		{"re", "restack"},
		{"su", "submit"},
		{"sy", "sync"},
		{"ab", "abort"},
		{"ad", "adopt"},
		{"cr", "create"},
		{"li", "link"},
		{"lo", "log"},
		{"cas", "cascade"},
		{"und", "undo"},
		{"unl", "unlink"},
		{"con", "continue"},
	}

	for _, tc := range tests {
		resolved, ambiguous := resolvePrefix(tc.prefix, cmds)
		if resolved != tc.want {
			t.Errorf("prefix %q: got resolved=%q, want %q", tc.prefix, resolved, tc.want)
		}
		if ambiguous != nil {
			t.Errorf("prefix %q: got ambiguous=%v, want nil", tc.prefix, ambiguous)
		}
	}
}

func TestResolvePrefix_Ambiguous(t *testing.T) {
	cmds := testCommands()

	tests := []struct {
		prefix    string
		wantNames []string
	}{
		{"a", []string{"abort", "adopt"}},
		{"s", []string{"submit", "sync"}},
		{"l", []string{"link", "log"}},
		{"u", []string{"undo", "unlink"}},
		{"c", []string{"cascade", "continue", "create"}},
		{"un", []string{"undo", "unlink"}},
	}

	for _, tc := range tests {
		resolved, ambiguous := resolvePrefix(tc.prefix, cmds)
		if resolved != "" {
			t.Errorf("prefix %q: got resolved=%q, want empty", tc.prefix, resolved)
		}
		if len(ambiguous) != len(tc.wantNames) {
			t.Errorf("prefix %q: got %d ambiguous matches %v, want %d %v",
				tc.prefix, len(ambiguous), ambiguous, len(tc.wantNames), tc.wantNames)
			continue
		}
		for i, name := range ambiguous {
			if name != tc.wantNames[i] {
				t.Errorf("prefix %q: ambiguous[%d]=%q, want %q", tc.prefix, i, name, tc.wantNames[i])
			}
		}
	}
}

func TestResolvePrefix_NoMatch(t *testing.T) {
	cmds := testCommands()

	for _, prefix := range []string{"x", "zz", "foobar", "initx"} {
		resolved, ambiguous := resolvePrefix(prefix, cmds)
		if resolved != "" {
			t.Errorf("prefix %q: got resolved=%q, want empty", prefix, resolved)
		}
		if ambiguous != nil {
			t.Errorf("prefix %q: got ambiguous=%v, want nil", prefix, ambiguous)
		}
	}
}

func TestResolvePrefix_EmptyString(t *testing.T) {
	cmds := testCommands()

	resolved, ambiguous := resolvePrefix("", cmds)
	if resolved != "" {
		t.Errorf("empty string: got resolved=%q, want empty", resolved)
	}
	if ambiguous != nil {
		t.Errorf("empty string: got ambiguous=%v, want nil", ambiguous)
	}
}

func TestResolvePrefix_HiddenCommandsExcluded(t *testing.T) {
	cmds := []*cobra.Command{
		{Use: "continue"},
		{Use: "completion", Hidden: true},
	}

	resolved, ambiguous := resolvePrefix("co", cmds)
	if resolved != "continue" {
		t.Errorf("prefix %q: got resolved=%q, want %q", "co", resolved, "continue")
	}
	if ambiguous != nil {
		t.Errorf("prefix %q: got ambiguous=%v, want nil", "co", ambiguous)
	}
}

func TestResolvePrefix_ExactMatchTakesPrecedenceOverPrefix(t *testing.T) {
	// If there's a command named "in" and another named "init",
	// typing "in" should exact-match "in" rather than being ambiguous.
	cmds := []*cobra.Command{
		{Use: "in"},
		{Use: "init"},
	}

	resolved, ambiguous := resolvePrefix("in", cmds)
	if resolved != "in" {
		t.Errorf("exact takes precedence: got resolved=%q, want %q", resolved, "in")
	}
	if ambiguous != nil {
		t.Errorf("exact takes precedence: got ambiguous=%v, want nil", ambiguous)
	}
}

// withArgs temporarily replaces os.Args for the duration of fn, then restores
// the original value.
func withArgs(args []string, fn func()) {
	orig := os.Args
	os.Args = args
	defer func() { os.Args = orig }()
	fn()
}

func TestExpandCommandShortcut_RewritesUnambiguousPrefix(t *testing.T) {
	withArgs([]string{"gh-stack", "re"}, func() {
		if err := expandCommandShortcut(rootCmd); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if os.Args[1] != "restack" {
			t.Errorf("os.Args[1] = %q, want %q", os.Args[1], "restack")
		}
	})
}

func TestExpandCommandShortcut_ExactMatchUnchanged(t *testing.T) {
	withArgs([]string{"gh-stack", "init"}, func() {
		if err := expandCommandShortcut(rootCmd); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if os.Args[1] != "init" {
			t.Errorf("os.Args[1] = %q, want %q", os.Args[1], "init")
		}
	})
}

func TestExpandCommandShortcut_FlagPassthrough(t *testing.T) {
	withArgs([]string{"gh-stack", "-h"}, func() {
		if err := expandCommandShortcut(rootCmd); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if os.Args[1] != "-h" {
			t.Errorf("os.Args[1] = %q, want %q", os.Args[1], "-h")
		}
	})
}

func TestExpandCommandShortcut_EmptyArgsNoop(t *testing.T) {
	withArgs([]string{"gh-stack"}, func() {
		if err := expandCommandShortcut(rootCmd); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestExpandCommandShortcut_AmbiguousPrefixReturnsError(t *testing.T) {
	withArgs([]string{"gh-stack", "a"}, func() {
		err := expandCommandShortcut(rootCmd)
		if err == nil {
			t.Fatal("expected error for ambiguous prefix, got nil")
		}
		msg := err.Error()
		if !strings.Contains(msg, "ambiguous") {
			t.Errorf("error %q should contain %q", msg, "ambiguous")
		}
		for _, want := range []string{"abort", "adopt"} {
			if !strings.Contains(msg, want) {
				t.Errorf("error %q should list candidate %q", msg, want)
			}
		}
	})
}
