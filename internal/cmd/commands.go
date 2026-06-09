package cmd

import (
	"strings"

	"github.com/alecthomas/kong"
)

// selectedCommandPath returns the resolved command path (e.g. []string{"mail",
// "get"}) for a parsed kong context. kong's Command() renders positional
// arguments as "<name>" tokens, which we drop so only command names remain.
func selectedCommandPath(kctx *kong.Context) []string {
	fields := strings.Fields(kctx.Command())
	path := make([]string, 0, len(fields))
	for _, tok := range fields {
		if strings.HasPrefix(tok, "<") || strings.HasPrefix(tok, "[") {
			continue
		}
		path = append(path, tok)
	}
	return path
}

// commandAllowed reports whether a command path may run under the active
// allow/deny lists. Semantics mirror gog:
//   - --disable-commands blocks a path or any descendant (overrides allows).
//   - --enable-commands allows a prefix and its descendants.
//   - --enable-commands-exact allows only that exact path (no descendants).
//   - if any allow list is set and nothing matches, the command is denied;
//     if no allow list is set, everything not explicitly disabled is allowed.
func commandAllowed(flags *RootFlags, path []string) bool {
	if len(path) == 0 {
		return true
	}
	dotted := strings.Join(path, ".")

	for _, d := range splitCSV(flags.DisableCommands) {
		if dotted == d || strings.HasPrefix(dotted, d+".") {
			return false
		}
	}

	exact := splitCSV(flags.EnableCommandsExact)
	prefixes := splitCSV(flags.EnableCommands)
	if len(exact) == 0 && len(prefixes) == 0 {
		return true // no allow list configured → default-allow
	}

	for _, e := range exact {
		if dotted == e {
			return true
		}
	}
	for _, p := range prefixes {
		if dotted == p || strings.HasPrefix(dotted, p+".") {
			return true
		}
	}
	return false
}

// splitCSV splits a comma-separated list, trimming whitespace and dropping empties.
func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
