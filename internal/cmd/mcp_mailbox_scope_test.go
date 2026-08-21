package cmd

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// mailboxScopedUnawareTools is the expected third class, written out rather than
// derived so that adding a curated tool has to state which class it falls in.
// Every one of these reads or writes mailbox-scoped data while ignoring
// --mailbox, which is why a launch mailbox withholds them.
var mailboxScopedUnawareTools = map[string]bool{
	"mail_attachments": true, "mail_categories_list": true, "mail_rules_list": true,
	"mail_ooo_get": true, "mail_flag": true, "mail_categorize": true,
	"mail_mark": true, "mail_move": true, "mail_folders_create": true,
	"mail_folders_rename": true, "mail_delete": true,
	"calendar_availability": true, "calendar_find_times": true,
	"calendar_respond": true, "calendar_create": true, "calendar_update": true,
	"calendar_delete": true,
	"contacts_create": true, "contacts_update": true, "contacts_delete": true,
}

// Each curated tool belongs to exactly one class. A new tool that nobody
// classified lands in scoped-but-unaware by default, which is the safe side —
// it is withheld under a launch mailbox — but this test still fails, so the
// choice is made deliberately rather than inherited.
func TestCuratedToolsAreClassifiedForMailboxScoping(t *testing.T) {
	classes := []struct {
		name  string
		set   map[string]bool
		known map[string]bool
	}{
		{name: "mailbox-aware", set: mailboxAwareTools},
		{name: "mailbox-irrelevant", set: mailboxIrrelevantTools},
		{name: "scoped-but-unaware", set: mailboxScopedUnawareTools},
	}

	curated := map[string]bool{}
	for _, ct := range curatedTools {
		curated[ct.name] = true
		var in []string
		for _, c := range classes {
			if c.set[ct.name] {
				in = append(in, c.name)
			}
		}
		if len(in) != 1 {
			sort.Strings(in)
			t.Errorf("curated tool %q is in %d mailbox classes (%s), want exactly 1",
				ct.name, len(in), strings.Join(in, ", "))
		}
		if got := mailboxScopedButUnaware(ct.name); got != mailboxScopedUnawareTools[ct.name] {
			t.Errorf("mailboxScopedButUnaware(%q) = %v, want %v", ct.name, got, mailboxScopedUnawareTools[ct.name])
		}
	}

	for _, c := range classes {
		for name := range c.set {
			if !curated[name] {
				t.Errorf("%s names %q, which is not a curated tool", c.name, name)
			}
		}
	}
}

// The finding this closes: a server started with --mailbox appended the flag to
// every tool call, including tools whose command ignores it, so a delete or a
// calendar write landed in the operator's own mailbox instead of the one the
// server was configured for.
func TestLaunchMailboxWithholdsToolsThatCannotHonourIt(t *testing.T) {
	allow := func(names ...string) map[string]bool {
		m := map[string]bool{}
		for _, n := range names {
			m[n] = true
		}
		return m
	}
	cfg := &mcpConfig{
		allowWrite:       allow("mail_flag", "mail_drafts_create", "contacts_create", "todo_create"),
		allowSend:        allow("mail_send", "calendar_create"),
		allowDestructive: allow("mail_delete", "todo_delete"),
		env:              callEnv{mailbox: "team@example.com"},
	}
	_, bindings, err := buildMCPServer(cfg)
	if err != nil {
		t.Fatalf("buildMCPServer: %v", err)
	}

	exposed := map[string]bool{}
	for _, b := range bindings {
		exposed[b.name] = true
		if mailboxScopedUnawareTools[b.name] {
			t.Errorf("tool %q ignores --mailbox but is exposed on a server serving %q",
				b.name, cfg.env.mailbox)
		}
	}

	// Withholding must be surgical: tools that honour the flag, and tools with no
	// mailbox dimension at all, are untouched by the choice.
	for _, want := range []string{"mail_list", "mail_send", "mail_drafts_create"} {
		if !exposed[want] {
			t.Errorf("tool %q honours --mailbox and should still be exposed", want)
		}
	}
	for _, want := range []string{"drive_ls", "todo_create", "todo_delete", "people_search", "whoami"} {
		if !exposed[want] {
			t.Errorf("tool %q has no mailbox dimension and should be unaffected", want)
		}
	}

	// Without a launch mailbox nothing is withheld, so the default server is
	// exactly what it was.
	_, plain, err := buildMCPServer(&mcpConfig{
		allowWrite:       cfg.allowWrite,
		allowSend:        cfg.allowSend,
		allowDestructive: cfg.allowDestructive,
	})
	if err != nil {
		t.Fatalf("buildMCPServer: %v", err)
	}
	for _, want := range []string{"mail_flag", "contacts_create", "calendar_create", "mail_delete"} {
		found := false
		for _, b := range plain {
			if b.name == want {
				found = true
			}
		}
		if !found {
			t.Errorf("tool %q should be exposed when no launch mailbox is set", want)
		}
	}
}

// Registration is the first layer. This is the second: were one of these tools
// ever registered in error, building its argv must still refuse rather than run
// it against the wrong mailbox.
func TestBuildArgv_RefusesAToolThatCannotHonourTheLaunchMailbox(t *testing.T) {
	b := &toolBinding{
		name: "mail_flag",
		path: []string{"mail", "flag"},
		node: leafByPath(t, "mail", "flag"),
		tier: tierSafeWrite,
		env:  callEnv{mailbox: "team@example.com"},
	}
	_, err := buildArgv(b, map[string]any{"id": "message-id", "status": "flagged"})
	if err == nil {
		t.Fatal("expected a refusal for a tool that ignores --mailbox")
	}
	if !strings.Contains(err.Error(), "team@example.com") {
		t.Errorf("refusal %q should name the mailbox it could not honour", err)
	}

	// A tool that honours the flag is unaffected.
	aware := &toolBinding{
		name: "mail_list",
		path: []string{"mail", "list"},
		node: leafByPath(t, "mail", "list"),
		tier: tierRead,
		env:  callEnv{mailbox: "team@example.com"},
	}
	argv, err := buildArgv(aware, map[string]any{})
	if err != nil {
		t.Fatalf("buildArgv for a mailbox-aware tool: %v", err)
	}
	if !strings.Contains(strings.Join(argv, " "), "--mailbox team@example.com") {
		t.Errorf("argv = %v, want the launch mailbox", argv)
	}
}

// --force is classified as argv-resolved: buildArgv supplies it for destructive
// tools, and nothing carries a launch --force. That classification only holds
// while no other curated command consults the flag, so check the sources.
func TestNoNonDestructiveCuratedCommandConsultsForce(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading package directory: %v", err)
	}
	fset := token.NewFileSet()
	readsForce := map[string]bool{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name.Name != "Run" || fn.Recv == nil || len(fn.Recv.List) != 1 {
				continue
			}
			star, ok := fn.Recv.List[0].Type.(*ast.StarExpr)
			if !ok {
				continue
			}
			ident, ok := star.X.(*ast.Ident)
			if !ok {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if ok && sel.Sel.Name == "Force" {
					readsForce[ident.Name] = true
				}
				return true
			})
		}
	}
	if len(readsForce) == 0 {
		t.Fatal("found no commands reading Force; the source scan is broken")
	}

	cli := &CLI{}
	k, err := newKongParser(cli)
	if err != nil {
		t.Fatalf("parser: %v", err)
	}
	leaves := indexLeaves(k.Model.Node)
	for _, ct := range curatedTools {
		if ct.tier == tierDestructive {
			continue
		}
		node, ok := leaves[strings.Join(ct.path, ".")]
		if !ok {
			t.Fatalf("curated tool %q has no command", ct.name)
		}
		typ := node.Target.Type()
		if typ.Kind() == reflect.Pointer {
			typ = typ.Elem()
		}
		if readsForce[typ.Name()] {
			t.Errorf("curated tool %q is not destructive yet %s reads --force; an ambient "+
				"OLK_FORCE would then reach it, so either reclassify the tool or carry the flag",
				ct.name, typ.Name())
		}
	}
}
