# olkcli — Microsoft Outlook in Your Terminal

A fast, scriptable CLI for Microsoft Outlook via the Microsoft Graph API. Manage email, calendar, and contacts from the command line.

Works with both **personal Microsoft accounts** and **enterprise (Azure AD / Entra ID)** accounts. Zero-config setup with device-code authentication — just run `olkcli auth login` and go.

## Key Capabilities

### Mail
- **List & read** inbox messages with filtering by sender, date, read status
- **Send** emails with To/CC/BCC, HTML bodies, stdin piping
- **Search** using KQL (Keyword Query Language) syntax
- **Reply, reply-all, forward** messages
- **Move** messages between folders
- **Delete** and **mark read/unread**
- **List folders** with message counts
- **View attachments**

### Calendar
- **List events** with configurable date ranges (default: 7 days ahead)
- **Create events** with location, attendees, all-day, and online meeting support
- **Update and delete** events
- **Respond** to invitations (accept, decline, tentative)
- **List calendars** across your account

### Contacts
- **List, search, create, update, delete** contacts
- Fields: name, email, phone, company, job title

## Installation

### From Source

```bash
git clone https://github.com/rlrghb/olkcli.git
cd olkcli
make build
# Binary is at ./bin/olkcli
```

### Go Install

```bash
go install github.com/rlrghb/olkcli/cmd/olkcli@latest
```

### Homebrew (coming soon)

```bash
brew install rlrghb/tap/olkcli
```

## Quick Start

```bash
# Authenticate (opens browser for device-code flow)
olkcli auth login

# List recent inbox messages
olkcli mail list

# Read a specific message
olkcli mail get <message-id>

# Send an email
olkcli mail send --to user@example.com --subject "Hello" --body "Hi there"

# Pipe body from stdin
echo "Hello from the CLI" | olkcli mail send --to user@example.com --subject "Piped"

# Search mail
olkcli mail search "from:boss@company.com subject:urgent"

# View this week's calendar
olkcli calendar events

# Today's events
olkcli today

# Create a meeting
olkcli calendar create --subject "Standup" --start 2024-01-15T09:00 --end 2024-01-15T09:30 --attendees colleague@company.com

# List contacts
olkcli contacts list
```

## Output Formats

| Flag | Format | Use Case |
|------|--------|----------|
| *(default)* | Aligned table | Human reading |
| `--json` | JSON envelope | Scripting with `jq` |
| `--plain` | Tab-separated | Piping to `awk`, `cut` |

### JSON Envelope

```bash
olkcli mail list --json
```

```json
{
  "results": [...],
  "count": 25,
  "nextLink": ""
}
```

Use `--results-only` to get just the array:

```bash
olkcli mail list --json --results-only | jq '.[0].subject'
```

### Field Selection

```bash
olkcli mail list --select from,subject
```

## Authentication

### Default (Zero Config)

```bash
olkcli auth login
```

Uses an embedded public client ID with device-code flow. Works for both personal and enterprise accounts.

### Custom App Registration

For enterprise environments with their own Azure app registration:

```bash
olkcli auth login --client-id YOUR_CLIENT_ID --tenant-id YOUR_TENANT_ID
```

Or via environment variables:

```bash
export OLK_CLIENT_ID=your-client-id
export OLK_TENANT_ID=your-tenant-id
olkcli auth login
```

### Multi-Account

```bash
# Login to a second account
olkcli auth login

# List accounts
olkcli auth list

# Use a specific account
olkcli mail list --account user2@example.com

# Check auth status
olkcli auth status
```

### Token Storage

Refresh tokens are stored in the OS credential manager:
- **macOS**: Keychain
- **Linux**: Secret Service (GNOME Keyring / KDE Wallet)
- **Windows**: Windows Credential Manager

## Shortcuts

For common workflows, `olkcli` provides top-level shortcuts:

| Shortcut | Expands To |
|----------|-----------|
| `olkcli send` | `olkcli mail send` |
| `olkcli ls` | `olkcli mail list` |
| `olkcli inbox` | `olkcli mail list` |
| `olkcli search <q>` | `olkcli mail search <q>` |
| `olkcli today` | `olkcli calendar events --days 1` |
| `olkcli week` | `olkcli calendar events --days 7` |

## Global Flags

| Flag | Env Var | Description |
|------|---------|-------------|
| `--json` | `OLK_JSON` | JSON output |
| `--plain` | `OLK_PLAIN` | TSV output |
| `--account EMAIL` | `OLK_ACCOUNT` | Account to use |
| `-v, --verbose` | `OLK_VERBOSE` | Verbose output |
| `--dry-run` | `OLK_DRY_RUN` | Dry run mode |
| `--force` | `OLK_FORCE` | Skip confirmations |
| `--color auto\|never\|always` | `OLK_COLOR` | Color mode |
| `--select FIELDS` | `OLK_SELECT` | Field projection |
| `--results-only` | `OLK_RESULTS_ONLY` | Unwrap JSON envelope |

## Commands Reference

### Auth

```
olkcli auth login [--client-id ID] [--tenant-id ID]    Login via device code
olkcli auth logout [EMAIL]                              Remove stored credentials
olkcli auth list                                        List authenticated accounts
olkcli auth status                                      Check token validity
```

### Mail

```
olkcli mail list [-n 25] [-f FOLDER] [-u] [--from X] [--after DATE] [--before DATE]
olkcli mail get <ID> [--format full|text|html]
olkcli mail send --to X --subject Y [--body Z] [--cc X] [--bcc X] [--html]
olkcli mail search <QUERY> [-n 25]
olkcli mail reply <ID> --body X [--reply-all]
olkcli mail forward <ID> --to X [--comment Y]
olkcli mail move <ID> <FOLDER>
olkcli mail delete <ID> [--force]
olkcli mail mark <ID> --read|--unread
olkcli mail folders
olkcli mail attachments <ID>
```

### Calendar

```
olkcli calendar events [-d 7] [--after DATE] [--before DATE] [--calendar ID] [-n 25]
olkcli calendar get <ID>
olkcli calendar create --subject X --start Y --end Z [--location L] [--attendees A] [--all-day] [--online-meeting]
olkcli calendar update <ID> [--subject X] [--start Y] [--end Z] [--location L]
olkcli calendar delete <ID> [--force]
olkcli calendar respond <ID> accept|decline|tentative
olkcli calendar calendars
```

### Contacts

```
olkcli contacts list [-n 25] [--folder ID]
olkcli contacts get <ID>
olkcli contacts create --first-name X --last-name Y [--email Z] [--phone P] [--company C] [--title T]
olkcli contacts update <ID> [--first-name X] [--last-name Y] [--email Z] [--phone P] [--company C] [--title T]
olkcli contacts delete <ID> [--force]
olkcli contacts search <QUERY> [-n 25]
```

## Configuration

Config is stored at `~/.config/olkcli/`:

```
~/.config/olkcli/
├── config.json          # Default account, client IDs
└── accounts/            # Account metadata (email, display name)
    └── user@example.com.json
```

Override the config directory with `OLK_CONFIG_DIR`.

## Scripting Examples

```bash
# Count unread messages
olkcli mail list --unread --json --results-only | jq length

# Get subjects of today's events
olkcli today --json --results-only | jq -r '.[].subject'

# Export contacts as CSV
olkcli contacts list --plain --select name,email

# Send from a script
olkcli send --to ops@company.com --subject "Deploy complete" --body "$(date): v1.2.3 deployed"

# Process inbox with jq
olkcli mail list --json --results-only | jq -r '.[] | select(.isRead == false) | "\(.from): \(.subject)"'
```

## Architecture

```
olkcli/
├── cmd/olkcli/main.go           # Entry point
├── internal/
│   ├── cmd/                     # CLI commands (kong)
│   ├── msauth/                  # Microsoft OAuth2 (device code, token refresh)
│   ├── graphapi/                # Microsoft Graph API wrapper
│   ├── config/                  # Configuration management
│   ├── secrets/                 # OS keyring integration
│   ├── outfmt/                  # Output formatting (JSON/table/TSV)
│   └── errfmt/                  # Error formatting
├── Makefile
├── .goreleaser.yaml
└── go.mod
```

## Development

```bash
make build      # Build to ./bin/olkcli
make test       # Run tests
make lint       # Run golangci-lint
make install    # Install to $GOPATH/bin
make clean      # Remove build artifacts
```

## License

MIT License. See [LICENSE](LICENSE) for details.
