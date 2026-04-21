# olk — Microsoft Outlook in Your Terminal

A fast, scriptable CLI for Microsoft Outlook and OneDrive via the Microsoft Graph API. Manage email, calendar, contacts, tasks, and OneDrive files from the command line.

Works with both **personal Microsoft accounts** and **enterprise (Azure AD / Entra ID)** accounts. Zero-config setup with device-code authentication — just run `olk auth login` and go. For enterprise accounts, use `olk auth login --enterprise` to enable additional features like out-of-office, inbox rules, and directory search.

## Key Capabilities

### Mail
- **List & read** inbox messages with filtering by sender, date, read status
- **Send** emails with To/CC/BCC, HTML bodies, stdin piping, attachments, importance
- **Search** using KQL (Keyword Query Language) syntax
- **Reply, reply-all, forward** messages
- **Move** messages between folders
- **Delete** and **mark read/unread**
- **Manage folders**: list, create, rename, delete mail folders
- **View and download attachments**
- **Drafts**: create, list, send, delete draft messages
- **Flags & categories**: flag for follow-up, set importance, assign categories, manage category definitions
- **Out-of-office**: get, set, and disable auto-reply / vacation responder *(enterprise only)*
- **Inbox rules**: list, create, and delete server-side mail rules *(enterprise only)*
- **Focused Inbox**: filter by `--focused` or `--other` classification
- **Read receipts**: request read receipts with `--read-receipt`

### Calendar
- **List events** with configurable date ranges (default: 7 days ahead)
- **Calendar view** with expanded recurring event occurrences
- **Recurring events** displayed with human-readable recurrence patterns
- **Create events** with location, attendees, all-day, online meeting, and recurrence support
- **Update and delete** events
- **Respond** to invitations (accept, decline, tentative)
- **List calendars** across your account
- **Check availability** / free-busy lookup for one or more users
- **Find meeting times** — suggest available slots for multiple attendees *(enterprise only)*

### Contacts
- **List, search, create, update, delete** contacts with sorting and pagination
- Fields: name, multiple emails, phone, company, job title, department, manager, birthday, notes, addresses, categories, and more

### Tasks (Microsoft To Do)
- **Manage task lists**: list, create, delete task lists
- **Create, update, complete, delete** tasks with due dates, importance, notes, start dates, reminders, recurrence, and categories
- **Checklists**: list, create, toggle, update, and delete checklist items within tasks
- **Attachments**: list, upload, download, and delete task file attachments
- **Linked resources**: list, create, and delete linked resources on tasks

### OneDrive
- **Browse** files and folders with `drive ls [path]`
- **Search** files by name or content
- **Download** and **upload** files (simple and resumable for large files)
- **Create folders**, **copy**, **move**, and **delete** items
- **Share** files with view or edit links
- **Version history** for files
- **Drive info** including quota usage

### People / Directory
- **Search** people by name — returns name, email, job title, department, company. Personal accounts search known contacts; enterprise accounts also search the organization directory

### User Profile
- **`olk whoami`** — display current user info (name, email, job title, department)

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

### Homebrew

```bash
brew install rlrghb/tap/olk
```

On macOS, if you see "olk can't be opened because Apple cannot verify it", run:

```bash
xattr -d com.apple.quarantine $(brew --prefix)/bin/olk
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
# Personal accounts (Outlook.com, Hotmail, Live.com)
olk auth login

# Enterprise accounts (work/school) — enables OOO, inbox rules, directory search
olk auth login --enterprise
```

Uses an embedded public client ID with device-code flow. The `--enterprise` flag requests additional scopes (`User.ReadBasic.All`, `MailboxSettings.ReadWrite`) needed for enterprise-only features. Personal accounts should not use `--enterprise` as these scopes are not supported.

> **Note:** If you upgrade to a version that adds new features (e.g. OneDrive support), you may need to re-run `olk auth login` to grant the new permissions. If you see "access denied" errors, re-login to refresh your token scopes.

### Custom App Registration

If your organization blocks the default client ID, or your admin requires apps to be registered under your tenant, you'll need to create your own app registration:

1. Go to [Azure Portal > App registrations](https://portal.azure.com/#view/Microsoft_AAD_RegisteredApps/ApplicationsListBlade) and click **New registration**
2. Set **Supported account types** to match your needs (single tenant or multi-tenant)
3. Under **Authentication > Advanced settings**, set **Allow public client flows** to **Yes** (required for device-code flow)
4. Under **API permissions**, add **Microsoft Graph** delegated permissions: `Mail.ReadWrite`, `Mail.Send`, `Calendars.ReadWrite`, `Contacts.ReadWrite`, `Tasks.ReadWrite`, `Files.ReadWrite`, `People.Read`, `User.Read`, `User.ReadBasic.All`, `MailboxSettings.ReadWrite`, `offline_access` (use `Files.Read` instead of `Files.ReadWrite` for read-only access)
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

When no native credential manager is available (e.g. headless Linux without Secret Service), `olk` falls back to an encrypted file store and prompts for a password on **stderr**.

#### Headless / Non-interactive Use

For headless environments, CI/CD pipelines, or LLM-driven automation, set the `OLK_KEYRING_PASSWORD` environment variable to supply the file-backend password without an interactive prompt:

```bash
export OLK_KEYRING_PASSWORD="your-keyring-password"
olk mail list --json | jq '.subject'
```

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
| `--tz TIMEZONE` | `OLK_TIMEZONE` | IANA time zone for display (e.g. `America/New_York`) |
| | `OLK_KEYRING_PASSWORD` | File-backend keyring password (for headless use) |

## Commands Reference

### Auth

```
olk auth login [--enterprise] [--client-id ID] [--tenant-id ID]  Login via device code
olk auth logout [EMAIL]                              Remove stored credentials
olk auth clean --force                               Remove ALL accounts and tokens
olk auth list                                        List authenticated accounts
olk auth status                                      Check token validity
```

### Mail

```
olk mail list [-n 25] [-f FOLDER] [-u] [--from X] [--after DATE] [--before DATE] [--focused] [--other]
olk mail get <ID> [--format full|text|html]
olk mail send --to X --subject Y [--body Z] [--cc X] [--bcc X] [--html] [--attach FILE] [--importance low|normal|high] [--read-receipt]
olk mail search <QUERY> [-n 25]
olk mail reply <ID> --body X [--reply-all]
olk mail forward <ID> --to X [--comment Y]
olk mail move <ID> <FOLDER>
olk mail delete <ID> [--force]
olk mail mark <ID> --read|--unread
olk mail folders                                     List mail folders
olk mail folders create -n "Name"                    Create a mail folder
olk mail folders rename <ID> -n "New Name"           Rename a mail folder
olk mail folders delete <ID> --force                 Delete a mail folder
olk mail attachments <ID>                            List attachments
olk mail attachments <ID> --save [--out DIR]         Download all attachments
olk mail attachments <ID> --attachment-id X [--out DIR]  Download specific attachment
olk mail drafts list [-n 25]                         List drafts
olk mail drafts create --to X --subject Y [--body Z] [--cc X] [--bcc X] [--html]
olk mail drafts send <DRAFT_ID>                      Send a draft
olk mail drafts delete <DRAFT_ID> --force            Delete a draft
olk mail flag <ID> flagged|complete|notFlagged       Set follow-up flag
olk mail importance <ID> low|normal|high             Set importance
olk mail categorize <ID> -c "Category Name"          Set categories (use -c none to clear)
olk mail categories list                             List category definitions
olk mail categories create -n "Name" [--preset X]    Create a category (preset0-preset24 or none)
olk mail categories delete <ID> --force              Delete a category
olk mail ooo get                                     Get auto-reply settings
olk mail ooo set -m "Message" [--start DATE] [--end DATE] [--audience none|contactsOnly|all]
olk mail ooo off                                     Disable auto-reply
olk mail rules list                                  List inbox rules
olk mail rules create --name X [--from Y] [--subject-contains Z] [--has-attachment] [--move FOLDER] [--mark-read] [--delete] [--forward-to EMAIL] [--set-importance low|normal|high]
olk mail rules delete <ID> --force                   Delete an inbox rule
```

### Calendar

```
olk calendar events [-d 7] [--after DATE] [--before DATE] [--calendar ID] [-n 25]
olk calendar view [-d 7] [--after DATE] [--before DATE] [--calendar ID] [-n 50]
olk calendar get <ID>
olk calendar create --subject X --start Y --end Z [--location L] [--attendees A] [--all-day] [--online-meeting] [-r daily|weekdays|weekly|monthly|yearly]
olk calendar update <ID> [--subject X] [--start Y] [--end Z] [--location L]
olk calendar delete <ID> [--force]
olk calendar respond <ID> accept|decline|tentative
olk calendar calendars
olk calendar availability --emails X [-d DAYS] [--after DATE] [--before DATE]
olk calendar find-times --attendees X [--attendees Y] [-d 60] [--after DATE] [--before DATE]
```

### People

```
olk people search <QUERY> [-n 25]
```

### Contacts

```
olk contacts list [-n 25] [--skip N] [--sort displayName|givenName|surname]
olk contacts get <ID>
olk contacts create --first-name X --last-name Y [-e EMAIL]... [-p MOBILE] [--business-phone P] [--home-phone P] [--company C] [--title T] [--department D] [--manager M] [--birthday YYYY-MM-DD] [--notes N] [--middle-name M] [--nickname N] [-g CATEGORY]... [--street S] [--city C] [--state S] [--postal-code P] [--country C] [--address-type business|home|other]
olk contacts update <ID> [--first-name X] [--last-name Y] [-e EMAIL]... [-p MOBILE] [--business-phone P] [--home-phone P] [--company C] [--title T] [--department D] [--manager M] [--birthday YYYY-MM-DD] [--notes N] [--middle-name M] [--nickname N] [-g CATEGORY]... [--street S] [--city C] [--state S] [--postal-code P] [--country C] [--address-type business|home|other]
olk contacts delete <ID> [--force]
olk contacts search <QUERY> [-n 25]
```

### Tasks (Microsoft To Do)

```
olk todo lists                                       List task lists
olk todo lists create -n "Name"                      Create a task list
olk todo lists delete <ID> --force                   Delete a task list
olk todo list [--list ID] [-n 25] [--status STATUS]  List tasks
olk todo get <TASK_ID> [--list ID]                   Get task details
olk todo create -t "Title" [--due DATE] [--importance low|normal|high] [--body TEXT] [--list ID] [--start DATE] [--reminder DATETIME] [--recurrence daily|weekdays|weekly|monthly|yearly] [--categories CAT]
olk todo update <TASK_ID> [--title X] [--due DATE] [--importance low|normal|high] [--body TEXT] [--list ID] [--start DATE] [--reminder DATETIME] [--recurrence daily|weekdays|weekly|monthly|yearly] [--categories CAT]
olk todo complete <TASK_ID> [--list ID]              Mark task complete
olk todo delete <TASK_ID> --force [--list ID]        Delete a task
olk todo checklist list <TASK_ID> [--list ID]        List checklist items
olk todo checklist create <TASK_ID> -n "Name" [--list ID]  Create a checklist item
olk todo checklist toggle <TASK_ID> <ITEM_ID> [--list ID]  Toggle checked/unchecked
olk todo checklist update <TASK_ID> <ITEM_ID> -n "Name" [--list ID]  Update checklist item
olk todo checklist delete <TASK_ID> <ITEM_ID> --force [--list ID]    Delete checklist item
olk todo attach list <TASK_ID> [--list ID]           List task attachments
olk todo attach upload <TASK_ID> <FILE> [--list ID]  Upload a file attachment
olk todo attach download <TASK_ID> <ATTACHMENT_ID> [--list ID] [--out DIR]  Download an attachment
olk todo attach delete <TASK_ID> <ATTACHMENT_ID> --force [--list ID]        Delete an attachment
olk todo links list <TASK_ID> [--list ID]            List linked resources
olk todo links create <TASK_ID> -n "Name" --url "URL" [--list ID]   Create a linked resource
olk todo links delete <TASK_ID> <RESOURCE_ID> --force [--list ID]   Delete a linked resource
```

### Configuration

```
olk config set timezone America/New_York             Set display timezone
olk config get timezone                              Get current timezone setting
```

### User Profile

```
olk whoami                                           Display current user info
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

### Timezone

By default, times are displayed in your system's local timezone. You can override this:

```bash
# Per-command
olk calendar events --tz America/New_York

# Via environment variable
export OLK_TIMEZONE=Europe/London

# Persistent (saved to config)
olk config set timezone America/Chicago
```

Precedence: `--tz` flag > `OLK_TIMEZONE` env var > config file > system local timezone. JSON output emits UTC timestamps as RFC3339 with a `Z` suffix (e.g. `2026-04-22T15:15:00Z`), so `new Date(...)` parses them correctly; the `timezone` field in the envelope indicates the display timezone.

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

## Account Compatibility

Most features work with both personal and enterprise accounts. A few features require an enterprise (work/school) account and `olk auth login --enterprise`:

| Feature | Personal | Enterprise |
|---------|----------|------------|
| Mail (list, send, search, reply, forward, move, delete) | Yes | Yes |
| Mail folders (list, create, rename, delete) | Yes | Yes |
| Mail drafts | Yes | Yes |
| Flags, importance, categories | Yes | Yes |
| Calendar (events, create, update, delete, respond) | Yes | Yes |
| Recurring events | Yes | Yes |
| Contacts | Yes | Yes |
| Tasks (Microsoft To Do) | Yes | Yes |
| People search | Yes | Yes |
| Out-of-office / auto-reply | No | Yes |
| Inbox rules | No | Yes |
| Focused Inbox | Yes | Yes |
| Availability / free-busy | Yes | Yes |
| Find meeting times | No | Yes |
| OneDrive (browse, upload, download, share) | Yes | Yes |
| Directory search (fallback) | No | Yes |

## Privacy & Security

- **No telemetry**: `olk` collects no analytics, usage data, or crash reports
- **No third-party services**: All communication is directly between your machine and Microsoft Graph API
- **Token storage**: OAuth refresh tokens are stored in your OS credential manager (macOS Keychain, Linux Secret Service, Windows Credential Manager) — never in plain-text files. The encrypted file fallback prompts on stderr; set `OLK_KEYRING_PASSWORD` for non-interactive use
- **PKCE**: Device code flow uses Proof Key for Code Exchange (RFC 7636) for defense-in-depth
- **Data stays local**: Email bodies, attachments, and contacts are streamed to stdout and never cached to disk
- **Clean removal**: Run `olk auth clean --force` to remove all stored accounts and tokens
- **Vulnerability reporting**: See [SECURITY.md](SECURITY.md) for our disclosure policy

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
