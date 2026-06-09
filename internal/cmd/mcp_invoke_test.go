package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestBuildArgv_AlwaysAppendsJSON(t *testing.T) {
	b := testBinding(t, "mail", "list")
	argv, err := buildArgv(b, map[string]any{})
	if err != nil {
		t.Fatalf("buildArgv: %v", err)
	}
	if !contains(argv, "--json") {
		t.Errorf("argv %v missing --json", argv)
	}
	if argv[0] != "mail" || argv[1] != "list" {
		t.Errorf("argv should start with command path, got %v", argv)
	}
}

func TestBuildArgv_Scalars(t *testing.T) {
	b := testBinding(t, "mail", "list")
	// JSON numbers decode to float64.
	argv, err := buildArgv(b, map[string]any{"top": float64(10), "from": "a@b.com", "unread": true})
	if err != nil {
		t.Fatalf("buildArgv: %v", err)
	}
	assertAdjacent(t, argv, "--top", "10")
	assertAdjacent(t, argv, "--from", "a@b.com")
	if !contains(argv, "--unread") {
		t.Errorf("argv %v missing --unread", argv)
	}
}

func TestBuildArgv_BoolFalseOmitted(t *testing.T) {
	b := testBinding(t, "mail", "list")
	argv, err := buildArgv(b, map[string]any{"unread": false})
	if err != nil {
		t.Fatalf("buildArgv: %v", err)
	}
	if contains(argv, "--unread") {
		t.Errorf("false bool should be omitted, got %v", argv)
	}
}

func TestBuildArgv_SliceRepeats(t *testing.T) {
	b := testBinding(t, "mail", "send")
	argv, err := buildArgv(b, map[string]any{
		"to":      []any{"a@b.com", "c@d.com"},
		"subject": "Hi",
	})
	if err != nil {
		t.Fatalf("buildArgv: %v", err)
	}
	count := 0
	for i := 0; i < len(argv)-1; i++ {
		if argv[i] == "--to" {
			count++
		}
	}
	if count != 2 {
		t.Errorf("expected --to repeated twice, got %d in %v", count, argv)
	}
}

func TestBuildArgv_PositionalsAfterSeparator(t *testing.T) {
	b := testBinding(t, "mail", "get")
	argv, err := buildArgv(b, map[string]any{"id": "AAMkABC"})
	if err != nil {
		t.Fatalf("buildArgv: %v", err)
	}
	sep := indexOf(argv, "--")
	if sep < 0 {
		t.Fatalf("expected -- separator in %v", argv)
	}
	// --json must come before the separator.
	if j := indexOf(argv, "--json"); j < 0 || j > sep {
		t.Errorf("--json (%d) must precede -- (%d) in %v", j, sep, argv)
	}
	// the positional value must come after the separator.
	if v := indexOf(argv, "AAMkABC"); v < sep {
		t.Errorf("positional should follow --, got %v", argv)
	}
}

func TestBuildArgv_MissingRequiredPositional(t *testing.T) {
	b := testBinding(t, "mail", "get")
	if _, err := buildArgv(b, map[string]any{}); err == nil {
		t.Error("expected error for missing required positional, got nil")
	}
}

func TestSprintArg(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{float64(25), "25"},
		{float64(1000000), "1000000"},
		{float64(1.5), "1.5"},
		{true, "true"},
		{false, "false"},
		{"hello", "hello"},
	}
	for _, tc := range cases {
		if got := sprintArg(tc.in); got != tc.want {
			t.Errorf("sprintArg(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func assertAdjacent(t *testing.T, argv []string, flag, value string) {
	t.Helper()
	for i := 0; i < len(argv)-1; i++ {
		if argv[i] == flag {
			if argv[i+1] == value {
				return
			}
			t.Errorf("%s followed by %q, want %q (argv %v)", flag, argv[i+1], value, argv)
			return
		}
	}
	t.Errorf("flag %s not found in %v", flag, argv)
}

func indexOf(ss []string, s string) int {
	for i, v := range ss {
		if v == s {
			return i
		}
	}
	return -1
}

func TestRejectUnknownArgs(t *testing.T) {
	b := testBinding(t, "mail", "list")
	// A declared flag is accepted.
	if err := rejectUnknownArgs(b, map[string]any{"top": float64(5)}); err != nil {
		t.Errorf("declared flag rejected: %v", err)
	}
	// An undeclared key is rejected (gog fixed-schema contract).
	if err := rejectUnknownArgs(b, map[string]any{"bogus": "x"}); err == nil {
		t.Error("expected error for unknown argument, got nil")
	}
	// A declared positional is accepted.
	get := testBinding(t, "mail", "get")
	if err := rejectUnknownArgs(get, map[string]any{"id": "AAA"}); err != nil {
		t.Errorf("declared positional rejected: %v", err)
	}
}

func TestClassifyError(t *testing.T) {
	cases := []struct {
		msg      string
		wantCode string
	}{
		{"no account configured", "unauthenticated"},
		{"error: InsufficientScope: Read.Shared required", "forbidden"},
		{"ErrorItemNotFound", "not_found"},
		{"TooManyRequests", "rate_limited"},
		{"unknown argument \"x\"", "invalid_input"},
		{"some other failure", "error"},
	}
	for _, tc := range cases {
		if code, _ := classifyError(tc.msg); code != tc.wantCode {
			t.Errorf("classifyError(%q) code = %q, want %q", tc.msg, code, tc.wantCode)
		}
	}
}

func TestErrorResult_StructuredEnvelope(t *testing.T) {
	res := errorResult("no account configured")
	if !res.IsError {
		t.Fatal("expected IsError")
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", res.Content[0])
	}
	var env map[string]string
	if err := json.Unmarshal([]byte(tc.Text), &env); err != nil {
		t.Fatalf("error body is not JSON: %v\n%s", err, tc.Text)
	}
	if env["code"] != "unauthenticated" {
		t.Errorf("code = %q, want unauthenticated", env["code"])
	}
	if env["action"] == "" {
		t.Error("expected a non-empty action")
	}
}

func TestCapText(t *testing.T) {
	if got := capText("short", 100); got != "short" {
		t.Errorf("under-cap text mutated: %q", got)
	}
	long := strings.Repeat("a", 500)
	got := capText(long, 100)
	if len(got) <= 100 || !strings.Contains(got, "truncated") {
		t.Errorf("expected truncation notice; len=%d", len(got))
	}
}

func TestBuildArgv_ConciseReadTool(t *testing.T) {
	b := testBinding(t, "mail", "list") // testBinding sets readOnly:true
	argv, err := buildArgv(b, map[string]any{"concise": true})
	if err != nil {
		t.Fatalf("buildArgv: %v", err)
	}
	if !contains(argv, "--concise") {
		t.Errorf("expected --concise in argv for read tool, got %v", argv)
	}
	// concise:false must not emit the flag.
	argv, _ = buildArgv(b, map[string]any{"concise": false})
	if contains(argv, "--concise") {
		t.Errorf("concise=false should omit --concise, got %v", argv)
	}
}

func TestRejectUnknownArgs_ConciseAllowedForReadOnly(t *testing.T) {
	read := testBinding(t, "mail", "list") // readOnly:true
	if err := rejectUnknownArgs(read, map[string]any{"concise": true}); err != nil {
		t.Errorf("concise should be accepted on a read tool: %v", err)
	}
	write := &toolBinding{name: "mail_drafts_create", path: []string{"mail", "drafts", "create"}, node: leafByPath(t, "mail", "drafts", "create"), readOnly: false}
	if err := rejectUnknownArgs(write, map[string]any{"concise": true}); err == nil {
		t.Error("concise should be rejected on a write tool")
	}
}

func TestToolSelectorPredicate(t *testing.T) {
	// nil when empty → caller treats as allow-all.
	if toolSelectorPredicate(nil) != nil {
		t.Error("no selectors should yield a nil predicate")
	}
	cases := []struct {
		sels     []string
		name     string
		readOnly bool
		want     bool
	}{
		{[]string{"all"}, "mail_list", true, true},
		{[]string{"*"}, "todo_create", false, true},
		{[]string{"read"}, "mail_list", true, true},
		{[]string{"read"}, "todo_create", false, false},
		{[]string{"write"}, "todo_create", false, true},
		{[]string{"write"}, "mail_list", true, false},
		{[]string{"mail_*"}, "mail_get", true, true},
		{[]string{"mail.*"}, "mail_get", true, true}, // dot normalized to underscore
		{[]string{"mail_*"}, "calendar_get", true, false},
		{[]string{"mail_list"}, "mail_list", true, true},
		{[]string{"mail_list"}, "mail_get", true, false},
		{[]string{"calendar_*", "todo_get"}, "todo_get", true, true}, // any-match
	}
	for _, tc := range cases {
		pred := toolSelectorPredicate(tc.sels)
		if got := pred(tc.name, tc.readOnly); got != tc.want {
			t.Errorf("selectors %v on (%q,ro=%v) = %v, want %v", tc.sels, tc.name, tc.readOnly, got, tc.want)
		}
	}
}

func TestBuildMCPServer_AllowToolNarrows(t *testing.T) {
	_, bindings, err := buildMCPServer(mcpConfig{allowTool: toolSelectorPredicate([]string{"mail_*"})})
	if err != nil {
		t.Fatalf("buildMCPServer: %v", err)
	}
	if len(bindings) == 0 {
		t.Fatal("expected some mail_* tools")
	}
	for _, b := range bindings {
		if !strings.HasPrefix(b.name, "mail_") {
			t.Errorf("allow-tool mail_* leaked non-mail tool %q", b.name)
		}
	}
}
