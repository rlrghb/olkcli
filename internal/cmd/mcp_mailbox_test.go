package cmd

import (
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func mailboxEnv(allowed ...string) callEnv {
	return callEnv{allowMailbox: allowed}
}

func bindingFor(t *testing.T, name string, env callEnv, path ...string) *toolBinding {
	t.Helper()
	return &toolBinding{name: name, path: path, node: leafByPath(t, path...), tier: tierRead, env: env}
}

// Without --allow-mailbox the property must not appear anywhere: the default
// server behaves exactly as it did before agents could name a mailbox.
func TestMailboxArg_AbsentWithoutAllowlist(t *testing.T) {
	_, bindings, err := buildMCPServer(&mcpConfig{})
	if err != nil {
		t.Fatalf("buildMCPServer: %v", err)
	}
	for _, b := range bindings {
		if _, ok := toolSchema(b).Properties[mailboxArg]; ok {
			t.Errorf("tool %q offers %q with no --allow-mailbox set", b.name, mailboxArg)
		}
	}
}

// With an allowlist, only tools that honour the flag advertise it, and the
// permitted values are enumerated in the schema.
func TestMailboxArg_OnlyOnMailboxAwareTools(t *testing.T) {
	env := mailboxEnv("team@example.com", "boss@example.com")
	_, bindings, err := buildMCPServer(&mcpConfig{env: env})
	if err != nil {
		t.Fatalf("buildMCPServer: %v", err)
	}
	seen := map[string]bool{}
	for _, b := range bindings {
		_, ok := toolSchema(b).Properties[mailboxArg]
		seen[b.name] = ok
		if ok != mailboxAwareTools[b.name] {
			t.Errorf("tool %q: offers mailbox arg = %v, want %v", b.name, ok, mailboxAwareTools[b.name])
		}
	}
	if !seen["mail_list"] {
		t.Error("mail_list should offer a mailbox argument")
	}
	if seen["drive_ls"] || seen["todo_list"] || seen["whoami"] {
		t.Error("tools that ignore --mailbox must not offer the argument")
	}

	b := bindingFor(t, "mail_list", env, "mail", "list")
	enum := toolSchema(b).Properties[mailboxArg].Enum
	if len(enum) != 2 || enum[0] != "team@example.com" {
		t.Fatalf("enum = %v, want the permitted mailboxes", enum)
	}
}

// The mailbox a server was started with must reach every call. This is the bug
// the launch-time context exists to fix: the rebuilt argv otherwise carries none
// of the operator's global flags.
func TestBuildArgv_LaunchMailboxReachesTheCall(t *testing.T) {
	b := bindingFor(t, "mail_list", callEnv{mailbox: "boss@example.com"}, "mail", "list")
	argv, err := buildArgv(b, nil)
	if err != nil {
		t.Fatalf("buildArgv: %v", err)
	}
	if !containsPair(argv, "--mailbox", "boss@example.com") {
		t.Fatalf("argv = %v, want the launch mailbox", argv)
	}
}

func TestBuildArgv_PerCallMailboxOverridesLaunchDefault(t *testing.T) {
	env := mailboxEnv("team@example.com")
	env.mailbox = "boss@example.com"
	b := bindingFor(t, "mail_list", env, "mail", "list")
	argv, err := buildArgv(b, map[string]any{mailboxArg: "team@example.com"})
	if err != nil {
		t.Fatalf("buildArgv: %v", err)
	}
	if !containsPair(argv, "--mailbox", "team@example.com") {
		t.Fatalf("argv = %v, want the per-call mailbox to win", argv)
	}
}

// The schema enum is advice to the model; this rejection is the actual guard.
func TestBuildArgv_UnpermittedMailboxRefused(t *testing.T) {
	b := bindingFor(t, "mail_list", mailboxEnv("team@example.com"), "mail", "list")
	_, err := buildArgv(b, map[string]any{mailboxArg: "ceo@example.com"})
	if err == nil || !strings.Contains(err.Error(), "not permitted") {
		t.Fatalf("want a refusal for an unlisted mailbox, got %v", err)
	}
}

// A tool that ignores --mailbox must refuse the argument rather than accept it
// and silently act on the wrong mailbox.
func TestRejectUnknownArgs_MailboxOnlyOnAwareTools(t *testing.T) {
	env := mailboxEnv("team@example.com")
	aware := bindingFor(t, "mail_list", env, "mail", "list")
	if err := rejectUnknownArgs(aware, map[string]any{mailboxArg: "team@example.com"}); err != nil {
		t.Fatalf("mail_list should accept a mailbox argument: %v", err)
	}
	unaware := bindingFor(t, "todo_list", env, "todo", "list")
	if err := rejectUnknownArgs(unaware, map[string]any{mailboxArg: "team@example.com"}); err == nil {
		t.Fatal("todo_list must refuse a mailbox argument")
	}
}

// Account and timeout are dropped by the argv rebuild unless carried over, and
// the capability guards must only ever tighten.
func TestApplyLaunchEnv_CarriesGlobalsAndOnlyTightensGuards(t *testing.T) {
	cli := &CLI{}
	applyLaunchEnv(cli, callEnv{account: "svc@example.com", timeout: 120, noSend: true, noWrite: true})
	if cli.Account != "svc@example.com" || cli.Timeout != 120 {
		t.Fatalf("account=%q timeout=%d, want the launch values", cli.Account, cli.Timeout)
	}
	if !cli.NoSend || !cli.NoWrite {
		t.Fatal("launch guards must apply to every call")
	}

	// No tool offers an account or a timeout, so a value surviving the per-call
	// parse came from an ambient OLK_* variable. The operator's command line must
	// outrank it, or a stray environment variable silently redirects every call to
	// another identity.
	ambient := &CLI{}
	ambient.Account = "personal@example.com"
	ambient.Timeout = 60
	applyLaunchEnv(ambient, callEnv{account: "svc@example.com", timeout: 120})
	if ambient.Account != "svc@example.com" {
		t.Errorf("account = %q, want the mailbox the server was started with", ambient.Account)
	}
	if ambient.Timeout != 120 {
		t.Errorf("timeout = %d, want the launch value", ambient.Timeout)
	}

	// With nothing set at launch, the ambient environment still applies.
	envOnly := &CLI{}
	envOnly.Account = "personal@example.com"
	envOnly.Timeout = 60
	applyLaunchEnv(envOnly, callEnv{})
	if envOnly.Account != "personal@example.com" || envOnly.Timeout != 60 {
		t.Errorf("account=%q timeout=%d, want the environment's values left alone",
			envOnly.Account, envOnly.Timeout)
	}

	// A call must never be able to lift a guard the server was started with.
	guarded := &CLI{}
	guarded.NoSend = true
	applyLaunchEnv(guarded, callEnv{noSend: false})
	if !guarded.NoSend {
		t.Error("a call must not be able to lift its own --no-send")
	}
}

func TestResolveAllowMailbox(t *testing.T) {
	got, err := resolveAllowMailbox([]string{"team@example.com, boss@example.com", "TEAM@example.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %v, want two mailboxes after splitting and de-duplicating", got)
	}
	if _, err := resolveAllowMailbox([]string{"not-an-address"}); err == nil {
		t.Fatal("a malformed mailbox must be refused, not silently dropped")
	}
}

// mailboxAwareTools is hand-maintained, so this checks it against the source: a
// command is mailbox-aware exactly when its Run method resolves a target. A
// mismatch either hides the argument on a tool that supports it, or offers
// scoping that never happens.
func TestMailboxAwareTools_MatchesSource(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading package directory: %v", err)
	}
	fset := token.NewFileSet()
	resolvesTarget := map[string]bool{}
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
				if id, ok := n.(*ast.Ident); ok && id.Name == "resolveMailboxTarget" {
					resolvesTarget[ident.Name] = true
				}
				return true
			})
		}
	}
	if len(resolvesTarget) == 0 {
		t.Fatal("found no commands resolving a mailbox target; the source scan is broken")
	}

	cli := &CLI{}
	k, err := newKongParser(cli)
	if err != nil {
		t.Fatalf("parser: %v", err)
	}
	leaves := indexLeaves(k.Model.Node)
	for _, ct := range curatedTools {
		node, ok := leaves[strings.Join(ct.path, ".")]
		if !ok {
			t.Fatalf("curated tool %q has no command", ct.name)
		}
		typ := node.Target.Type()
		if typ.Kind() == reflect.Pointer {
			typ = typ.Elem()
		}
		want := resolvesTarget[typ.Name()]
		if mailboxAwareTools[ct.name] != want {
			t.Errorf("mailboxAwareTools[%q] = %v, but %s %s resolveMailboxTarget",
				ct.name, mailboxAwareTools[ct.name], typ.Name(),
				map[bool]string{true: "calls", false: "does not call"}[want])
		}
	}
}

func containsPair(argv []string, flag, value string) bool {
	for i := 0; i+1 < len(argv); i++ {
		if argv[i] == flag && argv[i+1] == value {
			return true
		}
	}
	return false
}

// connectMailboxE2E wires an in-memory client to a server started with a
// permitted-mailbox list and send tools exposed.
func connectMailboxE2E(t *testing.T, allowed ...string) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	srv, _, err := buildMCPServer(&mcpConfig{
		allowSend: map[string]bool{"mail_send": true},
		env:       callEnv{allowMailbox: allowed},
	})
	if err != nil {
		t.Fatalf("buildMCPServer: %v", err)
	}
	serverT, clientT := mcp.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, serverT, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { _ = ss.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "olk-test", Version: "v0"}, nil)
	cs, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

// What an agent actually receives over the protocol: the permitted mailboxes,
// enumerated, and only on tools that honour them.
func TestE2E_MailboxArgIsAdvertisedWithItsPermittedValues(t *testing.T) {
	cs := connectMailboxE2E(t, "team@example.com")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	// The schema crosses the wire untyped, so decode it the way a client would.
	for _, tool := range res.Tools {
		raw, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatalf("marshal schema for %q: %v", tool.Name, err)
		}
		var schema struct {
			Properties map[string]struct {
				Enum []any `json:"enum"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatalf("decode schema for %q: %v", tool.Name, err)
		}
		prop, ok := schema.Properties[mailboxArg]
		if ok != mailboxAwareTools[tool.Name] {
			t.Errorf("tool %q advertises mailbox = %v, want %v", tool.Name, ok, mailboxAwareTools[tool.Name])
		}
		if ok && (len(prop.Enum) != 1 || prop.Enum[0] != "team@example.com") {
			t.Errorf("tool %q enum = %v, want the permitted mailbox", tool.Name, prop.Enum)
		}
	}
}

// An agent naming a mailbox it was not granted is refused before the call
// reaches Graph, so no message leaves and no mailbox is read.
func TestE2E_UnpermittedMailboxIsRefused(t *testing.T) {
	cs := connectMailboxE2E(t, "team@example.com")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name: "mail_send",
		Arguments: map[string]any{
			"to": []any{"someone@example.com"}, "subject": "s", "body": "b",
			mailboxArg: "ceo@example.com",
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("naming an unpermitted mailbox must fail the call")
	}
	if text := resultText(res); !strings.Contains(text, "not permitted") {
		t.Fatalf("result = %q, want a refusal naming the restriction", text)
	}
}
