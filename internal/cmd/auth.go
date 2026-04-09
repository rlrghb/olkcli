package cmd

import (
	"fmt"
	"strings"

	"github.com/rlrghb/olkcli/internal/config"
	"github.com/rlrghb/olkcli/internal/msauth"
	"github.com/rlrghb/olkcli/internal/outfmt"
)

type AuthCmd struct {
	Login  AuthLoginCmd  `cmd:"" help:"Login to a Microsoft account"`
	Logout AuthLogoutCmd `cmd:"" help:"Logout from an account"`
	List   AuthListCmd   `cmd:"" help:"List authenticated accounts"`
	Status AuthStatusCmd `cmd:"" help:"Show authentication status"`
}

type AuthLoginCmd struct {
	ClientID string `help:"OAuth2 client ID" env:"OLK_CLIENT_ID"`
	TenantID string `help:"Azure AD tenant ID" env:"OLK_TENANT_ID" default:"common"`
	ReadOnly bool   `help:"Request read-only permissions"`
}

func (c *AuthLoginCmd) Run(ctx *RunContext) error {
	clientID := c.ClientID
	if clientID == "" {
		clientID = config.DefaultClientID
	}

	auth, err := ctx.Authenticator(clientID, c.TenantID)
	if err != nil {
		return err
	}

	scopes := msauth.DefaultScopes()
	if c.ReadOnly {
		scopes = msauth.ReadOnlyScopes()
	}
	info, err := auth.LoginDeviceCode(ctx.Ctx, scopes)
	if err != nil {
		return err
	}

	// Save client config for this account
	cfg, err := ctx.Config()
	if err != nil {
		return err
	}

	cfg.SetClient(info.Email, config.Client{
		ClientID: clientID,
		TenantID: c.TenantID,
	})

	// Set as default if no default exists
	if cfg.GetDefaultAccount() == "" {
		cfg.SetDefaultAccount(info.Email)
	}

	if err := cfg.Save(); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Printf("Logged in as %s (%s)\n", outfmt.Sanitize(info.DisplayName), outfmt.Sanitize(info.Email))
	return nil
}

type AuthLogoutCmd struct {
	Email string `arg:"" optional:"" help:"Account email to logout (default: current account)"`
}

func (c *AuthLogoutCmd) Run(ctx *RunContext) error {
	cfg, err := ctx.Config()
	if err != nil {
		return err
	}

	email := c.Email
	if email == "" {
		email = ctx.Flags.Account
	}
	if email == "" {
		email = cfg.GetDefaultAccount()
	}
	if email == "" {
		return fmt.Errorf("no account specified. Use --account or pass an email argument")
	}

	clientCfg := cfg.GetClient(email)
	auth, err := ctx.Authenticator(clientCfg.ClientID, clientCfg.TenantID)
	if err != nil {
		return err
	}

	if err := auth.Logout(email); err != nil {
		return err
	}

	cfg.RemoveAccount(email)
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Printf("Logged out %s\n", outfmt.Sanitize(email))
	return nil
}

type AuthListCmd struct{}

func (c *AuthListCmd) Run(ctx *RunContext) error {
	cfg, err := ctx.Config()
	if err != nil {
		return err
	}

	// Use a temporary authenticator just to list accounts
	auth, err := ctx.Authenticator("", "")
	if err != nil {
		return err
	}

	accounts, err := auth.ListAccounts()
	if err != nil {
		return err
	}

	if len(accounts) == 0 {
		fmt.Println("No accounts configured. Run 'olkcli auth login' to get started.")
		return nil
	}

	defaultAccount := cfg.GetDefaultAccount()
	printer := ctx.Printer()

	if ctx.Flags.JSON {
		return printer.PrintJSON(accounts, len(accounts), "")
	}

	headers := []string{"EMAIL", "NAME", "DEFAULT"}
	var rows [][]string
	for _, a := range accounts {
		def := ""
		if strings.EqualFold(a.Email, defaultAccount) {
			def = "*"
		}
		rows = append(rows, []string{a.Email, a.DisplayName, def})
	}

	return printer.PrintTable(headers, rows)
}

type AuthStatusCmd struct{}

func (c *AuthStatusCmd) Run(ctx *RunContext) error {
	cfg, err := ctx.Config()
	if err != nil {
		return err
	}

	email := ctx.Flags.Account
	if email == "" {
		email = cfg.GetDefaultAccount()
	}
	if email == "" {
		fmt.Println("No account configured. Run 'olkcli auth login' to get started.")
		return nil
	}

	// Try to get a credential to verify token is valid
	_, err = ctx.GraphClient()
	if err != nil {
		fmt.Printf("Account: %s\nStatus: Invalid (token expired or revoked)\n", outfmt.Sanitize(email))
		fmt.Println("Run 'olkcli auth login' to re-authenticate.")
		return nil
	}

	fmt.Printf("Account: %s\nStatus: Authenticated\n", outfmt.Sanitize(email))
	return nil
}
