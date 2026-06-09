package cmd

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// connectE2E wires an in-memory client session to a freshly built server.
func connectE2E(t *testing.T, allowWrite bool) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()

	srv, _, err := buildMCPServer(mcpConfig{allowWrite: allowWrite})
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

func resultText(res *mcp.CallToolResult) string {
	var sb strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String()
}

func TestE2E_ListTools(t *testing.T) {
	cs := connectE2E(t, true)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(res.Tools) == 0 {
		t.Fatal("expected non-empty tool list")
	}
	names := map[string]bool{}
	for _, tool := range res.Tools {
		names[tool.Name] = true
	}
	// Curated read tools are present; destructive tools never are.
	for _, want := range []string{"version", "mail_list", "calendar_events"} {
		if !names[want] {
			t.Errorf("ListTools missing %q", want)
		}
	}
	if names["mail_delete"] {
		t.Error("destructive mail_delete must never be exposed")
	}
}

// TestE2E_CallVersion exercises the whole pipeline (schema -> argv -> kong parse
// -> run -> capture -> result) with a network-free command.
func TestE2E_CallVersion(t *testing.T) {
	cs := connectE2E(t, false)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "version",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool(version): %v", err)
	}
	if res.IsError {
		t.Fatalf("version call reported error: %q", resultText(res))
	}
	if strings.TrimSpace(resultText(res)) == "" {
		t.Error("version call returned empty content")
	}
}

// TestE2E_ErrorPath confirms the IsError contract: a command that needs an
// account, run with an empty config dir, surfaces a tool error (not a transport
// error) with diagnostics in the content.
func TestE2E_ErrorPath(t *testing.T) {
	t.Setenv("OLK_CONFIG_DIR", t.TempDir())
	t.Setenv("OLK_ACCOUNT", "")

	cs := connectE2E(t, false)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "mail_list",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool(mail_list) transport error: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected IsError for mail_list with no account, got: %q", resultText(res))
	}
	if strings.TrimSpace(resultText(res)) == "" {
		t.Error("error result should include diagnostics")
	}
}
