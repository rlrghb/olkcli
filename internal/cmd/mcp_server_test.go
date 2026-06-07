package cmd

import (
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
)

// bindingsByName builds the server for a profile and indexes the bindings.
func bindingsByName(t *testing.T, profile string) map[string]*toolBinding {
	t.Helper()
	_, bindings, err := buildMCPServer(profile)
	if err != nil {
		t.Fatalf("buildMCPServer(%q): %v", profile, err)
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

func TestBuildMCPServer_ProfileMembership(t *testing.T) {
	safe := bindingsByName(t, "safe")
	full := bindingsByName(t, "full")

	// Reads and safe writes appear in both.
	for _, name := range []string{"mail_list", "mail_get", "mail_send", "calendar_events", "whoami", "version"} {
		if _, ok := safe[name]; !ok {
			t.Errorf("expected %q in safe profile", name)
		}
		if _, ok := full[name]; !ok {
			t.Errorf("expected %q in full profile", name)
		}
	}

	// Destructive tools: full only.
	for _, name := range []string{"mail_delete", "drive_rm", "contacts_delete", "calendar_delete"} {
		if _, ok := safe[name]; ok {
			t.Errorf("destructive %q must NOT be in safe profile", name)
		}
		if _, ok := full[name]; !ok {
			t.Errorf("destructive %q expected in full profile", name)
		}
	}

	// The mcp command and interactive auth login are excluded from both.
	for _, name := range []string{"mcp", "auth_login"} {
		if _, ok := safe[name]; ok {
			t.Errorf("%q must not be exposed (safe)", name)
		}
		if _, ok := full[name]; ok {
			t.Errorf("%q must not be exposed (full)", name)
		}
	}

	if len(full) <= len(safe) {
		t.Errorf("full profile (%d tools) should expose more than safe (%d)", len(full), len(safe))
	}
}

// TestSafeProfileHasNoDestructiveLeaf is the guard test: no exposed safe tool
// may have a destructive final path token.
func TestSafeProfileHasNoDestructiveLeaf(t *testing.T) {
	safe := bindingsByName(t, "safe")
	for name, b := range safe {
		leaf := b.path[len(b.path)-1]
		if destructiveVerbs[leaf] {
			t.Errorf("safe profile exposes destructive tool %q (leaf %q)", name, leaf)
		}
		if b.kind == kindInteractive {
			t.Errorf("safe profile exposes interactive tool %q", name)
		}
	}
}

func TestFlagSchema_MailList(t *testing.T) {
	full := bindingsByName(t, "full")
	b, ok := full["mail_list"]
	if !ok {
		t.Fatal("mail_list tool missing")
	}
	schema := flagSchema(b.node)
	if schema.Type != "object" {
		t.Fatalf("schema type = %q, want object", schema.Type)
	}

	want := map[string]string{
		"top":    "integer",
		"unread": "boolean",
		"from":   "string",
	}
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
	full := bindingsByName(t, "full")
	b, ok := full["mail_get"]
	if !ok {
		t.Fatal("mail_get tool missing")
	}
	schema := flagSchema(b.node)
	if !contains(schema.Required, "id") {
		t.Errorf("mail_get schema Required = %v, want it to include \"id\"", schema.Required)
	}
	if prop := schema.Properties["id"]; prop == nil || prop.Type != "string" {
		t.Errorf("mail_get \"id\" positional should be a string property, got %+v", prop)
	}
}

func TestFlagSchema_Enum(t *testing.T) {
	full := bindingsByName(t, "full")
	b, ok := full["drive_share"]
	if !ok {
		t.Skip("drive_share not present")
	}
	schema := flagSchema(b.node)
	prop, ok := schema.Properties["type"]
	if !ok {
		t.Fatal("drive_share missing \"type\" flag")
	}
	if !enumContains(prop, "view") || !enumContains(prop, "edit") {
		t.Errorf("drive_share \"type\" enum = %v, want view/edit", prop.Enum)
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
