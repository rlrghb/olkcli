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

// toolTier classifies a curated tool by how dangerous it is, which determines
// what opt-in is required to expose it. Each higher tier needs its own explicit,
// separate flag — defense in depth, so granting reads/safe-writes never grants
// sending or deletion.
type toolTier int

const (
	tierRead        toolTier = iota // always exposed
	tierSafeWrite                   // --allow-write: reversible / non-send mutation
	tierSend                        // --allow-send: sends a message or meeting invite
	tierDestructive                 // --allow-destructive: hard-deletes server data
)

// curatedTool maps an MCP tool name to the olk leaf command it runs. The set is
// hand-picked and read-first rather than reflecting the whole CLI, so a new
// command is never auto-exposed to an agent.
type curatedTool struct {
	name string
	path []string
	tier toolTier
}

// curatedTools is the entire agent-facing surface. Reads are always exposed;
// every mutation requires the matching opt-in flag for its tier, and send /
// destructive tools are additionally vetoed by --no-send / --no-write.
var curatedTools = []curatedTool{
	// Mail (read)
	{"mail_list", []string{"mail", "list"}, tierRead},
	{"mail_get", []string{"mail", "get"}, tierRead},
	{"mail_batch", []string{"mail", "batch"}, tierRead},
	{"mail_thread", []string{"mail", "thread"}, tierRead},
	{"mail_search", []string{"mail", "search"}, tierRead},
	{"mail_folders_list", []string{"mail", "folders", "list"}, tierRead},
	{"mail_attachments", []string{"mail", "attachments"}, tierRead},
	{"mail_categories_list", []string{"mail", "categories", "list"}, tierRead},
	{"mail_rules_list", []string{"mail", "rules", "list"}, tierRead},
	{"mail_ooo_get", []string{"mail", "ooo", "get"}, tierRead},
	{"mail_delta", []string{"mail", "delta"}, tierRead},
	// Calendar (read)
	{"calendar_events", []string{"calendar", "events"}, tierRead},
	{"calendar_view", []string{"calendar", "view"}, tierRead},
	{"calendar_get", []string{"calendar", "get"}, tierRead},
	{"calendar_calendars", []string{"calendar", "calendars"}, tierRead},
	{"calendar_availability", []string{"calendar", "availability"}, tierRead},
	{"calendar_find_times", []string{"calendar", "find-times"}, tierRead},
	{"calendar_delta", []string{"calendar", "delta"}, tierRead},
	// Contacts (read)
	{"contacts_list", []string{"contacts", "list"}, tierRead},
	{"contacts_get", []string{"contacts", "get"}, tierRead},
	{"contacts_search", []string{"contacts", "search"}, tierRead},
	{"contacts_delta", []string{"contacts", "delta"}, tierRead},
	// Drive (read)
	{"drive_ls", []string{"drive", "ls"}, tierRead},
	{"drive_get", []string{"drive", "get"}, tierRead},
	{"drive_info", []string{"drive", "info"}, tierRead},
	{"drive_search", []string{"drive", "search"}, tierRead},
	{"drive_recent", []string{"drive", "recent"}, tierRead},
	{"drive_shared", []string{"drive", "shared"}, tierRead},
	{"drive_versions", []string{"drive", "versions"}, tierRead},
	// To Do (read)
	{"todo_lists_list", []string{"todo", "lists", "list"}, tierRead},
	{"todo_list", []string{"todo", "list"}, tierRead},
	{"todo_get", []string{"todo", "get"}, tierRead},
	{"todo_checklist_list", []string{"todo", "checklist", "list"}, tierRead},
	{"todo_links_list", []string{"todo", "links", "list"}, tierRead},
	// Directory + meta (read)
	{"people_search", []string{"people", "search"}, tierRead},
	{"changes", []string{"changes"}, tierRead},
	{"whoami", []string{"whoami"}, tierRead},
	{"version", []string{"version"}, tierRead},
	// Safe writes (--allow-write): non-send, non-destructive or reversible.
	{"mail_drafts_create", []string{"mail", "drafts", "create"}, tierSafeWrite},
	{"mail_flag", []string{"mail", "flag"}, tierSafeWrite},
	{"mail_categorize", []string{"mail", "categorize"}, tierSafeWrite},
	{"mail_mark", []string{"mail", "mark"}, tierSafeWrite},
	{"mail_move", []string{"mail", "move"}, tierSafeWrite}, // reversible: a move can be moved back
	{"mail_folders_create", []string{"mail", "folders", "create"}, tierSafeWrite},
	{"mail_folders_rename", []string{"mail", "folders", "rename"}, tierSafeWrite},
	{"contacts_create", []string{"contacts", "create"}, tierSafeWrite},
	{"contacts_update", []string{"contacts", "update"}, tierSafeWrite},
	{"todo_create", []string{"todo", "create"}, tierSafeWrite},
	{"todo_update", []string{"todo", "update"}, tierSafeWrite},
	{"todo_complete", []string{"todo", "complete"}, tierSafeWrite}, // reversible: status can be set back
	// Send (--allow-send, off by default, also vetoed by --no-send): transmits
	// to other people. calendar create/update are here because attendee'd events
	// send invitations.
	{"mail_send", []string{"mail", "send"}, tierSend},
	{"mail_reply", []string{"mail", "reply"}, tierSend},
	{"mail_forward", []string{"mail", "forward"}, tierSend},
	{"mail_drafts_send", []string{"mail", "drafts", "send"}, tierSend},
	{"calendar_respond", []string{"calendar", "respond"}, tierSend},
	{"calendar_create", []string{"calendar", "create"}, tierSend},
	{"calendar_update", []string{"calendar", "update"}, tierSend},
	// Destructive (--allow-destructive, off by default, vetoed by --no-write):
	// hard-deletes. The MCP layer supplies --force, since naming the tool is the
	// deliberate confirmation.
	{"mail_delete", []string{"mail", "delete"}, tierDestructive},
	{"calendar_delete", []string{"calendar", "delete"}, tierDestructive},
	{"contacts_delete", []string{"contacts", "delete"}, tierDestructive},
	{"todo_delete", []string{"todo", "delete"}, tierDestructive},
}

// mcpConfig controls which curated tools a server exposes.
type mcpConfig struct {
	allowWrite       map[string]bool                       // tierSafeWrite tools to expose
	allowSend        map[string]bool                       // tierSend tools to expose
	allowDestructive map[string]bool                       // tierDestructive tools to expose
	noWrite          bool                                  // --no-write: hide every mutating tool
	noSend           bool                                  // --no-send: hide send tools
	allowed          func(path []string) bool              // nil = allow all; else command allow/deny lists
	allowTool        func(name string, readOnly bool) bool // nil = allow all; else --allow-tool selectors
	maxOutputBytes   int                                   // cap on a single tool call's output text (<=0 = defaultMaxOutputBytes)
	env              callEnv                               // launch-time context applied to every tool call
}

// callEnv carries the operator's launch-time context into each tool call.
//
// A call re-parses a fresh CLI from a rebuilt argv, so nothing set on the
// `olk mcp` command line survives on its own: only OLK_* environment variables
// do, because kong re-reads them on every parse. Left unpropagated, a server
// started with `--mailbox boss@example.com` silently served the operator's own
// mailbox instead.
//
// The capability guards are carried too. Registration already hides the tools
// they veto, so this is the second layer: should a tool ever be registered in
// error, the graphapi guard still refuses the call.
type callEnv struct {
	mailbox      string   // --mailbox: default target for mailbox-aware tools
	account      string   // --account: which signed-in identity to use
	timeout      int      // --timeout: per-call ceiling, in seconds
	noWrite      bool     // --no-write
	noSend       bool     // --no-send
	allowMailbox []string // --allow-mailbox: mailboxes an agent may name per call
}

// mailboxAllowed reports whether an agent may direct a call at target. An empty
// allowlist permits nothing, which is what keeps the argument absent from every
// schema until the operator opts in.
func (e callEnv) mailboxAllowed(target string) bool {
	for _, m := range e.allowMailbox {
		if strings.EqualFold(m, target) {
			return true
		}
	}
	return false
}

// offersMailboxArg reports whether a tool should advertise, and accept, a
// per-call mailbox: it must honour the flag and the operator must have named
// mailboxes to choose from.
func (e callEnv) offersMailboxArg(toolName string) bool {
	return len(e.allowMailbox) > 0 && mailboxAwareTools[toolName]
}

// exposes reports whether ct may be registered under cfg: reads always; each
// mutation tier needs its matching allow-map entry, and send/destructive are
// additionally gated by the --no-send / --no-write capability guards so a
// guard-disabled tool is never even advertised.
func (cfg *mcpConfig) exposes(ct curatedTool) bool {
	switch ct.tier {
	case tierRead:
		return true
	case tierSafeWrite:
		return !cfg.noWrite && cfg.allowWrite[ct.name]
	case tierSend:
		return !cfg.noWrite && !cfg.noSend && cfg.allowSend[ct.name]
	case tierDestructive:
		return !cfg.noWrite && cfg.allowDestructive[ct.name]
	default:
		return false
	}
}

// defaultMaxOutputBytes bounds a single tool call's returned text so a runaway
// list can't flood the agent's context or the stdio transport.
const defaultMaxOutputBytes = 100_000

// helpFlagName is kong's auto-injected --help flag, skipped when projecting a
// command's flags into an MCP schema or argv.
const helpFlagName = "help"

// conciseArg is a synthetic boolean injected into every read tool's schema. It
// maps to olk's global --concise flag (which isn't a per-command flag, so it
// wouldn't otherwise appear in the tool schema), letting an agent shrink a
// response per call without affecting tools where the body is the point.
const conciseArg = "concise"

// mailboxArg is a synthetic string injected into the schema of mailbox-aware
// tools, but only when the operator has named permitted mailboxes with
// --allow-mailbox. It maps to olk's global --mailbox flag. Without that flag the
// property is absent everywhere, so an agent has no way to choose a mailbox and
// the server behaves exactly as it did before.
const mailboxArg = "mailbox"

// mailboxAwareTools names the curated tools whose command honours --mailbox.
// This is deliberately a separate list from curatedTools: whether a command
// reads the flag is a property of its Run method rather than of its place in the
// registry, and TestMailboxAwareTools_MatchesSource parses the command sources to
// keep the two in step. Offering the argument on a tool that ignores it would
// promise scoping that never happens, which is the same silent mismatch the
// mailbox work exists to remove.
// Note what is absent: the calendar and contacts write commands, and the folder
// write commands, do not read --mailbox at all, so they always act on the
// signed-in user's own mailbox. That is a wider gap than this list, and it is not
// addressed here; the list only records the state of things rather than papering
// over it.
var mailboxAwareTools = map[string]bool{
	"mail_list": true, "mail_get": true, "mail_batch": true, "mail_thread": true,
	"mail_search": true, "mail_folders_list": true, "mail_delta": true,
	"mail_drafts_create": true, "mail_drafts_send": true,
	"mail_send": true, "mail_reply": true, "mail_forward": true,
	"calendar_events": true, "calendar_view": true, "calendar_get": true,
	"calendar_calendars": true, "calendar_delta": true,
	"contacts_list": true, "contacts_get": true, "contacts_search": true,
	"contacts_delta": true,
	"changes":        true,
}

// toolNamesForTier returns the set of curated tool names in a given tier — the
// only names eligible for that tier's allow flag.
func toolNamesForTier(tier toolTier) map[string]bool {
	names := map[string]bool{}
	for _, ct := range curatedTools {
		if ct.tier == tier {
			names[ct.name] = true
		}
	}
	return names
}

func writeToolNames() map[string]bool       { return toolNamesForTier(tierSafeWrite) }
func sendToolNames() map[string]bool        { return toolNamesForTier(tierSend) }
func destructiveToolNames() map[string]bool { return toolNamesForTier(tierDestructive) }

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
	tier           toolTier
	maxOutputBytes int
	env            callEnv
}

func (b *toolBinding) readOnly() bool { return b.tier == tierRead }

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
func buildMCPServer(cfg *mcpConfig) (*mcp.Server, []*toolBinding, error) {
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
		if !cfg.exposes(ct) {
			continue
		}
		if cfg.allowed != nil && !cfg.allowed(ct.path) {
			continue
		}
		if cfg.allowTool != nil && !cfg.allowTool(ct.name, ct.tier == tierRead) {
			continue
		}
		node, ok := leaves[strings.Join(ct.path, ".")]
		if !ok {
			return nil, nil, fmt.Errorf("curated MCP tool %q maps to unknown command %q", ct.name, strings.Join(ct.path, " "))
		}
		b := &toolBinding{name: ct.name, path: ct.path, node: node, tier: ct.tier, maxOutputBytes: maxOut, env: cfg.env}
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
		InputSchema: toolSchema(b),
		Annotations: annotationsFor(b.tier),
	}
	srv.AddTool(tool, makeHandler(b))
}

// toolSchema builds a binding's MCP input schema: the command's own flags and
// positionals, plus the synthetic `concise` boolean for read tools.
func toolSchema(b *toolBinding) *jsonschema.Schema {
	schema := flagSchema(b.node)
	if b.readOnly() {
		schema.Properties[conciseArg] = &jsonschema.Schema{
			Type:        "boolean",
			Description: "Drop large free-text fields (message/event/task bodies, previews, attendee lists) to reduce payload size.",
		}
	}
	if b.env.offersMailboxArg(b.name) {
		allowed := make([]any, 0, len(b.env.allowMailbox))
		for _, m := range b.env.allowMailbox {
			allowed = append(allowed, m)
		}
		schema.Properties[mailboxArg] = &jsonschema.Schema{
			Type: "string",
			Enum: allowed,
			Description: "Act on this delegated mailbox instead of the signed-in user's own. " +
				"Only the listed addresses are permitted; any other value is refused. " +
				"Omit to use the mailbox the server was started with.",
		}
	}
	return schema
}

func annotationsFor(tier toolTier) *mcp.ToolAnnotations {
	if tier == tierRead {
		return &mcp.ToolAnnotations{ReadOnlyHint: true}
	}
	// Send and destructive tools have irreversible external effects.
	destructive := tier == tierSend || tier == tierDestructive
	return &mcp.ToolAnnotations{DestructiveHint: &destructive}
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
