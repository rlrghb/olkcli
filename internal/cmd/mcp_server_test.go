package cmd

import (
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/google/jsonschema-go/jsonschema"
)

// leafByPath resolves a leaf command node by path from a fresh parser, so schema
// and argv tests can target any command regardless of whether it is curated.
func leafByPath(t *testing.T, path ...string) *kong.Node {
	t.Helper()
	k, err := newKongParser(&CLI{})
	if err != nil {
		t.Fatalf("newKongParser: %v", err)
	}
	n, ok := indexLeaves(k.Model.Node)[strings.Join(path, ".")]
	if !ok {
		t.Fatalf("no leaf command at %v", path)
	}
	return n
}

// testBinding builds a binding for any command path (used by argv tests).
func testBinding(t *testing.T, path ...string) *toolBinding {
	t.Helper()
	return &toolBinding{name: strings.Join(path, "_"), path: path, node: leafByPath(t, path...), tier: tierRead}
}

func bindingsMap(t *testing.T, cfg mcpConfig) map[string]*toolBinding {
	t.Helper()
	_, bindings, err := buildMCPServer(cfg)
	if err != nil {
		t.Fatalf("buildMCPServer(%+v): %v", cfg, err)
	}
	m := make(map[string]*toolBinding, len(bindings))
	for _, b := range bindings {
		if _, dup := m[b.name]; dup {
			t.Errorf("duplicate tool name %q", b.name)
		}
		m[b.name] = b
	}
	return m
}

func setOf(names ...string) map[string]bool {
	m := map[string]bool{}
	for _, n := range names {
		m[n] = true
	}
	return m
}

func buildBindings(t *testing.T, writes ...string) map[string]*toolBinding {
	t.Helper()
	return bindingsMap(t, mcpConfig{allowWrite: setOf(writes...)})
}

func TestCuratedRegistry_DefaultIsReadOnly(t *testing.T) {
	def := buildBindings(t) // no writes named

	for _, name := range []string{"mail_list", "mail_get", "mail_search", "calendar_events", "whoami", "version"} {
		if _, ok := def[name]; !ok {
			t.Errorf("expected read tool %q in default registry", name)
		}
	}
	// Write tools are absent unless named via --allow-write.
	for _, name := range []string{"mail_drafts_create", "todo_create"} {
		if _, ok := def[name]; ok {
			t.Errorf("write tool %q must not appear without --allow-write", name)
		}
	}
	for _, b := range def {
		if !b.readOnly() {
			t.Errorf("default registry exposed non-read tool %q", b.name)
		}
	}
}

func TestCuratedRegistry_AllowWriteIsPerTool(t *testing.T) {
	// Naming one write exposes only that one — not the other.
	one := buildBindings(t, "mail_drafts_create")
	if _, ok := one["mail_drafts_create"]; !ok {
		t.Error("expected mail_drafts_create when named")
	}
	if _, ok := one["todo_create"]; ok {
		t.Error("todo_create must NOT appear when only mail_drafts_create is named")
	}
	if len(one) <= len(buildBindings(t)) {
		t.Error("naming a write should expose strictly more tools")
	}

	// Naming both exposes both.
	both := buildBindings(t, "mail_drafts_create", "todo_create")
	for _, name := range []string{"mail_drafts_create", "todo_create"} {
		if _, ok := both[name]; !ok {
			t.Errorf("expected %q when named", name)
		}
	}
}

// TestCuratedRegistry_TierGating is the core guard: each tier is exposed only by
// its own opt-in flag, and lower-tier grants never leak higher-tier tools. In
// particular a send (mail_send) or destructive (mail_delete) tool never appears
// by default or under --allow-write.
func TestCuratedRegistry_TierGating(t *testing.T) {
	// Default: only reads; no mutation of any kind.
	def := buildBindings(t)
	for name, b := range def {
		if !b.readOnly() {
			t.Errorf("default registry exposed non-read tool %q", name)
		}
	}

	// --allow-write exposes safe writes but never send or destructive tools.
	w := bindingsMap(t, mcpConfig{allowWrite: writeToolNames()})
	if _, ok := w["mail_flag"]; !ok {
		t.Error("--allow-write should expose mail_flag")
	}
	for _, leaked := range []string{"mail_send", "mail_reply", "mail_delete", "calendar_delete"} {
		if _, ok := w[leaked]; ok {
			t.Errorf("--allow-write must not expose %q (wrong tier)", leaked)
		}
	}

	// --allow-send exposes only the named send tool; not safe writes/destructive.
	s := bindingsMap(t, mcpConfig{allowSend: setOf("mail_send")})
	if _, ok := s["mail_send"]; !ok {
		t.Error("--allow-send mail_send should expose mail_send")
	}
	for _, absent := range []string{"mail_flag", "mail_delete", "mail_reply"} {
		if _, ok := s[absent]; ok {
			t.Errorf("naming one send tool must not expose %q", absent)
		}
	}

	// --allow-destructive exposes only the named destructive tool.
	d := bindingsMap(t, mcpConfig{allowDestructive: setOf("mail_delete")})
	if _, ok := d["mail_delete"]; !ok {
		t.Error("--allow-destructive mail_delete should expose mail_delete")
	}
	if _, ok := d["mail_flag"]; ok {
		t.Error("--allow-destructive must not expose a safe write")
	}
}

// TestCuratedRegistry_GuardsVetoExposure verifies --no-send / --no-write hide
// mutating tools even when explicitly named (the capability guard wins).
func TestCuratedRegistry_GuardsVetoExposure(t *testing.T) {
	// --no-send hides send tools even if named (safe writes still allowed).
	ns := bindingsMap(t, mcpConfig{
		noSend:     true,
		allowSend:  setOf("mail_send"),
		allowWrite: setOf("mail_flag"),
	})
	if _, ok := ns["mail_send"]; ok {
		t.Error("--no-send must hide mail_send even when named via --allow-send")
	}
	if _, ok := ns["mail_flag"]; !ok {
		t.Error("--no-send should still allow a named safe write")
	}

	// --no-write hides every mutating tier even when named.
	nw := bindingsMap(t, mcpConfig{
		noWrite:          true,
		allowWrite:       writeToolNames(),
		allowSend:        sendToolNames(),
		allowDestructive: destructiveToolNames(),
	})
	for name, b := range nw {
		if !b.readOnly() {
			t.Errorf("--no-write must hide mutating tool %q", name)
		}
	}
}

// TestCuratedToolsResolve ensures every curated path maps to a real leaf command
// (catches drift if a command is renamed). buildMCPServer errors otherwise.
func TestCuratedToolsResolve(t *testing.T) {
	// Expose every curated tool (all tiers named) so resolution covers them all.
	if _, _, err := buildMCPServer(mcpConfig{
		allowWrite:       writeToolNames(),
		allowSend:        sendToolNames(),
		allowDestructive: destructiveToolNames(),
	}); err != nil {
		t.Fatalf("a curated tool failed to resolve: %v", err)
	}
}

func TestCommandAllowed_FiltersRegistry(t *testing.T) {
	// Only allow mail.list exactly; the registry should shrink to that one tool.
	_, bindings, err := buildMCPServer(mcpConfig{
		allowed: func(path []string) bool {
			return commandAllowed(&RootFlags{EnableCommandsExact: "mail.list"}, path)
		},
	})
	if err != nil {
		t.Fatalf("buildMCPServer: %v", err)
	}
	if len(bindings) != 1 || bindings[0].name != "mail_list" {
		names := make([]string, 0, len(bindings))
		for _, b := range bindings {
			names = append(names, b.name)
		}
		t.Errorf("expected only mail_list, got %v", names)
	}
}

func TestFlagSchema_MailList(t *testing.T) {
	schema := flagSchema(leafByPath(t, "mail", "list"))
	if schema.Type != "object" {
		t.Fatalf("schema type = %q, want object", schema.Type)
	}
	want := map[string]string{"top": "integer", "unread": "boolean", "from": "string"}
	for field, typ := range want {
		prop, ok := schema.Properties[field]
		if !ok {
			t.Errorf("mail_list schema missing %q", field)
			continue
		}
		if prop.Type != typ {
			t.Errorf("mail_list %q type = %q, want %q", field, prop.Type, typ)
		}
	}
}

func TestFlagSchema_RequiredPositional(t *testing.T) {
	schema := flagSchema(leafByPath(t, "mail", "get"))
	if !contains(schema.Required, "id") {
		t.Errorf("mail_get schema Required = %v, want it to include \"id\"", schema.Required)
	}
	if prop := schema.Properties["id"]; prop == nil || prop.Type != "string" {
		t.Errorf("mail_get \"id\" positional should be a string property, got %+v", prop)
	}
}

func TestFlagSchema_Enum(t *testing.T) {
	// drive share is not curated, but flagSchema works on any node.
	schema := flagSchema(leafByPath(t, "drive", "share"))
	prop, ok := schema.Properties["type"]
	if !ok {
		t.Fatal("drive share missing \"type\" flag")
	}
	if !enumContains(prop, "view") || !enumContains(prop, "edit") {
		t.Errorf("drive share \"type\" enum = %v, want view/edit", prop.Enum)
	}
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

func enumContains(s *jsonschema.Schema, want string) bool {
	for _, e := range s.Enum {
		if str, ok := e.(string); ok && strings.EqualFold(str, want) {
			return true
		}
	}
	return false
}

func TestToolSchema_ConciseInjectedForReadOnly(t *testing.T) {
	read := &toolBinding{name: "mail_list", path: []string{"mail", "list"}, node: leafByPath(t, "mail", "list"), tier: tierRead}
	if _, ok := toolSchema(read).Properties[conciseArg]; !ok {
		t.Error("read tool schema should include synthetic concise property")
	}
	write := &toolBinding{name: "mail_drafts_create", path: []string{"mail", "drafts", "create"}, node: leafByPath(t, "mail", "drafts", "create"), tier: tierSafeWrite}
	if _, ok := toolSchema(write).Properties[conciseArg]; ok {
		t.Error("write tool schema must not include concise property")
	}
}

// TestCuratedReadToolCount pins the default (read-only) registry size so the
// "N read tools" figure in README.md can't silently drift. If you add or remove
// a read tool, update both this number and the README.
func TestCuratedReadToolCount(t *testing.T) {
	const documentedReadTools = 38
	if got := len(buildBindings(t)); got != documentedReadTools {
		t.Errorf("default read-only registry has %d tools; expected %d — update README.md and this constant in lockstep", got, documentedReadTools)
	}
}

// TestCuratedRegistry_SafeWritesOptIn verifies the Phase 4 safe writes are
// hidden by default and appear only when named via --allow-write (per-tool).
func TestCuratedRegistry_SafeWritesOptIn(t *testing.T) {
	safeWrites := []string{
		"mail_flag", "mail_categorize", "mail_mark", "mail_move",
		"mail_folders_create", "mail_folders_rename",
		"contacts_create", "contacts_update", "todo_update", "todo_complete",
	}
	def := buildBindings(t) // no writes named
	for _, name := range safeWrites {
		if _, ok := def[name]; ok {
			t.Errorf("safe write %q must be hidden until named via --allow-write", name)
		}
	}
	// Each is eligible (a known write tool) and appears when named.
	known := writeToolNames()
	for _, name := range safeWrites {
		if !known[name] {
			t.Errorf("%q should be a curated write tool", name)
		}
		if _, ok := buildBindings(t, name)[name]; !ok {
			t.Errorf("%q should appear when named via --allow-write", name)
		}
	}
}
