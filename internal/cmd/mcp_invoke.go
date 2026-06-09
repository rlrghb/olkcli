package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/alecthomas/kong"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// makeHandler returns the MCP tool handler for a single command binding. Each
// call reconstructs an argv, re-parses it through a fresh kong instance, runs
// the matched command against a per-call RunContext, and returns whatever the
// command wrote to stdout/stderr.
func makeHandler(b *toolBinding) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args map[string]any
		if req.Params != nil && len(req.Params.Arguments) > 0 {
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return errorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
			}
		}

		// Reject arguments the tool doesn't declare (fixed-schema contract) so a
		// model can't smuggle in unvetted flags.
		if err := rejectUnknownArgs(b, args); err != nil {
			return errorResult(err.Error()), nil
		}

		argv, err := buildArgv(b, args)
		if err != nil {
			return errorResult(err.Error()), nil
		}

		cli := &CLI{}
		k, err := newKongParser(cli)
		if err != nil {
			return nil, fmt.Errorf("building parser: %w", err)
		}
		kctx, err := k.Parse(argv)
		if err != nil {
			return errorResult(fmt.Sprintf("parse error: %v", err)), nil
		}

		timeout := cli.Timeout
		if timeout <= 0 {
			timeout = 60
		}
		if timeout > 600 {
			timeout = 600
		}
		callCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		defer cancel()

		// Force agent-safe output regardless of what the reparsed argv set:
		// never block on a prompt, and wrap externally-controlled free text so
		// the model treats it as data, not instructions.
		cli.NoInput = true
		cli.WrapUntrusted = true

		runCtx := &RunContext{Ctx: callCtx, Flags: &cli.RootFlags}

		stdout, stderr, runErr := captureStd(func() error {
			return kctx.Run(runCtx)
		})

		if runErr != nil {
			parts := []string{}
			if s := strings.TrimSpace(stdout); s != "" {
				parts = append(parts, s)
			}
			if s := strings.TrimSpace(stderr); s != "" {
				parts = append(parts, s)
			}
			parts = append(parts, "error: "+runErr.Error())
			return errorResult(strings.Join(parts, "\n")), nil
		}

		text := stdout
		if s := strings.TrimSpace(stderr); s != "" {
			text = strings.TrimRight(stdout, "\n") + "\n[stderr]\n" + s
		}
		text = capText(text, b.maxOutputBytes)
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: text}},
		}, nil
	}
}

// rejectUnknownArgs fails the call if args carries a key the tool's schema does
// not declare (fixed-schema contract).
func rejectUnknownArgs(b *toolBinding, args map[string]any) error {
	if len(args) == 0 {
		return nil
	}
	known := map[string]bool{}
	for _, f := range b.node.Flags {
		if f.Hidden || f.Name == helpFlagName {
			continue
		}
		known[f.Name] = true
	}
	for _, p := range b.node.Positional {
		known[p.Name] = true
	}
	if b.readOnly() {
		known[conciseArg] = true // synthetic flag injected into read-tool schemas
	}
	for k := range args {
		if !known[k] {
			return fmt.Errorf("unknown argument %q for tool %q", k, b.name)
		}
	}
	return nil
}

// capText truncates s to at most limit bytes (on a UTF-8 boundary), appending a
// notice so the agent knows output was clipped rather than silently complete.
func capText(s string, limit int) string {
	if limit <= 0 {
		limit = defaultMaxOutputBytes
	}
	if len(s) <= limit {
		return s
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + fmt.Sprintf("\n[output truncated at %d bytes; refine the query or raise --max-output-bytes]", limit)
}

// errorResult builds an IsError tool result as a {code, message, action} JSON
// envelope so an agent gets a machine classification plus an actionable next
// step, not just a raw Graph error.
func errorResult(msg string) *mcp.CallToolResult {
	code, action := classifyError(msg)
	env := map[string]string{"code": code, "message": msg}
	if action != "" {
		env["action"] = action
	}
	body, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		body = []byte(msg)
	}
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: string(body)}},
	}
}

// classifyError maps a free-text error into a stable code and a recovery action.
// Codes mirror common Graph failure modes (auth/scope/not-found/throttling) so
// agents can branch on `code` instead of pattern-matching prose.
func classifyError(msg string) (code, action string) {
	switch {
	case strings.Contains(msg, "no account configured"),
		strings.Contains(msg, "no account"),
		strings.Contains(msg, "InvalidAuthenticationToken"),
		strings.Contains(msg, "401"):
		return "unauthenticated", "run `olk auth login` first (or re-run it if the token expired)"
	case strings.Contains(msg, "Read.Shared"),
		strings.Contains(msg, "InsufficientScope"),
		strings.Contains(msg, "ErrorAccessDenied"),
		strings.Contains(msg, "Forbidden"),
		strings.Contains(msg, "403"):
		return "forbidden", "the signed-in token may lack a required scope; re-run `olk auth login --scope <Scope>` (add --enterprise for work/school-only scopes)"
	case strings.Contains(msg, "ErrorItemNotFound"),
		strings.Contains(msg, "ResourceNotFound"),
		strings.Contains(msg, "404"):
		return "not_found", "the id may be stale; re-list to get a current id"
	case strings.Contains(msg, "TooManyRequests"),
		strings.Contains(msg, "throttl"),
		strings.Contains(msg, "429"):
		return "rate_limited", "back off and retry after a short delay"
	case strings.Contains(msg, "unknown argument"),
		strings.Contains(msg, "invalid arguments"),
		strings.Contains(msg, "parse error"),
		strings.Contains(msg, "missing required argument"):
		return "invalid_input", "check the tool's input schema and retry with valid arguments"
	}
	return "error", ""
}

// buildArgv reconstructs a CLI argv from a tool's arguments. Ordering is
// [path..., --json, flags..., --, positionals...]: --json and all option flags
// precede the literal "--" so kong never treats them as positionals.
func buildArgv(b *toolBinding, args map[string]any) ([]string, error) {
	argv := append([]string{}, b.path...)

	for _, f := range b.node.Flags {
		if f.Hidden || f.Name == helpFlagName {
			continue
		}
		v, ok := args[f.Name]
		if !ok {
			continue
		}
		toks, err := flagTokens(f, v)
		if err != nil {
			return nil, err
		}
		argv = append(argv, toks...)
	}

	// Honor the synthetic concise flag injected into read-tool schemas: it maps
	// to olk's global --concise rather than a per-command flag.
	if b.readOnly() {
		if on, _ := asBool(args[conciseArg]); on {
			argv = append(argv, "--concise")
		}
	}

	// Destructive tools require --force at the CLI to confirm; under MCP, naming
	// the tool via --allow-destructive is itself the deliberate confirmation, so
	// supply --force here (the --no-write capability guard still vetoes at the API
	// layer regardless).
	if b.tier == tierDestructive {
		argv = append(argv, "--force")
	}

	// Force structured output.
	argv = append(argv, "--json")

	pos := make([]string, 0, len(b.node.Positional))
	for _, p := range b.node.Positional {
		v, ok := args[p.Name]
		if !ok {
			if p.Required {
				return nil, fmt.Errorf("missing required argument %q", p.Name)
			}
			continue
		}
		pos = append(pos, sprintArg(v))
	}
	if len(pos) > 0 {
		argv = append(argv, "--")
		argv = append(argv, pos...)
	}

	return argv, nil
}

func flagTokens(f *kong.Flag, v any) ([]string, error) {
	name := "--" + f.Name
	switch {
	case f.IsBool():
		on, err := asBool(v)
		if err != nil {
			return nil, fmt.Errorf("flag %q: %w", f.Name, err)
		}
		if on {
			return []string{name}, nil
		}
		return nil, nil
	case f.IsSlice():
		arr, ok := v.([]any)
		if !ok {
			return nil, fmt.Errorf("flag %q expects an array", f.Name)
		}
		toks := make([]string, 0, len(arr)*2)
		for _, e := range arr {
			toks = append(toks, name, sprintArg(e))
		}
		return toks, nil
	default:
		return []string{name, sprintArg(v)}, nil
	}
}

func asBool(v any) (bool, error) {
	switch b := v.(type) {
	case bool:
		return b, nil
	case string:
		return b == "true", nil
	default:
		return false, fmt.Errorf("expects a boolean")
	}
}

// sprintArg renders a JSON-decoded argument value as a CLI token. JSON numbers
// decode to float64; integral values are emitted without a decimal point or
// exponent so flags like --top receive "25", not "25" via "2.5e+01".
func sprintArg(v any) string {
	switch n := v.(type) {
	case float64:
		if n == float64(int64(n)) {
			return strconv.FormatInt(int64(n), 10)
		}
		return strconv.FormatFloat(n, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(n)
	case string:
		return n
	default:
		return fmt.Sprint(v)
	}
}
