// cmd/shortcuts.go
package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// resolvePrefix finds the subcommand whose name (or alias) uniquely matches
// the given prefix.  It returns:
//   - (fullName, nil, nil) for an unambiguous match
//   - ("", matches, nil) when the prefix is ambiguous (len(matches) > 1)
//   - ("", nil, nil) when nothing matches
//
// Exact matches short-circuit: if the prefix equals a command name or alias
// exactly, that command wins regardless of other prefix overlaps.
// Hidden commands are excluded.
func resolvePrefix(prefix string, commands []*cobra.Command) (resolved string, ambiguous []string) {
	if prefix == "" {
		return "", nil
	}

	type candidate struct {
		name    string // the name or alias that matched
		cmdName string // the command's primary Use name
	}

	var matches []candidate

	for _, cmd := range commands {
		if cmd.Hidden {
			continue
		}

		name := cmd.Name()

		// Exact match on primary name — done.
		if name == prefix {
			return name, nil
		}
		if strings.HasPrefix(name, prefix) {
			matches = append(matches, candidate{name: name, cmdName: name})
		}

		for _, alias := range cmd.Aliases {
			// Exact match on alias — done.
			if alias == prefix {
				return alias, nil
			}
			if strings.HasPrefix(alias, prefix) {
				matches = append(matches, candidate{name: alias, cmdName: name})
			}
		}
	}

	switch len(matches) {
	case 0:
		return "", nil
	case 1:
		return matches[0].name, nil
	default:
		names := make([]string, len(matches))
		for i, m := range matches {
			names[i] = m.name
		}
		sort.Strings(names)
		return "", names
	}
}

// expandCommandShortcut rewrites os.Args[1] when it is an unambiguous prefix
// of a registered subcommand.  For ambiguous prefixes it prints an error and
// exits.  Exact matches, flags, and empty arg lists pass through untouched.
func expandCommandShortcut(root *cobra.Command) {
	if len(os.Args) < 2 {
		return
	}

	input := os.Args[1]
	if strings.HasPrefix(input, "-") {
		return
	}

	resolved, ambiguous := resolvePrefix(input, root.Commands())

	if len(ambiguous) > 0 {
		fmt.Fprintf(os.Stderr, "Error: %q is ambiguous: %s\n", input, strings.Join(ambiguous, ", "))
		os.Exit(1)
	}

	if resolved != "" && resolved != input {
		os.Args[1] = resolved
	}
}
