package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

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
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: text}},
		}, nil
	}
}

func errorResult(msg string) *mcp.CallToolResult {
	if h := hintFor(msg); h != "" {
		msg = msg + "\nhint: " + h
	}
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: msg}},
	}
}

// hintFor returns a short recovery action for common failure modes, so an agent
// gets an actionable next step rather than a bare Graph error (a lightweight
// take on outlook-mcp's {code,message,action} envelopes).
func hintFor(msg string) string {
	switch {
	case strings.Contains(msg, "no account configured"):
		return "run `olk auth login` first"
	case strings.Contains(msg, "Read.Shared"),
		strings.Contains(msg, "InsufficientScope"),
		strings.Contains(msg, "ErrorAccessDenied"),
		strings.Contains(msg, "Forbidden"):
		return "the signed-in token may lack a required scope; re-run `olk auth login --scope <Scope>`"
	}
	return ""
}

// buildArgv reconstructs a CLI argv from a tool's arguments. Ordering is
// [path..., --json, flags..., --, positionals...]: --json and all option flags
// precede the literal "--" so kong never treats them as positionals.
func buildArgv(b *toolBinding, args map[string]any) ([]string, error) {
	argv := append([]string{}, b.path...)

	for _, f := range b.node.Flags {
		if f.Hidden || f.Name == "help" {
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
