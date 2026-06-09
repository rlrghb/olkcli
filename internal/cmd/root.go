package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/alecthomas/kong"

	"github.com/rlrghb/olkcli/internal/config"
	"github.com/rlrghb/olkcli/internal/graphapi"
	"github.com/rlrghb/olkcli/internal/msauth"
	"github.com/rlrghb/olkcli/internal/outfmt"
	"github.com/rlrghb/olkcli/internal/secrets"
)

var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

type RootFlags struct {
	JSON        bool   `help:"Output as JSON" env:"OLK_JSON"`
	Plain       bool   `help:"Output as plain TSV" env:"OLK_PLAIN"`
	Account     string `help:"Account email to use" env:"OLK_ACCOUNT"`
	Mailbox     string `help:"Target a different user's mailbox via delegated access (requires Mail.Read.Shared at login)" env:"OLK_MAILBOX"`
	Verbose     bool   `help:"Verbose output" short:"v" env:"OLK_VERBOSE"`
	DryRun      bool   `help:"Dry run mode" env:"OLK_DRY_RUN"`
	Force       bool   `help:"Force operation" env:"OLK_FORCE"`
	Color       string `help:"Color mode: auto|never|always" default:"auto" env:"OLK_COLOR" enum:"auto,never,always"`
	Select      string `help:"Comma-separated fields to output" env:"OLK_SELECT"`
	ResultsOnly bool   `help:"Output only the results array (no envelope)" env:"OLK_RESULTS_ONLY"`
	Timeout     int    `help:"Request timeout in seconds" default:"60" env:"OLK_TIMEOUT"`
	TimeZone    string `help:"IANA time zone for display (e.g. America/New_York, Local, UTC)" name:"tz" env:"OLK_TIMEZONE"`

	// Capability guards (enforced for CLI, MCP, scripts, and --mailbox alike).
	// Named --no-write rather than --read-only because `auth login --read-only`
	// already exists (it requests read-only OAuth scopes).
	NoWrite       bool `help:"Refuse any mutating operation" env:"OLK_NO_WRITE"`
	NoSend        bool `help:"Refuse sending mail or meeting invites" env:"OLK_NO_SEND"`
	NoInput       bool `help:"Fail instead of prompting (headless/agent safety)" env:"OLK_NO_INPUT"`
	WrapUntrusted bool `help:"Wrap external free-text in untrusted-content markers (JSON/plain output)" env:"OLK_WRAP_UNTRUSTED"`

	// Command-scoping (gog-style allow/deny lists, comma-separated dotted paths).
	EnableCommands      string `help:"Allow only these command prefixes (csv; e.g. mail,calendar)" env:"OLK_ENABLE_COMMANDS"`
	EnableCommandsExact string `help:"Allow only these exact command paths (csv; e.g. mail.list,mail.get)" env:"OLK_ENABLE_COMMANDS_EXACT"`
	DisableCommands     string `help:"Block these command paths (csv; overrides allows)" env:"OLK_DISABLE_COMMANDS"`
}

type RunContext struct {
	Ctx    context.Context
	Flags  *RootFlags
	client *graphapi.Client
	store  secrets.Store
	cfg    *config.Config
}

// Store returns the keyring store, initializing if needed
func (r *RunContext) Store() (secrets.Store, error) {
	if r.store != nil {
		return r.store, nil
	}
	store, err := secrets.NewKeyringStore()
	if err != nil {
		return nil, fmt.Errorf("initializing keyring: %w", err)
	}
	r.store = store
	return store, nil
}

// Config returns the config, loading if needed
func (r *RunContext) Config() (*config.Config, error) {
	if r.cfg != nil {
		return r.cfg, nil
	}
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}
	r.cfg = cfg
	return cfg, nil
}

// Authenticator returns the auth manager
func (r *RunContext) Authenticator(clientID, tenantID string) (*msauth.Authenticator, error) {
	store, err := r.Store()
	if err != nil {
		return nil, err
	}
	if clientID == "" {
		clientID = config.DefaultClientID
	}
	if tenantID == "" {
		tenantID = config.DefaultTenantID
	}
	return msauth.NewAuthenticator(store, clientID, tenantID), nil
}

// GraphClient returns the Graph API client for the active account
func (r *RunContext) GraphClient() (*graphapi.Client, error) {
	if r.client != nil {
		return r.client, nil
	}

	store, err := r.Store()
	if err != nil {
		return nil, err
	}

	cfg, err := r.Config()
	if err != nil {
		return nil, err
	}

	// Determine account
	email := r.Flags.Account
	if email == "" {
		email = cfg.GetDefaultAccount()
	}
	if email == "" {
		return nil, fmt.Errorf("no account configured. Run 'olk auth login' first")
	}

	// Get client config for this account
	clientCfg := cfg.GetClient(email)

	auth := msauth.NewAuthenticator(store, clientCfg.ClientID, clientCfg.TenantID)
	cred, err := auth.GetCredential(r.Ctx, email)
	if err != nil {
		return nil, fmt.Errorf("getting credentials: %w", err)
	}

	var client *graphapi.Client
	if r.Flags.Verbose {
		client, err = graphapi.NewClientVerbose(cred)
	} else {
		client, err = graphapi.NewClient(cred)
	}
	if err != nil {
		return nil, fmt.Errorf("creating Graph client: %w", err)
	}

	// Capability guards apply at the client layer, so they cover every command
	// path uniformly (CLI, MCP, scripts, delegated --mailbox).
	client.SetGuards(r.Flags.NoWrite, r.Flags.NoSend)

	r.client = client
	return client, nil
}

// Timezone returns the resolved time.Location for display.
// Precedence: --tz flag > OLK_TIMEZONE env > config file > Local.
func (r *RunContext) Timezone() (*time.Location, error) {
	tz := r.Flags.TimeZone
	if tz == "" {
		if cfg, err := r.Config(); err == nil {
			tz = cfg.GetTimezone()
		}
	}
	if tz == "" {
		tz = "Local"
	}
	return time.LoadLocation(tz)
}

// Printer returns an output printer based on flags
func (r *RunContext) Printer() *outfmt.Printer {
	tzName := "Local"
	if loc, err := r.Timezone(); err == nil {
		tzName = loc.String()
	}
	return outfmt.NewPrinter(r.Flags.JSON, r.Flags.Plain, r.Flags.ResultsOnly, r.Flags.Select, tzName, r.Flags.WrapUntrusted)
}

type CLI struct {
	RootFlags

	Auth     AuthCmd     `cmd:"" help:"Authentication commands"`
	Mail     MailCmd     `cmd:"" help:"Mail commands"`
	Calendar CalendarCmd `cmd:"" help:"Calendar commands"`
	Contacts ContactsCmd `cmd:"" help:"Contacts commands"`
	Todo     TodoCmd     `cmd:"" help:"Microsoft To Do tasks"`
	People   PeopleCmd   `cmd:"" help:"People directory search"`
	Drive    DriveCmd    `cmd:"" help:"OneDrive file operations"`
	Config   ConfigCmd   `cmd:"" help:"Configuration management"`
	MCP      MCPCmd      `cmd:"" name:"mcp" help:"Run an MCP server exposing olk as tools (stdio)"`
	Version  VersionCmd  `cmd:"" help:"Show version information"`
	Whoami   WhoamiCmd   `cmd:"" help:"Show current user profile"`

	// Desire path shortcuts
	Send   SendCmd   `cmd:"" help:"Send an email (shortcut for mail send)" hidden:""`
	Ls     LsCmd     `cmd:"" help:"List inbox (shortcut for mail list)" hidden:""`
	Inbox  InboxCmd  `cmd:"" help:"List inbox (shortcut for mail list)" hidden:""`
	Search SearchCmd `cmd:"" help:"Search mail (shortcut for mail search)" hidden:""`
	Today  TodayCmd  `cmd:"" help:"Today's events (shortcut for calendar events --days 1)" hidden:""`
	Week   WeekCmd   `cmd:"" help:"This week's events (shortcut for calendar events --days 7)" hidden:""`
}

func Execute() int {
	cli := &CLI{}
	ctx := kong.Parse(cli,
		kong.Name("olk"),
		kong.Description("Microsoft Outlook CLI - Access email, calendar, and contacts from the command line"),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact: true,
		}),
		kong.Vars{
			"version": Version,
		},
	)

	ctx_bg := context.Background()
	timeout := cli.Timeout
	if timeout <= 0 {
		timeout = 60
	}
	if timeout > 600 {
		fmt.Fprintf(os.Stderr, "warning: --timeout %d exceeds maximum, clamping to 600s\n", timeout)
		timeout = 600
	}
	var cancel context.CancelFunc
	ctx_bg, cancel = context.WithTimeout(ctx_bg, time.Duration(timeout)*time.Second)
	defer cancel()
	runCtx := &RunContext{
		Ctx:   ctx_bg,
		Flags: &cli.RootFlags,
	}

	// Command allow/deny lists gate dispatch (gog-style). Applies to the bare
	// CLI; the MCP server reuses the same predicate to filter its tool registry.
	if path := selectedCommandPath(ctx); !commandAllowed(&cli.RootFlags, path) {
		fmt.Fprintf(os.Stderr, "Error: command %q is not allowed by --enable-commands/--disable-commands\n", strings.Join(path, " "))
		return 1
	}

	err := ctx.Run(runCtx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", outfmt.SanitizeMultiline(err.Error()))
		return 1
	}
	return 0
}
