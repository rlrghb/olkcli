package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rlrghb/olkcli/internal/graphapi"
)

// MCPCmd runs a stdio MCP server exposing a curated, read-first set of olk
// commands as typed tools — there is no arbitrary command bridge. The server is
// read-only by default; write tools are exposed only when named explicitly via
// --allow-write (defense in depth: opting into MCP and naming each write tool
// are two separate, conscious actions). There is intentionally no HTTP transport.
//
// Tool calls are executed in-process and serialized (one at a time), because
// capturing command output requires temporarily redirecting the process stdout.
type MCPCmd struct {
	AllowWrite       []string `help:"Expose these curated safe-write tools by name (e.g. mail_flag, todo_update). Repeatable; default exposes none." name:"allow-write" env:"OLK_MCP_ALLOW_WRITE"`
	AllowSend        []string `help:"Expose these curated SEND tools by name (mail_send, mail_reply, mail_forward, calendar_respond, …). Off by default; each transmits to other people and is vetoed by --no-send." name:"allow-send" env:"OLK_MCP_ALLOW_SEND"`
	AllowDestructive []string `help:"Expose these curated DESTRUCTIVE tools by name (mail_delete, calendar_delete, …). Off by default; each hard-deletes and is vetoed by --no-write." name:"allow-destructive" env:"OLK_MCP_ALLOW_DESTRUCTIVE"`
	AllowTool        []string `help:"Restrict exposed tools to these selectors: exact name (mail_list), prefix glob (mail_*, mail.*), or category (read, write, all). Repeatable/csv; default exposes all curated tools." name:"allow-tool" env:"OLK_MCP_ALLOW_TOOL"`
	MaxOutputBytes   int      `help:"Cap a single tool call's output text in bytes (truncated past this; 0 uses the built-in default)." name:"max-output-bytes" env:"OLK_MCP_MAX_OUTPUT_BYTES"`
	AllowMailbox     []string `help:"Permit agents to direct calls at these delegated mailboxes by name. Repeatable/csv; default permits none, in which case tools act only on the mailbox given by --mailbox at launch." name:"allow-mailbox" env:"OLK_MCP_ALLOW_MAILBOX"`
}

// resolveAllowMailbox validates the operator's permitted mailboxes. A malformed
// address is refused outright rather than dropped with a warning: unlike a
// mistyped tool name, which simply exposes nothing, a mistyped mailbox would
// leave agents believing a mailbox is reachable when no call can ever name it.
func resolveAllowMailbox(values []string) ([]string, error) {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, v := range values {
		for _, addr := range strings.Split(v, ",") {
			addr = strings.TrimSpace(addr)
			if addr == "" {
				continue
			}
			if err := graphapi.ValidateEmail(addr); err != nil {
				return nil, fmt.Errorf("invalid --allow-mailbox value %q: %w", addr, err)
			}
			if key := strings.ToLower(addr); !seen[key] {
				seen[key] = true
				out = append(out, addr)
			}
		}
	}
	return out, nil
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

// launchEnv captures the operator's command line for the life of the server.
//
// Kong re-reads OLK_* variables on every parse but nothing else survives, so a
// flag missing from here is a flag the operator set and every tool call then
// ignored. That is how --dry-run came to be dropped by a server started with it.
// The one test that can catch such an omission has to start from RootFlags, so
// this is a function rather than a literal inside Run.
func launchEnv(flags *RootFlags, mailbox string, allowMailbox []string) callEnv {
	return callEnv{
		mailbox:      mailbox,
		account:      flags.Account,
		timeout:      flags.Timeout,
		noWrite:      flags.NoWrite,
		noSend:       flags.NoSend,
		verbose:      flags.Verbose,
		dryRun:       flags.DryRun,
		concise:      flags.Concise,
		resultsOnly:  flags.ResultsOnly,
		immutableIDs: flags.ImmutableIDs,
		timezone:     flags.TimeZone,
		selectFields: flags.Select,
		allowMailbox: allowMailbox,
	}
}

func (c *MCPCmd) Run(ctx *RunContext) error {
	// The MCP server is long-running, so it must not inherit the per-command
	// timeout that Execute applies to ctx.Ctx. Run until interrupted instead.
	runCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	flags := ctx.Flags
	allowMailbox, err := resolveAllowMailbox(c.AllowMailbox)
	if err != nil {
		return err
	}
	mailbox, err := resolveMailboxTarget(flags.Mailbox)
	if err != nil {
		return err
	}
	srv, _, err := buildMCPServer(&mcpConfig{
		allowWrite:       resolveAllowList(c.AllowWrite, writeToolNames(), "--allow-write"),
		allowSend:        resolveAllowList(c.AllowSend, sendToolNames(), "--allow-send"),
		allowDestructive: resolveAllowList(c.AllowDestructive, destructiveToolNames(), "--allow-destructive"),
		noWrite:          flags.NoWrite,
		noSend:           flags.NoSend,
		allowed:          func(path []string) bool { return commandAllowed(flags, path) },
		allowTool:        toolSelectorPredicate(c.AllowTool),
		maxOutputBytes:   c.MaxOutputBytes,
		env:              launchEnv(flags, mailbox, allowMailbox),
	})
	if err != nil {
		return err
	}
	return srv.Run(runCtx, &mcp.StdioTransport{})
}
