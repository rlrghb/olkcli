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
olk mail move <ID> ID_OR_PATH
olk mail delete <ID> --force
olk mail attachments <ID> --attachment-id ID
olk mail folders list|create|rename|delete  # list traverses visible child folders
olk mail drafts list|create|send|delete
olk mail drafts create --to EMAIL --subject SUBJECT --body '<img src="cid:logo">' --html --inline logo=logo.png
olk mail flag <ID> flagged|complete|notFlagged
olk mail categorize <ID> --category NAME
olk mail importance <ID> low|normal|high
olk mail ooo get|set|off
olk mail rules list|create|delete
```

`mail reply --draft` creates a true threaded Outlook reply or reply-all draft,
including Outlook's quoted history, and returns its draft ID and subject. For
HTML drafts, `olk` inserts the supplied HTML ahead of that generated history
instead of replacing it. It does not send; without `--draft`, replies retain
their immediate-send behavior.

`--inline CID=PATH` is repeatable on `mail reply --html --draft` and
`mail drafts create --html`. Reference every supplied CID in the HTML as
`cid:CID`; CIDs must be unique, files must be images, and each file must be
under 3 MB. Immediate replies, sends, and forwards do not accept `--inline`.

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
  [--all-day] [--online-meeting] [--transaction-id ID] [--no-reminder]
  [-r daily|weekdays|weekly|monthly|yearly]
olk calendar update <ID> [--subject SUBJECT] [--start TIME] [--end TIME]
  [--location LOCATION|none] [--all-day|--timed] [--no-reminder]
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
