package cmd

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPCmd runs a stdio MCP server exposing a curated, read-first set of olk
// commands as tools (mirroring gog's "typed tools, no arbitrary command
// bridge" model). The server is read-only by default; --allow-write additionally
// exposes the curated safe-write tools, which must still be named via
// --enable-commands-exact. There is intentionally no HTTP transport.
//
// Tool calls are executed in-process and serialized (one at a time), because
// capturing command output requires temporarily redirecting the process stdout.
type MCPCmd struct {
	AllowWrite bool `help:"Also expose curated write tools (each must still be named via --enable-commands-exact)" env:"OLK_MCP_ALLOW_WRITE"`
}

func (c *MCPCmd) Run(ctx *RunContext) error {
	// The MCP server is long-running, so it must not inherit the per-command
	// timeout that Execute applies to ctx.Ctx. Run until interrupted instead.
	runCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	flags := ctx.Flags
	srv, _, err := buildMCPServer(mcpConfig{
		allowWrite: c.AllowWrite,
		allowed:    func(path []string) bool { return commandAllowed(flags, path) },
	})
	if err != nil {
		return err
	}
	return srv.Run(runCtx, &mcp.StdioTransport{})
}
