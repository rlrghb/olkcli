---
name: olk
description: Microsoft Outlook and OneDrive CLI for email, calendar, contacts, tasks, and files via Microsoft Graph API.
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
              "id": "brew",
              "kind": "brew",
              "formula": "rlrghb/tap/olk",
              "bins": ["olk"],
              "label": "Install olk (Homebrew)",
            },
          ],
      },
  }
---

# olk

Use `olk` for Outlook Mail/Calendar/Contacts/Tasks and OneDrive files. Works with personal Microsoft accounts and enterprise Azure AD/Entra ID.

Setup (once)

- `olk auth login` — device-code OAuth2 flow for personal accounts (opens browser)
- `olk auth login --enterprise` — login with enterprise scopes for work/school accounts (enables OOO, inbox rules, directory search)
- `olk auth login --client-id ID --tenant-id ID` — enterprise custom app registration
- `olk auth list` — list authenticated accounts
- `olk auth status` — check token validity
- `olk auth logout [EMAIL]` — remove stored credentials
- `olk auth clean --force` — remove ALL stored accounts and tokens

Mail

- List inbox: `olk mail list [-n 25] [-f FOLDER] [-u] [--from SENDER] [--after DATE] [--before DATE] [--focused] [--other]`
- Read message: `olk mail get <ID> [--format full|text|html]`
- Send (plain): `olk mail send --to a@b.com --subject "Hi" --body "Hello"`
- Send (HTML): `olk mail send --to a@b.com --subject "Hi" --body "<p>Hello</p>" --html`
- Send (stdin): `echo "Hello" | olk mail send --to a@b.com --subject "Hi"`
- Send (multi-recipient): `olk mail send --to a@b.com --to b@c.com --cc d@e.com --subject "Hi" --body "Hello"`
- Send with attachments: `olk mail send --to a@b.com --subject "Report" --body "See attached" --attach report.pdf --attach data.csv`
- Send with importance: `olk mail send --to a@b.com --subject "Urgent" --body "ASAP" --importance high`
- Send with read receipt: `olk mail send --to a@b.com --subject "Contract" --body "Please review" --read-receipt`
- Search (KQL): `olk mail search "from:boss@co.com subject:urgent" [-n 25]`
- Reply: `olk mail reply <ID> --body "Thanks"`
- Reply all: `olk mail reply <ID> --body "Thanks" --reply-all`
- Forward: `olk mail forward <ID> --to a@b.com [--comment "FYI"]`
- Move: `olk mail move <ID> <FOLDER>`
- Delete: `olk mail delete <ID> --force`
- Mark read/unread: `olk mail mark <ID> --read` or `olk mail mark <ID> --unread`
- List folders: `olk mail folders`
- Create folder: `olk mail folders create -n "Project X"`
- Rename folder: `olk mail folders rename <FOLDER_ID> -n "New Name"`
- Delete folder: `olk mail folders delete <FOLDER_ID> --force`
- List attachments: `olk mail attachments <ID>`
- Download all attachments: `olk mail attachments <ID> --save [--out DIR]`
- Download specific attachment: `olk mail attachments <ID> --attachment-id <ATT_ID> [--out DIR]`

Drafts

- List drafts: `olk mail drafts list [-n 25]`
- Create draft: `olk mail drafts create --to a@b.com --subject "Draft" --body "WIP" [--cc X] [--bcc X] [--html]`
- Create draft (stdin): `echo "WIP" | olk mail drafts create --to a@b.com --subject "Draft"`
- Send draft: `olk mail drafts send <DRAFT_ID>`
- Delete draft: `olk mail drafts delete <DRAFT_ID> --force`

Flags & Categories

- Flag for follow-up: `olk mail flag <ID> flagged|complete|notFlagged`
- Set importance: `olk mail importance <ID> low|normal|high`
- Set categories: `olk mail categorize <ID> -c "Red Category" -c "Blue Category"`
- Clear categories: `olk mail categorize <ID> -c none`
- List category definitions: `olk mail categories list`
- Create category: `olk mail categories create -n "My Category" [--preset preset0]`
- Delete category: `olk mail categories delete <ID> --force`
- Color presets: `none`, `preset0` (red) through `preset24`

Out-of-Office (enterprise/work accounts only — requires `olk auth login --enterprise`)

- Get auto-reply settings: `olk mail ooo get`
- Enable auto-reply: `olk mail ooo set --message "I'm out of office"`
- Enable scheduled: `olk mail ooo set --message "On vacation" --start 2026-04-10 --end 2026-04-17 [--audience none|contactsOnly|all]`
- External message: `olk mail ooo set --message "Internal msg" --external-message "External msg"`
- Disable auto-reply: `olk mail ooo off`

Inbox Rules (enterprise/work accounts only — requires `olk auth login --enterprise`)

- List rules: `olk mail rules list`
- Create rule: `olk mail rules create --name "Archive boss" --from boss@co.com --move Archive`
- Create rule with multiple actions: `olk mail rules create --name "Auto-read newsletters" --subject-contains "newsletter" --mark-read`
- Create rule with forwarding: `olk mail rules create --name "Forward invoices" --subject-contains "invoice" --forward-to accounting@co.com`
- Delete rule: `olk mail rules delete <RULE_ID> --force`

Focused Inbox

- List focused messages: `olk mail list --focused`
- List other messages: `olk mail list --other`
- Combine with filters: `olk mail list --focused --unread`

Well-known folder names: `inbox`, `sentitems`, `drafts`, `deleteditems`, `junkemail`, `archive`.

Calendar

- List events (next 7 days): `olk calendar events [-d DAYS] [--after DATE] [--before DATE] [--calendar ID] [-n 25]`
- Get event: `olk calendar get <ID>`
- Create event: `olk calendar create --subject "Standup" --start 2025-06-15T09:00 --end 2025-06-15T09:30`
- Create with attendees: `olk calendar create --subject "Sync" --start 2025-06-15T10:00 --end 2025-06-15T10:30 --attendees a@b.com --attendees c@d.com`
- Create all-day: `olk calendar create --subject "Offsite" --start 2025-06-15 --end 2025-06-16 --all-day`
- Create with Teams link: `olk calendar create --subject "Call" --start 2025-06-15T14:00 --end 2025-06-15T14:30 --online-meeting`
- Create recurring: `olk calendar create --subject "Standup" --start 2025-06-15T09:00 --end 2025-06-15T09:15 -r daily`
- Recurrence options: `daily`, `weekdays` (Mon-Fri), `weekly`, `monthly`, `yearly`
- Update event: `olk calendar update <ID> [--subject X] [--start Y] [--end Z] [--location L]`
- Delete event: `olk calendar delete <ID> --force`
- Respond to invite: `olk calendar respond <ID> accept|decline|tentative`
- List calendars: `olk calendar calendars`
- Check availability: `olk calendar availability --emails user@co.com [--emails user2@co.com] [-d DAYS] [--after DATE] [--before DATE]`
- Calendar view (expanded recurring): `olk calendar view [-d 7] [--after DATE] [--before DATE] [--calendar ID] [-n 50]`
- Find meeting times (enterprise only): `olk calendar find-times --attendees a@b.com --attendees c@d.com [-d 60] [--after DATE] [--before DATE]`

People / Directory

- Search people: `olk people search "john" [-n 25]`
- Search by name: `olk people search "Jane Smith"`
- Personal accounts search known contacts; enterprise accounts also search the organization directory

Contacts

- List: `olk contacts list [-n 25] [--skip N] [--sort displayName|givenName|surname]`
- Get: `olk contacts get <ID>`
- Create: `olk contacts create --first-name John --last-name Doe [-e j@d.com] [-e backup@d.com] [-p 555-1234] [--business-phone P] [--home-phone P] [--company Acme] [--title Engineer] [--department D] [--manager M] [--birthday YYYY-MM-DD] [--notes N] [--middle-name M] [--nickname N] [-g CATEGORY] [--street S] [--city C] [--state S] [--postal-code P] [--country C] [--address-type business|home|other]`
- Update: `olk contacts update <ID> [--first-name X] [--last-name Y] [-e EMAIL]... [-p MOBILE] [--business-phone P] [--home-phone P] [--company C] [--title T] [--department D] [--manager M] [--birthday YYYY-MM-DD] [--notes N] [--middle-name M] [--nickname N] [-g CATEGORY]... [--street S] [--city C] [--state S] [--postal-code P] [--country C] [--address-type business|home|other]`
- Delete: `olk contacts delete <ID> --force`
- Search: `olk contacts search "John" [-n 25]`

Tasks (Microsoft To Do)

- List task lists: `olk todo lists`
- Create task list: `olk todo lists create -n "Project Tasks"`
- Delete task list: `olk todo lists delete <LIST_ID> --force`
- List tasks: `olk todo list [--list LIST_ID] [-n 25] [--status notStarted|inProgress|completed|waitingOnOthers|deferred]`
- Get task: `olk todo get <TASK_ID> [--list LIST_ID]`
- Create task: `olk todo create --title "Buy groceries" [--due 2026-04-15] [--start 2026-04-10] [--importance low|normal|high] [--body "Notes"] [--reminder 2026-04-14T09:00] [--recurrence daily|weekdays|weekly|monthly|yearly] [-c "Work" -c "Urgent"] [--list LIST_ID]`
- Update task: `olk todo update <TASK_ID> [--title X] [--due DATE] [--start DATE] [--importance low|normal|high] [--body TEXT] [--reminder DATETIME] [--recurrence PATTERN] [-c CATEGORY] [--list LIST_ID]`
- Complete task: `olk todo complete <TASK_ID> [--list LIST_ID]`
- Delete task: `olk todo delete <TASK_ID> --force [--list LIST_ID]`

Checklist Items

- List checklist items: `olk todo checklist list <TASK_ID> [--list LIST_ID]`
- Create checklist item: `olk todo checklist create <TASK_ID> -n "Step 1" [--list LIST_ID]`
- Toggle checked/unchecked: `olk todo checklist toggle <TASK_ID> <ITEM_ID> [--list LIST_ID]`
- Update checklist item: `olk todo checklist update <TASK_ID> <ITEM_ID> -n "New name" [--list LIST_ID]`
- Delete checklist item: `olk todo checklist delete <TASK_ID> <ITEM_ID> --force [--list LIST_ID]`

Task Attachments

- List attachments: `olk todo attach list <TASK_ID> [--list LIST_ID]`
- Upload attachment: `olk todo attach upload <TASK_ID> <FILE> [--list LIST_ID]`
- Download attachment: `olk todo attach download <TASK_ID> <ATTACHMENT_ID> [--out DIR] [--list LIST_ID]`
- Delete attachment: `olk todo attach delete <TASK_ID> <ATTACHMENT_ID> --force [--list LIST_ID]`

Linked Resources

- List linked resources: `olk todo links list <TASK_ID> [--list LIST_ID]`
- Create linked resource: `olk todo links create <TASK_ID> -n "Resource name" [--url URL] [--app-name APP] [--external-id ID] [--list LIST_ID]`
- Delete linked resource: `olk todo links delete <TASK_ID> <RESOURCE_ID> --force [--list LIST_ID]`

If `--list` is omitted, the default (first) task list is used automatically.

OneDrive

- List drives: `olk drive list`
- Drive info: `olk drive info [--drive-id ID]`
- List folder: `olk drive ls [PATH] [--drive-id ID] [-n 50]`
- Get item details: `olk drive get <ID> [--drive-id ID]`
- Search: `olk drive search <QUERY> [--drive-id ID] [-n 25]`
- Recent files: `olk drive recent [--drive-id ID]`
- Shared with me: `olk drive shared [--drive-id ID]`
- Download: `olk drive download <ID> [--out DIR] [--drive-id ID]`
- Upload: `olk drive upload <LOCAL_PATH> <REMOTE_PATH> [--drive-id ID] [--replace]`
- Create folder: `olk drive mkdir <PATH> [--drive-id ID]`
- Copy: `olk drive cp <ID> <DEST_PATH> [--name NEW_NAME] [--drive-id ID]`
- Move/rename: `olk drive mv <ID> <DEST_PATH> [--drive-id ID]`
- Delete: `olk drive rm <ID> --force [--drive-id ID]`
- Share: `olk drive share <ID> [--type view|edit] [--scope anonymous|organization] [--drive-id ID]`
- Version history: `olk drive versions <ID> [--drive-id ID]`

If `--drive-id` is omitted, the user's primary drive is used automatically.

Configuration

- Set timezone: `olk config set timezone America/New_York`
- Get timezone: `olk config get timezone`
- Timezone precedence: `--tz` flag > `OLK_TIMEZONE` env > config file > system local
- JSON output keeps raw UTC times; envelope includes `"timezone"` field

User Profile

- `olk whoami` — display current user's name, email, job title, department, office, phone

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
- `--tz TIMEZONE` — IANA time zone for display, e.g. `America/New_York`, `UTC`, `Local` (env: `OLK_TIMEZONE`)

Scripting Examples

- Count unread: `olk mail list --unread --json --results-only | jq length`
- Today's subjects: `olk today --json --results-only | jq -r '.[].subject'`
- Export contacts CSV: `olk contacts list --plain --select name,email`
- Send from script: `olk send --to ops@co.com --subject "Deploy done" --body "$(date): v1.2.3 deployed"`
- Send with attachment: `olk send --to boss@co.com --subject "Report" --attach report.pdf`
- Process inbox: `olk mail list --json --results-only | jq -r '.[] | select(.isRead == false) | "\(.from): \(.subject)"'`
- Download all attachments: `olk mail attachments <ID> --save --out ./downloads`
- Check if someone is free: `olk calendar availability --emails colleague@co.com --json --results-only | jq '.[] | .items'`
- List incomplete tasks: `olk todo list --status notStarted --json --results-only | jq -r '.[].title'`
- Set vacation responder: `olk mail ooo set --message "On vacation until April 17" --start 2026-04-10 --end 2026-04-17`
- List inbox rules: `olk mail rules list --json --results-only | jq -r '.[] | select(.isEnabled) | .displayName'`
- Find meeting times: `olk calendar find-times --attendees a@b.com --attendees c@d.com --json --results-only | jq '.[0]'`
- Search people: `olk people search "engineering" --json --results-only | jq -r '.[].email'`
- Focused inbox unread: `olk mail list --focused --unread --json --results-only | jq length`
- List large files: `olk drive ls /Documents --json --results-only | jq '[.[] | select(.size > 10000000)] | sort_by(.size) | reverse'`
- Check quota: `olk drive info --json --results-only | jq '{used: .quotaUsed, total: .quotaTotal}'`

Notes

- Set `OLK_TIMEZONE=America/New_York` to display times in your timezone.
- Set `OLK_ACCOUNT=you@example.com` to avoid repeating `--account`.
- Set `OLK_TODO_LIST=<list-id>` to avoid repeating `--list` for todo commands.
- Set `OLK_DRIVE_ID=<drive-id>` to avoid repeating `--drive-id` for drive commands.
- Set `OLK_KEYRING_PASSWORD=<password>` for headless/non-interactive environments (file-backend keyring).
- For scripting, prefer `--json --results-only` plus `jq`.
- IDs are opaque Microsoft Graph strings. Always get them from `list` or `search` first — never guess.
- Dates are ISO 8601: `2025-06-15` or `2025-06-15T09:00`.
- Mail search uses KQL, not regex. Operators: `from:`, `to:`, `subject:`, `hasAttachment:`, `received>=`.
- If `--body` is omitted from `mail send` or `mail drafts create`, body is read from stdin.
- Destructive commands (`delete`) require `--force` or will prompt for confirmation.
- Confirm before sending mail or creating/deleting events.
- If a command fails with an auth error, check `olk auth status` first.
- Some features are enterprise-only (work/school accounts): out-of-office, inbox rules, find meeting times, and directory search. These require `olk auth login --enterprise`.
- OneDrive commands require re-login (`olk auth login`) if you authenticated before OneDrive support was added.
