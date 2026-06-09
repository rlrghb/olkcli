package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPCmd runs a stdio MCP server exposing a curated, read-first set of olk
// commands as tools (mirroring gog's "typed tools, no arbitrary command
// bridge" model). The server is read-only by default; write tools are exposed
// only when named explicitly via --allow-write (defense in depth: opting into
// MCP and naming each write tool are two separate, conscious actions). There is
// intentionally no HTTP transport.
//
// Tool calls are executed in-process and serialized (one at a time), because
// capturing command output requires temporarily redirecting the process stdout.
type MCPCmd struct {
	AllowWrite     []string `help:"Expose these curated write tools by name (e.g. mail_drafts_create, todo_create). Repeatable; default exposes none." name:"allow-write" env:"OLK_MCP_ALLOW_WRITE"`
	MaxOutputBytes int      `help:"Cap a single tool call's output text in bytes (truncated past this; 0 uses the built-in default)." name:"max-output-bytes" env:"OLK_MCP_MAX_OUTPUT_BYTES"`
}

func (c *MCPCmd) Run(ctx *RunContext) error {
	// The MCP server is long-running, so it must not inherit the per-command
	// timeout that Execute applies to ctx.Ctx. Run until interrupted instead.
	runCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	allowWrite := map[string]bool{}
	known := writeToolNames()
	for _, name := range c.AllowWrite {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if !known[name] {
			fmt.Fprintf(os.Stderr, "warning: --allow-write %q is not a curated write tool; ignoring (valid: %s)\n", name, strings.Join(sortedKeys(known), ", "))
			continue
		}
		allowWrite[name] = true
	}

	flags := ctx.Flags
	srv, _, err := buildMCPServer(mcpConfig{
		allowWrite:     allowWrite,
		allowed:        func(path []string) bool { return commandAllowed(flags, path) },
		maxOutputBytes: c.MaxOutputBytes,
	})
	if err != nil {
		return err
	}
	return srv.Run(runCtx, &mcp.StdioTransport{})
}
