---
name: olk
description: Microsoft Outlook CLI for email, calendar, and contacts via Microsoft Graph API.
homepage: https://github.com/rlrghb/olkcli
metadata:
  {
    "openclaw":
      {
        "emoji": "📬",
        "requires": { "bins": ["olk"] },
        "install":
          [
            {
              "id": "go",
              "kind": "go",
              "module": "github.com/rlrghb/olkcli/cmd/olk@latest",
              "bins": ["olk"],
              "label": "Install olk (go install)",
            },
          ],
      },
  }
---

# olk

Use `olk` for Outlook Mail/Calendar/Contacts. Works with personal Microsoft accounts and enterprise Azure AD/Entra ID.

Setup (once)

- `olk auth login` — device-code OAuth2 flow (opens browser)
- `olk auth login --client-id ID --tenant-id ID` — enterprise custom app registration
- `olk auth list` — list authenticated accounts
- `olk auth status` — check token validity
- `olk auth logout [EMAIL]` — remove stored credentials

Mail

- List inbox: `olk mail list [-n 25] [-f FOLDER] [-u] [--from SENDER] [--after DATE] [--before DATE]`
- Read message: `olk mail get <ID> [--format full|text|html]`
- Send (plain): `olk mail send --to a@b.com --subject "Hi" --body "Hello"`
- Send (HTML): `olk mail send --to a@b.com --subject "Hi" --body "<p>Hello</p>" --html`
- Send (stdin): `echo "Hello" | olk mail send --to a@b.com --subject "Hi"`
- Send (multi-recipient): `olk mail send --to a@b.com --to b@c.com --cc d@e.com --subject "Hi" --body "Hello"`
- Search (KQL): `olk mail search "from:boss@co.com subject:urgent" [-n 25]`
- Reply: `olk mail reply <ID> --body "Thanks"`
- Reply all: `olk mail reply <ID> --body "Thanks" --reply-all`
- Forward: `olk mail forward <ID> --to a@b.com [--comment "FYI"]`
- Move: `olk mail move <ID> <FOLDER>`
- Delete: `olk mail delete <ID> --force`
- Mark read/unread: `olk mail mark <ID> --read` or `olk mail mark <ID> --unread`
- List folders: `olk mail folders`
- List attachments: `olk mail attachments <ID>`

Well-known folder names: `inbox`, `sentitems`, `drafts`, `deleteditems`, `junkemail`, `archive`.

Calendar

- List events (next 7 days): `olk calendar events [-d DAYS] [--after DATE] [--before DATE] [--calendar ID] [-n 25]`
- Get event: `olk calendar get <ID>`
- Create event: `olk calendar create --subject "Standup" --start 2025-06-15T09:00 --end 2025-06-15T09:30`
- Create with attendees: `olk calendar create --subject "Sync" --start 2025-06-15T10:00 --end 2025-06-15T10:30 --attendees a@b.com --attendees c@d.com`
- Create all-day: `olk calendar create --subject "Offsite" --start 2025-06-15 --end 2025-06-16 --all-day`
- Create with Teams link: `olk calendar create --subject "Call" --start 2025-06-15T14:00 --end 2025-06-15T14:30 --online-meeting`
- Update event: `olk calendar update <ID> [--subject X] [--start Y] [--end Z] [--location L]`
- Delete event: `olk calendar delete <ID> --force`
- Respond to invite: `olk calendar respond <ID> accept|decline|tentative`
- List calendars: `olk calendar calendars`

Contacts

- List: `olk contacts list [-n 25]`
- Get: `olk contacts get <ID>`
- Create: `olk contacts create --first-name John --last-name Doe [--email j@d.com] [--phone 555-1234] [--company Acme] [--title Engineer]`
- Update: `olk contacts update <ID> [--first-name X] [--last-name Y] [--email Z] [--phone P] [--company C] [--title T]`
- Delete: `olk contacts delete <ID> --force`
- Search: `olk contacts search "John" [-n 25]`

Shortcuts

- `olk send ...` → `olk mail send ...`
- `olk ls ...` → `olk mail list ...`
- `olk inbox ...` → `olk mail list ...`
- `olk search <Q>` → `olk mail search <Q>`
- `olk today` → `olk calendar events --days 1`
- `olk week` → `olk calendar events --days 7`

Output Formats

- Default: human-readable aligned table.
- `--json`: JSON envelope `{ results, count, nextLink }`.
- `--json --results-only`: bare JSON array (best for scripting).
- `--plain`: tab-separated values for piping to `awk`, `cut`.
- `--select from,subject`: comma-separated field projection.

Global Flags

- `--json` — JSON output (env: `OLK_JSON`)
- `--plain` — TSV output (env: `OLK_PLAIN`)
- `--account EMAIL` — use a specific account (env: `OLK_ACCOUNT`)
- `--results-only` — unwrap JSON envelope (env: `OLK_RESULTS_ONLY`)
- `--select FIELDS` — field projection (env: `OLK_SELECT`)
- `--force` — skip confirmations (env: `OLK_FORCE`)
- `--dry-run` — preview without executing (env: `OLK_DRY_RUN`)
- `-v, --verbose` — verbose output (env: `OLK_VERBOSE`)
- `--color auto|never|always` — color mode (env: `OLK_COLOR`)
- `--timeout SECONDS` — request timeout, default 60 (env: `OLK_TIMEOUT`)

Scripting Examples

- Count unread: `olk mail list --unread --json --results-only | jq length`
- Today's subjects: `olk today --json --results-only | jq -r '.[].subject'`
- Export contacts CSV: `olk contacts list --plain --select name,email`
- Send from script: `olk send --to ops@co.com --subject "Deploy done" --body "$(date): v1.2.3 deployed"`
- Process inbox: `olk mail list --json --results-only | jq -r '.[] | select(.isRead == false) | "\(.from): \(.subject)"'`

Notes

- Set `OLK_ACCOUNT=you@example.com` to avoid repeating `--account`.
- For scripting, prefer `--json --results-only` plus `jq`.
- IDs are opaque Microsoft Graph strings. Always get them from `list` or `search` first — never guess.
- Dates are ISO 8601: `2025-06-15` or `2025-06-15T09:00`.
- Mail search uses KQL, not regex. Operators: `from:`, `to:`, `subject:`, `hasAttachment:`, `received>=`.
- If `--body` is omitted from `mail send`, body is read from stdin.
- Destructive commands (`delete`) require `--force` or will prompt for confirmation.
- Confirm before sending mail or creating/deleting events.
- If a command fails with an auth error, check `olk auth status` first.
