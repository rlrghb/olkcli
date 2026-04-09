# olk — Microsoft Outlook in Your Terminal

A fast, scriptable CLI for Microsoft Outlook via the Microsoft Graph API. Manage email, calendar, and contacts from the command line.

Works with both **personal Microsoft accounts** and **enterprise (Azure AD / Entra ID)** accounts. Zero-config setup with device-code authentication — just run `olk auth login` and go.

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
# Binary is at ./bin/olk
```

### Go Install

```bash
go install github.com/rlrghb/olkcli/cmd/olk@latest
```

### Homebrew (coming soon)

```bash
brew install rlrghb/tap/olk
```

## Quick Start

```bash
# Authenticate (opens browser for device-code flow)
olk auth login

# List recent inbox messages
olk mail list

# Read a specific message
olk mail get <message-id>

# Send an email
olk mail send --to user@example.com --subject "Hello" --body "Hi there"

# Pipe body from stdin
echo "Hello from the CLI" | olk mail send --to user@example.com --subject "Piped"

# Search mail
olk mail search "from:boss@company.com subject:urgent"

# View this week's calendar
olk calendar events

# Today's events
olk today

# Create a meeting
olk calendar create --subject "Standup" --start 2024-01-15T09:00 --end 2024-01-15T09:30 --attendees colleague@company.com

# List contacts
olk contacts list
```

## Output Formats

| Flag | Format | Use Case |
|------|--------|----------|
| *(default)* | Aligned table | Human reading |
| `--json` | JSON envelope | Scripting with `jq` |
| `--plain` | Tab-separated | Piping to `awk`, `cut` |

### JSON Envelope

```bash
olk mail list --json
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
olk mail list --json --results-only | jq '.[0].subject'
```

### Field Selection

```bash
olk mail list --select from,subject
```

## Authentication

### Default (Zero Config)

```bash
olk auth login
```

Uses an embedded public client ID with device-code flow. Works for both personal and enterprise accounts.

### Custom App Registration

If your organization blocks the default client ID, or your admin requires apps to be registered under your tenant, you'll need to create your own app registration:

1. Go to [Azure Portal > App registrations](https://portal.azure.com/#view/Microsoft_AAD_RegisteredApps/ApplicationsListBlade) and click **New registration**
2. Set **Supported account types** to match your needs (single tenant or multi-tenant)
3. Under **Authentication > Advanced settings**, set **Allow public client flows** to **Yes** (required for device-code flow)
4. Under **API permissions**, add **Microsoft Graph** delegated permissions: `Mail.ReadWrite`, `Calendars.ReadWrite`, `Contacts.ReadWrite`, `User.Read`, `offline_access`
5. Copy the **Application (client) ID** and **Directory (tenant) ID** from the app's Overview page

Then use them:

```bash
olk auth login --client-id YOUR_CLIENT_ID --tenant-id YOUR_TENANT_ID
```

Or via environment variables:

```bash
export OLK_CLIENT_ID=your-client-id
export OLK_TENANT_ID=your-tenant-id
olk auth login
```

### Multi-Account

```bash
# Login to a second account
olk auth login

# List accounts
olk auth list

# Use a specific account
olk mail list --account user2@example.com

# Check auth status
olk auth status
```

### Token Storage

Refresh tokens are stored in the OS credential manager:
- **macOS**: Keychain
- **Linux**: Secret Service (GNOME Keyring / KDE Wallet)
- **Windows**: Windows Credential Manager

## Shortcuts

For common workflows, `olk` provides top-level shortcuts:

| Shortcut | Expands To |
|----------|-----------|
| `olk send` | `olk mail send` |
| `olk ls` | `olk mail list` |
| `olk inbox` | `olk mail list` |
| `olk search <q>` | `olk mail search <q>` |
| `olk today` | `olk calendar events --days 1` |
| `olk week` | `olk calendar events --days 7` |

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
olk auth login [--client-id ID] [--tenant-id ID]    Login via device code
olk auth logout [EMAIL]                              Remove stored credentials
olk auth list                                        List authenticated accounts
olk auth status                                      Check token validity
```

### Mail

```
olk mail list [-n 25] [-f FOLDER] [-u] [--from X] [--after DATE] [--before DATE]
olk mail get <ID> [--format full|text|html]
olk mail send --to X --subject Y [--body Z] [--cc X] [--bcc X] [--html]
olk mail search <QUERY> [-n 25]
olk mail reply <ID> --body X [--reply-all]
olk mail forward <ID> --to X [--comment Y]
olk mail move <ID> <FOLDER>
olk mail delete <ID> [--force]
olk mail mark <ID> --read|--unread
olk mail folders
olk mail attachments <ID>
```

### Calendar

```
olk calendar events [-d 7] [--after DATE] [--before DATE] [--calendar ID] [-n 25]
olk calendar get <ID>
olk calendar create --subject X --start Y --end Z [--location L] [--attendees A] [--all-day] [--online-meeting]
olk calendar update <ID> [--subject X] [--start Y] [--end Z] [--location L]
olk calendar delete <ID> [--force]
olk calendar respond <ID> accept|decline|tentative
olk calendar calendars
```

### Contacts

```
olk contacts list [-n 25] [--folder ID]
olk contacts get <ID>
olk contacts create --first-name X --last-name Y [--email Z] [--phone P] [--company C] [--title T]
olk contacts update <ID> [--first-name X] [--last-name Y] [--email Z] [--phone P] [--company C] [--title T]
olk contacts delete <ID> [--force]
olk contacts search <QUERY> [-n 25]
```

## Configuration

Config is stored at `~/.config/olk/`:

```
~/.config/olk/
├── config.json          # Default account, client IDs
└── accounts/            # Account metadata (email, display name)
    └── user@example.com.json
```

Override the config directory with `OLK_CONFIG_DIR`.

## Scripting Examples

```bash
# Count unread messages
olk mail list --unread --json --results-only | jq length

# Get subjects of today's events
olk today --json --results-only | jq -r '.[].subject'

# Export contacts as CSV
olk contacts list --plain --select name,email

# Send from a script
olk send --to ops@company.com --subject "Deploy complete" --body "$(date): v1.2.3 deployed"

# Process inbox with jq
olk mail list --json --results-only | jq -r '.[] | select(.isRead == false) | "\(.from): \(.subject)"'
```

## AI Agent Integration

`olk` ships with a [`SKILL.md`](SKILL.md) that follows the [Agent Skills](https://agentskills.io) open standard. This lets AI coding assistants discover and use `olk` commands on your behalf — checking mail, scheduling meetings, managing contacts, all from within your AI workflow.

### Supported Platforms

| Platform | How it works |
|----------|-------------|
| [Claude Code](https://claude.com/claude-code) | Copy `SKILL.md` into your Claude skills directory. Invoke with `/olk` or let Claude use it automatically. |
| [OpenClaw](https://openclaw.ai) | Reads `SKILL.md` with metadata gating (`requires.bins: ["olk"]`). Auto-installs via `go install` if missing. |
| Other [AgentSkills](https://agentskills.io)-compatible tools | Any tool supporting the Agent Skills standard can pick up the `SKILL.md` for command discovery and usage instructions. |

### Installation

**Claude Code** (personal — available across all projects):

```bash
mkdir -p ~/.claude/skills/olk
cp SKILL.md ~/.claude/skills/olk/SKILL.md
```

**Claude Code** (project-scoped — available only in this repo):

```bash
mkdir -p .claude/skills/olk
cp SKILL.md .claude/skills/olk/SKILL.md
```

**OpenClaw**: Place `SKILL.md` in your OpenClaw skills directory, or point OpenClaw to this repo.

Then ask your AI assistant to "check my inbox" or "send an email" and it will use `olk`.

### What the skill teaches AI agents

- All commands, flags, and output formats
- When to use `--json --results-only` for programmatic parsing
- KQL search syntax for mail
- How to handle auth errors
- Safety rules (confirm before sending, never guess IDs, use `--force` for deletes)

## Architecture

```
olkcli/
├── cmd/olk/main.go              # Entry point
├── internal/
│   ├── cmd/                     # CLI commands (kong)
│   ├── msauth/                  # Microsoft OAuth2 (device code, token refresh)
│   ├── graphapi/                # Microsoft Graph API wrapper
│   ├── config/                  # Configuration management
│   ├── secrets/                 # OS keyring integration
│   ├── outfmt/                  # Output formatting (JSON/table/TSV)
│   └── errfmt/                  # Error formatting
├── SKILL.md                     # Agent Skills standard — AI assistant integration
├── Makefile
├── .goreleaser.yaml
└── go.mod
```

## Development

```bash
make build      # Build to ./bin/olk
make test       # Run tests
make lint       # Run golangci-lint
make install    # Install to $GOPATH/bin
make clean      # Remove build artifacts
```

## License

MIT License. See [LICENSE](LICENSE) for details.
