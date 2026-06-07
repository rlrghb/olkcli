package cmd

import "strings"

// cmdKind classifies a leaf command for MCP profile filtering and tool annotations.
type cmdKind int

const (
	kindRead cmdKind = iota
	kindWrite
	kindDestructive
	kindInteractive
)

// destructiveVerbs match on a command's final path token at any depth, so nested
// commands like "mail categories delete" and "todo lists delete" are caught.
var destructiveVerbs = map[string]bool{
	"delete": true,
	"rm":     true,
	"clean":  true,
	"logout": true,
}

// readVerbs match on a command's final path token. Anything that is neither a
// read verb nor destructive is treated as a write.
var readVerbs = map[string]bool{
	"list":         true,
	"get":          true,
	"search":       true,
	"view":         true,
	"events":       true,
	"ls":           true,
	"info":         true,
	"recent":       true,
	"shared":       true,
	"status":       true,
	"calendars":    true,
	"folders":      true,
	"categories":   true,
	"versions":     true,
	"find-times":   true,
	"availability": true,
	"lists":        true,
	"drafts":       true,
}

// pathOverrides classify by full command path where the verb heuristic is wrong
// or ambiguous. Keys are space-joined command paths.
var pathOverrides = map[string]cmdKind{
	"whoami":      kindRead,
	"version":     kindRead,
	"config get":  kindRead,
	"auth login":  kindInteractive,
	"auth list":   kindRead,
	"auth status": kindRead,
}

// classify returns the kind of a leaf command identified by its path
// (e.g. []string{"mail", "delete"}).
func classify(path []string) cmdKind {
	if len(path) == 0 {
		return kindWrite
	}
	if k, ok := pathOverrides[strings.Join(path, " ")]; ok {
		return k
	}
	leaf := path[len(path)-1]
	switch {
	case destructiveVerbs[leaf]:
		return kindDestructive
	case readVerbs[leaf]:
		return kindRead
	default:
		return kindWrite
	}
}

// includeInProfile reports whether a command of the given kind should be exposed
// as a tool under the named profile. Interactive commands (and the mcp command
// itself, filtered separately) are never exposed.
func includeInProfile(profile string, kind cmdKind) bool {
	switch kind {
	case kindInteractive:
		return false
	case kindDestructive:
		return profile == "full"
	case kindRead, kindWrite:
		return true
	default:
		return false
	}
}
