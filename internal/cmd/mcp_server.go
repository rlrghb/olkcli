package cmd

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// curatedTool maps an MCP tool name to the olk leaf command it runs. The set is
// hand-picked and read-first (gog's model) rather than reflecting the whole CLI,
// so a new command is never auto-exposed to an agent. write tools are eligible
// only when the server is started with --allow-write.
type curatedTool struct {
	name  string
	path  []string
	write bool
}

// curatedTools is the entire agent-facing surface. Destructive and send
// commands are deliberately absent — they have no MCP exposure path at all.
var curatedTools = []curatedTool{
	// Mail (read)
	{"mail_list", []string{"mail", "list"}, false},
	{"mail_get", []string{"mail", "get"}, false},
	{"mail_search", []string{"mail", "search"}, false},
	{"mail_folders_list", []string{"mail", "folders", "list"}, false},
	{"mail_attachments", []string{"mail", "attachments"}, false},
	{"mail_categories_list", []string{"mail", "categories", "list"}, false},
	{"mail_rules_list", []string{"mail", "rules", "list"}, false},
	{"mail_ooo_get", []string{"mail", "ooo", "get"}, false},
	// Calendar (read)
	{"calendar_events", []string{"calendar", "events"}, false},
	{"calendar_view", []string{"calendar", "view"}, false},
	{"calendar_get", []string{"calendar", "get"}, false},
	{"calendar_calendars", []string{"calendar", "calendars"}, false},
	{"calendar_availability", []string{"calendar", "availability"}, false},
	{"calendar_find_times", []string{"calendar", "find-times"}, false},
	// Contacts (read)
	{"contacts_list", []string{"contacts", "list"}, false},
	{"contacts_get", []string{"contacts", "get"}, false},
	{"contacts_search", []string{"contacts", "search"}, false},
	// Drive (read)
	{"drive_ls", []string{"drive", "ls"}, false},
	{"drive_get", []string{"drive", "get"}, false},
	{"drive_info", []string{"drive", "info"}, false},
	{"drive_search", []string{"drive", "search"}, false},
	{"drive_recent", []string{"drive", "recent"}, false},
	{"drive_shared", []string{"drive", "shared"}, false},
	{"drive_versions", []string{"drive", "versions"}, false},
	// To Do (read)
	{"todo_lists_list", []string{"todo", "lists", "list"}, false},
	{"todo_list", []string{"todo", "list"}, false},
	{"todo_get", []string{"todo", "get"}, false},
	{"todo_checklist_list", []string{"todo", "checklist", "list"}, false},
	{"todo_links_list", []string{"todo", "links", "list"}, false},
	// Directory + meta (read)
	{"people_search", []string{"people", "search"}, false},
	{"whoami", []string{"whoami"}, false},
	{"version", []string{"version"}, false},
	// Safe writes (opt-in: --allow-write, non-send and non-destructive)
	{"mail_drafts_create", []string{"mail", "drafts", "create"}, true},
	{"todo_create", []string{"todo", "create"}, true},
}

// mcpConfig controls which curated tools a server exposes.
type mcpConfig struct {
	allowWrite     map[string]bool          // set of curated write tool names to expose (nil/empty = none)
	allowed        func(path []string) bool // nil = allow all; else command allow/deny lists
	maxOutputBytes int                      // cap on a single tool call's output text (<=0 = defaultMaxOutputBytes)
}

// defaultMaxOutputBytes bounds a single tool call's returned text so a runaway
// list can't flood the agent's context or the stdio transport (gog's
// --max-output-bytes default).
const defaultMaxOutputBytes = 100_000

// helpFlagName is kong's auto-injected --help flag, skipped when projecting a
// command's flags into an MCP schema or argv.
const helpFlagName = "help"

// writeToolNames returns the set of curated write tool names (the only tools
// eligible to be exposed via --allow-write).
func writeToolNames() map[string]bool {
	names := map[string]bool{}
	for _, ct := range curatedTools {
		if ct.write {
			names[ct.name] = true
		}
	}
	return names
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// toolBinding ties a generated MCP tool name back to the kong command it runs.
type toolBinding struct {
	name           string
	path           []string
	node           *kong.Node
	readOnly       bool
	maxOutputBytes int
}

// newKongParser builds a kong parser over the full CLI grammar, mirroring the
// options used by Execute so the introspected model matches the real CLI. The
// no-op Exit prevents kong from calling os.Exit on the in-process reparse path.
func newKongParser(cli *CLI) (*kong.Kong, error) {
	return kong.New(cli,
		kong.Name("olk"),
		kong.Description("Microsoft Outlook CLI - Access email, calendar, and contacts from the command line"),
		kong.ConfigureHelp(kong.HelpOptions{Compact: true}),
		kong.Vars{"version": Version},
		kong.Exit(func(int) {}),
	)
}

// buildMCPServer constructs an MCP server exposing the curated tools the config
// permits. It returns the bindings so callers (and tests) can inspect the set.
func buildMCPServer(cfg mcpConfig) (*mcp.Server, []*toolBinding, error) {
	k, err := newKongParser(&CLI{})
	if err != nil {
		return nil, nil, err
	}
	leaves := indexLeaves(k.Model.Node)

	srv := mcp.NewServer(&mcp.Implementation{Name: "olk", Version: Version}, nil)

	maxOut := cfg.maxOutputBytes
	if maxOut <= 0 {
		maxOut = defaultMaxOutputBytes
	}

	bindings := make([]*toolBinding, 0, len(curatedTools))
	for _, ct := range curatedTools {
		if ct.write && !cfg.allowWrite[ct.name] {
			continue
		}
		if cfg.allowed != nil && !cfg.allowed(ct.path) {
			continue
		}
		node, ok := leaves[strings.Join(ct.path, ".")]
		if !ok {
			return nil, nil, fmt.Errorf("curated MCP tool %q maps to unknown command %q", ct.name, strings.Join(ct.path, " "))
		}
		b := &toolBinding{name: ct.name, path: ct.path, node: node, readOnly: !ct.write, maxOutputBytes: maxOut}
		registerTool(srv, b)
		bindings = append(bindings, b)
	}
	return srv, bindings, nil
}

// indexLeaves walks the kong model and returns every visible leaf command keyed
// by its dot-joined path (e.g. "mail.folders.list").
func indexLeaves(root *kong.Node) map[string]*kong.Node {
	leaves := map[string]*kong.Node{}
	var walk func(n *kong.Node, path []string)
	walk = func(n *kong.Node, path []string) {
		for _, child := range n.Children {
			if child.Type != kong.CommandNode || child.Hidden {
				continue
			}
			cp := append(append([]string{}, path...), child.Name)
			if child.Leaf() {
				leaves[strings.Join(cp, ".")] = child
			} else {
				walk(child, cp)
			}
		}
	}
	walk(root, nil)
	return leaves
}

func registerTool(srv *mcp.Server, b *toolBinding) {
	desc := b.node.Help
	if b.node.Detail != "" {
		desc = strings.TrimSpace(desc + "\n\n" + b.node.Detail)
	}
	tool := &mcp.Tool{
		Name:        b.name,
		Description: desc,
		InputSchema: flagSchema(b.node),
		Annotations: annotationsFor(b.readOnly),
	}
	srv.AddTool(tool, makeHandler(b))
}

func annotationsFor(readOnly bool) *mcp.ToolAnnotations {
	if readOnly {
		return &mcp.ToolAnnotations{ReadOnlyHint: true}
	}
	f := false
	return &mcp.ToolAnnotations{DestructiveHint: &f}
}

// flagSchema builds the JSON Schema for a leaf command's flags and positionals.
func flagSchema(node *kong.Node) *jsonschema.Schema {
	s := &jsonschema.Schema{
		Type:       "object",
		Properties: map[string]*jsonschema.Schema{},
	}
	var required []string

	for _, f := range node.Flags {
		if f.Hidden || f.Name == helpFlagName {
			continue
		}
		s.Properties[f.Name] = valueSchema(f.Value)
		if f.Required {
			required = append(required, f.Name)
		}
	}
	for _, p := range node.Positional {
		s.Properties[p.Name] = valueSchema(p)
		if p.Required {
			required = append(required, p.Name)
		}
	}
	s.Required = required
	return s
}

func valueSchema(v *kong.Value) *jsonschema.Schema {
	sch := &jsonschema.Schema{Description: v.Help}
	switch {
	case v.IsBool():
		sch.Type = "boolean"
	case v.IsSlice():
		sch.Type = "array"
		sch.Items = &jsonschema.Schema{Type: "string"}
	default:
		switch v.Target.Kind() { //nolint:exhaustive // numeric kinds map to integer/number; default is string
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			sch.Type = "integer"
		case reflect.Float32, reflect.Float64:
			sch.Type = "number"
		default:
			sch.Type = "string"
		}
	}

	if v.Enum != "" {
		for _, e := range v.EnumSlice() {
			if e == "" {
				continue
			}
			sch.Enum = append(sch.Enum, e)
		}
	}
	return sch
}
