# Command reference

All commands accept the global flags shown below. Run `olk <command> --help`
for the exact generated schema.

## Global flags

```text
--json --plain --results-only --select FIELDS --concise
--account EMAIL --mailbox EMAIL --tz IANA_ZONE --timeout SECONDS
--immutable-ids --dry-run --force --verbose
--no-write --no-send --no-input --wrap-untrusted
--enable-commands CSV --enable-commands-exact CSV --disable-commands CSV
```

### Timeouts and retries

A timeout means the outcome is unknown: the request may have reached Microsoft
Graph and the message may have been sent. Before retrying a send, reply, or
forward, verify Sent Items, Drafts, or the recipient mailbox. Blindly retrying
can create duplicate mail.

## Authentication and profile

```bash
olk auth login [--enterprise] [--browser] [--scope SCOPE]
olk auth logout [EMAIL]
olk auth status
olk auth accounts
olk whoami
```

## Mail

```bash
olk mail list [-n N] [--folder ID_OR_PATH] [--from EMAIL] [--unread] [--focused|--other]
olk mail get <ID> [--body-format text|html]
olk mail search <KQL>
olk mail batch <ID> [--id ID ...]
olk mail thread <CONVERSATION_ID>
olk mail delta [--token TOKEN]
olk mail send --to EMAIL --subject SUBJECT --body BODY [--html]
olk mail reply <ID> --body BODY [--reply-all] [--html] [--draft] [--inline CID=PATH ...]
olk mail reply <ID> --body "Thanks" --draft
olk mail reply <ID> --body '<p>Thanks</p>' --html --draft
olk mail reply <ID> --body '<p>Thanks all</p>' --reply-all --html --draft
olk mail reply <ID> --body '<p><img src="cid:steps"></p>' --html --draft --inline steps=steps.png
olk mail forward <ID> --to EMAIL [--comment COMMENT] [--html]
olk mail mark <ID> read|unread
olk mail move <ID> ID_OR_PATH [--mailbox EMAIL]
olk mail delete <ID> --force [--mailbox EMAIL]
olk mail attachments <ID> [--save] [--out DIR] [--attachment-id ID]
olk mail folders list|create|rename|delete  # list traverses visible child folders
olk mail drafts list|create|send|delete|attach|update
olk mail drafts create --to EMAIL --subject SUBJECT --body '<img src="cid:logo">' --html --inline logo=logo.png
olk mail flag <ID> flagged|complete|notFlagged
olk mail categorize <ID> --category NAME
olk mail importance <ID> low|normal|high
olk mail ooo get|set|off
olk mail rules list|create|delete
```

`mail list` defaults to the Inbox when `--folder` is omitted. The `olk inbox`
and `olk ls` shortcuts use the same default; pass `--folder` to list another
folder.

To edit an existing draft without sending it:

```bash
olk mail drafts attach <DRAFT_ID> report.pdf --mailbox shared@example.com --json
olk mail drafts update <DRAFT_ID> --cc colleague@example.com --mailbox shared@example.com --json
olk mail drafts update <DRAFT_ID> --bcc= --dry-run --json
```

`attach` uploads one regular file under 3 MB. `update` replaces only the supplied
To, CC or BCC lists; omit a flag to leave that list unchanged, or pass an empty
string to clear it. The body, subject and reply history are left unchanged.
Both commands honour `--mailbox`, `--no-write`, `--no-send`, `--dry-run`, `--json`
and `--plain`. They check that the message is a draft before editing. Shared
mailbox edits need `Mail.ReadWrite.Shared` and Full Access. JSON receipts include
`id`, `mailbox` (empty for the signed-in user), and `dryRun`, plus `attachment`
or `recipients`. In recipient receipts, null means unchanged and [] means clear.
The new commands are CLI-only; the MCP tool allowlist is unchanged.

`mail reply --draft` creates a true threaded Outlook reply or reply-all draft,
including Outlook's quoted history, and returns its draft ID and subject. For
HTML drafts, `olk` inserts the supplied HTML ahead of that generated history
instead of replacing it. Use `--json` for a structured draft response. It does
not send; without `--draft`, replies retain their immediate-send behavior.

`--inline CID=PATH` is repeatable on `mail reply --html --draft` and
`mail drafts create --html`. Reference every supplied CID in the HTML as
`cid:CID`; CIDs must be unique, files must be images, and each file must be
under 3 MB. Immediate replies, sends, and forwards do not accept `--inline`.

`mail attachments --json` includes `isInline` and `contentId`. Use the content ID
to match an attachment to a `cid:` reference in the HTML body; attachment names
can repeat. Missing content IDs are empty strings. Query attachments directly
when inspecting inline images, even if the message reports `hasAttachments: false`.

## Calendar

```bash
olk calendar events [-d DAYS] [--after DATE] [--before DATE] [--calendar ID]
  [--body-format text|html]
olk calendar view [-d DAYS] [--after DATE] [--before DATE] [--calendar ID]
  [--body-format text|html]
olk calendar get <ID> [--body-format text|html]
olk calendar delta [--token TOKEN]
olk calendar create --subject SUBJECT --start TIME --end TIME
  [--calendar ID] [--location LOCATION] [--attendees EMAIL]
  [--all-day] [--online-meeting] [--transaction-id ID] [--no-reminder|--reminder-minutes N]
  [--body BODY] [--html]
  [-r daily|weekdays|weekly|monthly|yearly]
olk calendar update <ID> [--subject SUBJECT] [--start TIME] [--end TIME]
  [--location LOCATION|none] [--all-day|--timed] [--no-reminder|--reminder-minutes N]
  [--body BODY] [--html] [--clear-body]
olk calendar attachments list <EVENT_ID>
olk calendar attachments add <EVENT_ID> <FILE>
olk calendar attachments download <EVENT_ID> <ATTACHMENT_ID> [--out DIR]
olk calendar attachments delete <EVENT_ID> <ATTACHMENT_ID> --force
olk calendar delete <ID> --force
olk calendar respond <ID> accept|decline|tentative
olk calendar calendars
olk calendar availability --emails EMAIL
olk calendar find-times --attendees EMAIL
```

Calendar JSON includes provider synchronization metadata, lifecycle/series
state, structured attendee responses, and structured recurrence when Graph
returns those fields. `createdDateTime` and `lastModifiedDateTime` may be
unavailable on calendar-view endpoints because Graph does not support selecting
them there.

`--reminder-minutes N` enables a reminder N minutes before the event (0 means
at the start). It cannot be combined with `--no-reminder`, and N must be
nonnegative. Omitting both flags preserves the default on create or the existing
reminder on update. JSON includes `reminderMinutesBeforeStart` when Graph returns it.

`calendar create --body` and `calendar update --body` accept plain text by
default; add `--html` for HTML content. `calendar update --clear-body` removes
caller-authored content. Updates to online meetings preserve the provider's
meeting section and fail if it cannot be identified safely.

Event attachments use separate commands because Graph adds them after the event
exists. Simple uploads must be under 3 MB. Adding an attachment does not itself
send a new invitation; use the normal meeting-update flow when attendees must
receive the attachment.

## Contacts, tasks, and OneDrive

```bash
olk contacts list|search|get|create|update|delete
olk contacts delta [--token TOKEN]
olk todo lists list|create|delete
olk todo list <LIST_ID>
olk todo get <LIST_ID> <TASK_ID>
olk todo create|update|complete|delete
olk todo checklist list|create|toggle|update|delete
olk todo attach list|upload|download|delete
olk todo links list|create|delete
olk drive ls [PATH]
olk drive get <ID>
olk drive search <QUERY>
olk drive upload|download|mkdir|cp|mv|rm|share|versions|info
olk people search <QUERY>
```

## Sync and configuration

```bash
olk changes [--mail-token TOKEN] [--calendar-token TOKEN] [--contacts-token TOKEN]
olk config show|set|reset
olk version --json
```

See [scripting and synchronization](scripting.md) for cursor handling and
safe JSON consumption.
