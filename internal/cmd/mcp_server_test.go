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
	return &toolBinding{name: strings.Join(path, "_"), path: path, node: leafByPath(t, path...), readOnly: true}
}

func buildBindings(t *testing.T, allowWrite bool) map[string]*toolBinding {
	t.Helper()
	_, bindings, err := buildMCPServer(mcpConfig{allowWrite: allowWrite})
	if err != nil {
		t.Fatalf("buildMCPServer(allowWrite=%v): %v", allowWrite, err)
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

func TestCuratedRegistry_DefaultIsReadOnly(t *testing.T) {
	def := buildBindings(t, false)

	for _, name := range []string{"mail_list", "mail_get", "mail_search", "calendar_events", "whoami", "version"} {
		if _, ok := def[name]; !ok {
			t.Errorf("expected read tool %q in default registry", name)
		}
	}
	// Write tools are absent without --allow-write.
	for _, name := range []string{"mail_drafts_create", "todo_create"} {
		if _, ok := def[name]; ok {
			t.Errorf("write tool %q must not appear without --allow-write", name)
		}
	}
	for _, b := range def {
		if !b.readOnly {
			t.Errorf("default registry exposed non-read tool %q", b.name)
		}
	}
}

func TestCuratedRegistry_AllowWriteAddsSafeWrites(t *testing.T) {
	full := buildBindings(t, true)
	for _, name := range []string{"mail_drafts_create", "todo_create"} {
		if _, ok := full[name]; !ok {
			t.Errorf("expected %q with --allow-write", name)
		}
	}
	if len(full) <= len(buildBindings(t, false)) {
		t.Error("--allow-write should expose strictly more tools")
	}
}

// TestCuratedRegistry_NoDestructiveOrSend is the guard: the curated set must
// never include a delete/send/destructive command, even behind --allow-write.
func TestCuratedRegistry_NoDestructiveOrSend(t *testing.T) {
	forbidden := []string{"delete", "rm", "send", "reply", "forward", "move", "logout", "clean"}
	for _, ct := range curatedTools {
		for _, tok := range ct.path {
			for _, f := range forbidden {
				if tok == f {
					t.Errorf("curated tool %q includes forbidden verb %q", ct.name, f)
				}
			}
		}
		if ct.name == "mail_drafts_send" || ct.name == "auth_login" {
			t.Errorf("curated tool %q must not be exposed", ct.name)
		}
	}
}

// TestCuratedToolsResolve ensures every curated path maps to a real leaf command
// (catches drift if a command is renamed). buildMCPServer errors otherwise.
func TestCuratedToolsResolve(t *testing.T) {
	if _, _, err := buildMCPServer(mcpConfig{allowWrite: true}); err != nil {
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
