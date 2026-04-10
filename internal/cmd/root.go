package cmd

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

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
	Verbose     bool   `help:"Verbose output" short:"v" env:"OLK_VERBOSE"`
	DryRun      bool   `help:"Dry run mode" env:"OLK_DRY_RUN"`
	Force       bool   `help:"Force operation" env:"OLK_FORCE"`
	Color       string `help:"Color mode: auto|never|always" default:"auto" env:"OLK_COLOR" enum:"auto,never,always"`
	Select      string `help:"Comma-separated fields to output" env:"OLK_SELECT"`
	ResultsOnly bool   `help:"Output only the results array (no envelope)" env:"OLK_RESULTS_ONLY"`
	Timeout     int    `help:"Request timeout in seconds" default:"60" env:"OLK_TIMEOUT"`
}

type RunContext struct {
	Ctx    context.Context
	Flags  *RootFlags
	client *graphapi.Client
	store  secrets.Store
	auth   *msauth.Authenticator
	cfg    *config.Config

	storeOnce sync.Once
	storeErr  error
	cfgOnce   sync.Once
	cfgErr    error
}

// Store returns the keyring store, initializing if needed
func (r *RunContext) Store() (secrets.Store, error) {
	r.storeOnce.Do(func() {
		r.store, r.storeErr = secrets.NewKeyringStore()
	})
	if r.storeErr != nil {
		return nil, fmt.Errorf("initializing keyring: %w", r.storeErr)
	}
	return r.store, nil
}

// Config returns the config, loading if needed
func (r *RunContext) Config() (*config.Config, error) {
	r.cfgOnce.Do(func() {
		r.cfg, r.cfgErr = config.Load()
	})
	if r.cfgErr != nil {
		return nil, fmt.Errorf("loading config: %w", r.cfgErr)
	}
	return r.cfg, nil
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

	client, err := graphapi.NewClient(cred)
	if err != nil {
		return nil, fmt.Errorf("creating Graph client: %w", err)
	}

	r.client = client
	return client, nil
}

// Printer returns an output printer based on flags
func (r *RunContext) Printer() *outfmt.Printer {
	return outfmt.NewPrinter(r.Flags.JSON, r.Flags.Plain, r.Flags.ResultsOnly, r.Flags.Select)
}

type CLI struct {
	RootFlags

	Auth     AuthCmd     `cmd:"" help:"Authentication commands"`
	Mail     MailCmd     `cmd:"" help:"Mail commands"`
	Calendar CalendarCmd `cmd:"" help:"Calendar commands"`
	Contacts ContactsCmd `cmd:"" help:"Contacts commands"`
	Todo     TodoCmd     `cmd:"" help:"Microsoft To Do tasks"`
	People   PeopleCmd   `cmd:"" help:"People directory search"`
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
	timeout := cli.RootFlags.Timeout
	if timeout <= 0 {
		timeout = 60
	}
	if timeout > 600 {
		timeout = 600
	}
	var cancel context.CancelFunc
	ctx_bg, cancel = context.WithTimeout(ctx_bg, time.Duration(timeout)*time.Second)
	defer cancel()
	runCtx := &RunContext{
		Ctx:   ctx_bg,
		Flags: &cli.RootFlags,
	}

	err := ctx.Run(runCtx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", outfmt.SanitizeMultiline(err.Error()))
		return 1
	}
	return 0
}
