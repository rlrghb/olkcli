# CLAUDE.md

This file provides context for Claude Code when working on the olkcli project.

## What is this project?

`olkcli` is a CLI tool for Microsoft Outlook via the Microsoft Graph API. It provides terminal access to email, calendar, and contacts for both personal Microsoft accounts and enterprise Azure AD/Entra ID accounts.

## Quick Reference

```bash
make build          # Build binary to ./bin/olkcli
make test           # Run tests
make lint           # Lint with golangci-lint
go mod tidy         # After changing dependencies
```

## Architecture

- **CLI framework**: `github.com/alecthomas/kong` — commands are Go structs with `Run(ctx *RunContext) error`
- **Auth**: Raw OAuth2 device code flow against `login.microsoftonline.com` — no MSAL
- **API**: Official `msgraph-sdk-go` wrapped in `internal/graphapi/` for ergonomic access
- **Secrets**: OS keyring via `github.com/99designs/keyring` (macOS Keychain, Linux Secret Service, Windows WinCred)
- **Output**: JSON envelope (`--json`), aligned table (default), TSV (`--plain`)

## Key Patterns

- `RunContext` (in `internal/cmd/root.go`) lazily initializes the Graph client — auth commands skip it
- Graph SDK uses pointer types everywhere — always nil-check: `if x.GetFoo() != nil { *x.GetFoo() }`
- Each command is in its own file: `mail_list.go`, `mail_get.go`, etc.
- Desire paths in `desire_paths.go` delegate to real commands (e.g. `SendCmd` creates `MailSendCmd`)
- Config lives at `~/.config/olkcli/`, tokens in OS keyring keyed by `olkcli:token:<email>`

## Common Tasks

### Adding a new mail subcommand
1. Create `internal/cmd/mail_<name>.go` with the command struct and `Run` method
2. Add the struct to `MailCmd` in `internal/cmd/mail.go`
3. If needed, add the API method to `internal/graphapi/mail.go`

### Adding a new flag to all commands
Add it to `RootFlags` in `internal/cmd/root.go` with `env:"OLK_*"` tag.

### Changing Graph API calls
Edit files in `internal/graphapi/` — these wrap the verbose SDK calls into simple methods returning plain structs.

## Dependencies

The project uses `msgraph-sdk-go` v1.96.0 which has some naming quirks:
- Attendee type uses `SetTypeEscaped()` not `SetType()` (Go keyword collision)
- Contact emails use `models.NewEmailAddress()` not `NewTypedEmailAddress()`
- Contact phones: `GetBusinessPhones()`, `GetHomePhones()`, `GetMobilePhone()` (no unified `GetPhones()`)
- Message item request builders: `ItemMessagesMessageItemRequestBuilder*` (note double "Messages")
