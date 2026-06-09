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
	AllowWrite       []string `help:"Expose these curated safe-write tools by name (e.g. mail_flag, todo_update). Repeatable; default exposes none." name:"allow-write" env:"OLK_MCP_ALLOW_WRITE"`
	AllowSend        []string `help:"Expose these curated SEND tools by name (mail_send, mail_reply, mail_forward, calendar_respond, …). Off by default; each transmits to other people and is vetoed by --no-send." name:"allow-send" env:"OLK_MCP_ALLOW_SEND"`
	AllowDestructive []string `help:"Expose these curated DESTRUCTIVE tools by name (mail_delete, calendar_delete, …). Off by default; each hard-deletes and is vetoed by --no-write." name:"allow-destructive" env:"OLK_MCP_ALLOW_DESTRUCTIVE"`
	AllowTool        []string `help:"Restrict exposed tools to these selectors: exact name (mail_list), prefix glob (mail_*, mail.*), or category (read, write, all). Repeatable/csv; default exposes all curated tools." name:"allow-tool" env:"OLK_MCP_ALLOW_TOOL"`
	MaxOutputBytes   int      `help:"Cap a single tool call's output text in bytes (truncated past this; 0 uses the built-in default)." name:"max-output-bytes" env:"OLK_MCP_MAX_OUTPUT_BYTES"`
}

// resolveAllowList validates the named tools against the set eligible for a
// flag, warning (to stderr, not stdout) about any that aren't and dropping them.
func resolveAllowList(names []string, eligible map[string]bool, flagName string) map[string]bool {
	out := map[string]bool{}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if !eligible[name] {
			fmt.Fprintf(os.Stderr, "warning: %s %q is not a curated %s tool; ignoring (valid: %s)\n",
				flagName, name, flagName, strings.Join(sortedKeys(eligible), ", "))
			continue
		}
		out[name] = true
	}
	return out
}

func (c *MCPCmd) Run(ctx *RunContext) error {
	// The MCP server is long-running, so it must not inherit the per-command
	// timeout that Execute applies to ctx.Ctx. Run until interrupted instead.
	runCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	flags := ctx.Flags
	srv, _, err := buildMCPServer(mcpConfig{
		allowWrite:       resolveAllowList(c.AllowWrite, writeToolNames(), "--allow-write"),
		allowSend:        resolveAllowList(c.AllowSend, sendToolNames(), "--allow-send"),
		allowDestructive: resolveAllowList(c.AllowDestructive, destructiveToolNames(), "--allow-destructive"),
		noWrite:          flags.NoWrite,
		noSend:           flags.NoSend,
		allowed:          func(path []string) bool { return commandAllowed(flags, path) },
		allowTool:        toolSelectorPredicate(c.AllowTool),
		maxOutputBytes:   c.MaxOutputBytes,
	})
	if err != nil {
		return err
	}
	return srv.Run(runCtx, &mcp.StdioTransport{})
}
