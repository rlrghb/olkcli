package cmd

import (
	"reflect"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// toolBinding ties a generated MCP tool name back to the kong command it runs.
type toolBinding struct {
	name string
	path []string
	node *kong.Node
	kind cmdKind
}

// newKongParser builds a kong parser over the full CLI grammar, mirroring the
// options used by Execute so the introspected model matches the real CLI. The
// no-op Exit prevents kong from calling os.Exit on the in-process reparse path.
func newKongParser(cli *CLI) (*kong.Kong, error) {
	return kong.New(cli,
		kong.Name("olk"),
		kong.Description("Microsoft Outlook CLI - Access email, calendar, and contacts from the command line"),
		kong.ConfigureHelp(kong.HelpOptions{Compact: true}),
		kong.Vars{"version": Version},
		kong.Exit(func(int) {}),
	)
}

// buildMCPServer constructs an MCP server exposing one tool per leaf command
// that the given profile permits. It returns the bindings so callers (and
// tests) can inspect the generated tool set.
func buildMCPServer(profile string) (*mcp.Server, []*toolBinding, error) {
	k, err := newKongParser(&CLI{})
	if err != nil {
		return nil, nil, err
	}

	srv := mcp.NewServer(&mcp.Implementation{Name: "olk", Version: Version}, nil)

	var bindings []*toolBinding
	var walk func(n *kong.Node, path []string)
	walk = func(n *kong.Node, path []string) {
		for _, child := range n.Children {
			if child.Type != kong.CommandNode || child.Hidden {
				continue
			}
			cp := append(append([]string{}, path...), child.Name)
			// Never expose the mcp command itself.
			if len(cp) == 1 && cp[0] == "mcp" {
				continue
			}
			if !child.Leaf() {
				walk(child, cp)
				continue
			}
			kind := classify(cp)
			if !includeInProfile(profile, kind) {
				continue
			}
			b := &toolBinding{name: strings.Join(cp, "_"), path: cp, node: child, kind: kind}
			registerTool(srv, b)
			bindings = append(bindings, b)
		}
	}
	walk(k.Model.Node, nil)

	return srv, bindings, nil
}

func registerTool(srv *mcp.Server, b *toolBinding) {
	desc := b.node.Help
	if b.node.Detail != "" {
		desc = strings.TrimSpace(desc + "\n\n" + b.node.Detail)
	}
	tool := &mcp.Tool{
		Name:        b.name,
		Description: desc,
		InputSchema: flagSchema(b.node),
		Annotations: annotationsFor(b.kind),
	}
	srv.AddTool(tool, makeHandler(b))
}

func annotationsFor(kind cmdKind) *mcp.ToolAnnotations {
	switch kind {
	case kindRead:
		return &mcp.ToolAnnotations{ReadOnlyHint: true}
	case kindDestructive:
		t := true
		return &mcp.ToolAnnotations{DestructiveHint: &t}
	default:
		f := false
		return &mcp.ToolAnnotations{DestructiveHint: &f}
	}
}

// flagSchema builds the JSON Schema for a leaf command's flags and positionals.
func flagSchema(node *kong.Node) *jsonschema.Schema {
	s := &jsonschema.Schema{
		Type:       "object",
		Properties: map[string]*jsonschema.Schema{},
	}
	var required []string

	for _, f := range node.Flags {
		if f.Hidden || f.Name == "help" {
			continue
		}
		s.Properties[f.Name] = valueSchema(f.Value)
		if f.Required {
			required = append(required, f.Name)
		}
	}
	for _, p := range node.Positional {
		s.Properties[p.Name] = valueSchema(p)
		if p.Required {
			required = append(required, p.Name)
		}
	}
	s.Required = required
	return s
}

func valueSchema(v *kong.Value) *jsonschema.Schema {
	sch := &jsonschema.Schema{Description: v.Help}
	switch {
	case v.IsBool():
		sch.Type = "boolean"
	case v.IsSlice():
		sch.Type = "array"
		sch.Items = &jsonschema.Schema{Type: "string"}
	default:
		switch v.Target.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			sch.Type = "integer"
		case reflect.Float32, reflect.Float64:
			sch.Type = "number"
		default:
			sch.Type = "string"
		}
	}

	if v.Enum != "" {
		for _, e := range v.EnumSlice() {
			if e == "" {
				continue
			}
			sch.Enum = append(sch.Enum, e)
		}
	}
	return sch
}
