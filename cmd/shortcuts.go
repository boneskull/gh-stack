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
//   - (name, nil) for an unambiguous match (name may be a primary name or alias)
//   - ("", matches) when the prefix is ambiguous (len(matches) > 1)
//   - ("", nil) when nothing matches
//
// Exact matches short-circuit: if the prefix equals a command name or alias
// exactly, that command wins regardless of other prefix overlaps.
// Hidden commands are excluded.
func resolvePrefix(prefix string, commands []*cobra.Command) (resolved string, ambiguous []string) {
	if prefix == "" {
		return "", nil
	}

	var matches []string

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
			matches = append(matches, name)
		}

		for _, alias := range cmd.Aliases {
			// Exact match on alias — done.
			if alias == prefix {
				return alias, nil
			}
			if strings.HasPrefix(alias, prefix) {
				matches = append(matches, alias)
			}
		}
	}

	switch len(matches) {
	case 0:
		return "", nil
	case 1:
		return matches[0], nil
	default:
		sort.Strings(matches)
		return "", matches
	}
}

// expandCommandShortcut rewrites os.Args[1] when it is an unambiguous prefix
// of a registered subcommand.  For ambiguous prefixes it returns an error.
// Exact matches, flags, and empty arg lists pass through untouched.
func expandCommandShortcut(root *cobra.Command) error {
	if len(os.Args) < 2 {
		return nil
	}

	input := os.Args[1]
	if strings.HasPrefix(input, "-") {
		return nil
	}

	resolved, ambiguous := resolvePrefix(input, root.Commands())

	if len(ambiguous) > 0 {
		return fmt.Errorf("%q is ambiguous: %s", input, strings.Join(ambiguous, ", "))
	}

	if resolved != "" && resolved != input {
		os.Args[1] = resolved
	}

	return nil
}
