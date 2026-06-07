# CLAUDE.md

This file provides context for Claude Code when working on the olk project.

## What is this project?

`olk` is a CLI tool for Microsoft Outlook and OneDrive via the Microsoft Graph API. It provides terminal access to email, calendar, contacts, tasks, and OneDrive files for both personal Microsoft accounts and enterprise Azure AD/Entra ID accounts.

## Quick Reference

```bash
make build          # Build binary to ./bin/olk
make test           # Run tests
make lint           # Lint with golangci-lint
go mod tidy         # After changing dependencies
```

## Architecture

- **CLI framework**: `github.com/alecthomas/kong` — commands are Go structs with `Run(ctx *RunContext) error`
- **Auth**: Raw OAuth2 device code flow with PKCE (RFC 7636) against `login.microsoftonline.com` — no MSAL. Scopes defined in `internal/msauth/scopes.go`. Enterprise-only scopes (`MailboxSettings.ReadWrite`, `User.ReadBasic.All`) are only requested with `--enterprise` flag — personal accounts cannot consent to them. `auth login --scope <s>` (repeatable) layers extra scopes (e.g. `Mail.Read.Shared`) onto the default set via `MergeScopes` (case-insensitive dedup). Token refresh is serialized per-email via `sync.Map` of mutexes to prevent race conditions
- **API**: Official `msgraph-sdk-go` wrapped in `internal/graphapi/` for ergonomic access
- **Secrets**: OS keyring via `github.com/99designs/keyring` (macOS Keychain, Linux Secret Service, Windows WinCred). File-backend password prompt writes to stderr (not stdout) to avoid corrupting piped output. Set `OLK_KEYRING_PASSWORD` for headless/non-interactive use
- **Output**: JSON envelope (`--json`), aligned table (default), TSV (`--plain`)
- **Timezone**: Display-layer conversion via `outfmt.ConvertTime()`. Resolved once per command via `RunContext.Timezone()` (flag > env > config > Local). JSON output emits UTC timestamps as RFC3339 with a `Z` suffix (normalized via `normalizeGraphUTC` — Graph's `DateTimeTimeZone.dateTime` strings lack a zone); envelope includes `timezone` field. IANA db embedded via `import _ "time/tzdata"`

## Key Patterns

- `RunContext` (in `internal/cmd/root.go`) lazily initializes the Graph client — auth commands skip it
- Graph SDK uses pointer types everywhere — always nil-check: `if x.GetFoo() != nil { *x.GetFoo() }`
- Each command is in its own file: `mail_list.go`, `mail_get.go`, etc.
- Desire paths in `desire_paths.go` delegate to real commands (e.g. `SendCmd` creates `MailSendCmd`)
- Config lives at `~/.config/olk/`, tokens in OS keyring keyed by `olk:token:<email>`
- **Delegated mailbox access** (`--mailbox <email>` / `OLK_MAILBOX`): read paths can target another user's mailbox. Commands resolve and validate the flag via `resolveMailboxTarget(ctx.Flags.Mailbox)` (in `paging.go`), then pass the resulting `target string` as the first arg to graphapi methods. `Client.targetUser(target)` (in `graphapi/client.go`) routes to `Me()` when empty or `Users().ByUserId(target)` otherwise — both return the same `*UserItemRequestBuilder`, so chained calls are identical. Requires the `*.Shared` scope (e.g. `Mail.Read.Shared`) granted at login. Only wired for read paths, not writes
- **MCP server** (`olk mcp`, in package `cmd`): lives in `cmd` — not a separate package — because it must introspect the kong grammar, re-parse an argv into a `kong.Context`, and run commands with a `RunContext` (all `cmd`-internal); a separate package would create an import cycle. `buildMCPServer(profile)` (`mcp_server.go`) walks the kong model (`kong.New(&CLI{}).Model`) and auto-registers one MCP tool per leaf command, with the input schema derived from each command's flags/positionals. The handler (`mcp_invoke.go`) rebuilds an argv (`[path…, --json, flags…, --, positionals…]`), reparses with a fresh `CLI`, runs it, and returns captured output. `captureStd` (`mcp_capture.go`) redirects `os.Stdout`/`os.Stderr` to pipes under a global mutex (so command output — 221 direct `os.Stdout` writes plus the Printer — never corrupts the MCP transport's own stdout); this makes tool calls single-flight. Profiles `safe`/`full` are classified in `mcp_profiles.go` by the leaf command's final path token. Uses `github.com/modelcontextprotocol/go-sdk` + `github.com/google/jsonschema-go`

## Common Tasks

### Adding a new mail subcommand
1. Create `internal/cmd/mail_<name>.go` with the command struct and `Run` method
2. Add the struct to `MailCmd` in `internal/cmd/mail.go`
3. If needed, add the API method to `internal/graphapi/mail.go`

### Adding a new calendar subcommand
1. Create `internal/cmd/calendar_<name>.go` with the command struct and `Run` method
2. Add the struct to `CalendarCmd` in `internal/cmd/calendar.go`
3. If needed, add the API method to `internal/graphapi/calendar.go`

### Adding a new people subcommand
1. Create `internal/cmd/people_<name>.go` or add to `internal/cmd/people.go`
2. Add the struct to `PeopleCmd` in `internal/cmd/people.go`
3. If needed, add the API method to `internal/graphapi/people.go`

### Adding a new todo subcommand
1. Create `internal/cmd/todo_<name>.go` or add to `internal/cmd/todo.go`
2. Add the struct to `TodoCmd` in `internal/cmd/todo.go`
3. If needed, add the API method to `internal/graphapi/todo.go`

### Adding a new drive subcommand
1. Create `internal/cmd/drive_<name>.go` with the command struct and `Run` method
2. Add the struct to `DriveCmd` in `internal/cmd/drive.go`
3. If needed, add the API method to `internal/graphapi/drive.go`

### Supporting `--mailbox` in a new read command
graphapi read methods take a `target string` first param threaded to `targetUser(target)`. In the command's `Run`, resolve it once with `target, err := resolveMailboxTarget(ctx.Flags.Mailbox)` and pass it through. Pass `""` for write paths (delegation is read-only by design).

### Adding a new flag to all commands
Add it to `RootFlags` in `internal/cmd/root.go` with `env:"OLK_*"` tag.

### Adding timezone conversion to a new command
1. Get the location: `loc, _ := ctx.Timezone()`
2. Wrap time fields: `outfmt.ConvertTime(field, loc)`
3. Only convert for table/plain output — JSON keeps RFC3339 UTC strings (`...Z`). When pulling a value from Graph's `DateTimeTimeZone.GetDateTime()` into a JSON-tagged field, wrap the deref with `normalizeGraphUTC(...)` so the emitted string has a zone suffix.

### Changing Graph API calls
Edit files in `internal/graphapi/` — these wrap the verbose SDK calls into simple methods returning plain structs.

### MCP tools (auto-generated)
Any new leaf command is automatically exposed as an MCP tool — no per-tool wiring. It is classified by its final path token in `internal/cmd/mcp_profiles.go`: `delete`/`rm`/`clean`/`logout` ⇒ destructive (`full` profile only); a read verb (`list`/`get`/`search`/…) ⇒ read; otherwise write. If you add a command with a new destructive verb or an ambiguous name, add it to `destructiveVerbs`/`readVerbs` or `pathOverrides`, or the `TestSafeProfileHasNoDestructiveLeaf` guard test will catch the gap. Interactive commands (only `auth login` today) and the `mcp` command are excluded.

## Dependencies

The project uses `msgraph-sdk-go` v1.96.0 which has some naming quirks:
- Attendee type uses `SetTypeEscaped()` not `SetType()` (Go keyword collision)
- Contact emails use `models.NewEmailAddress()` not `NewTypedEmailAddress()` — supports multiple emails as `[]EmailAddressable`
- Contact phones: `GetBusinessPhones()`, `GetHomePhones()`, `GetMobilePhone()` (no unified `GetPhones()`)
- Contact addresses: `GetBusinessAddress()`, `GetHomeAddress()`, `GetOtherAddress()` return `PhysicalAddressable`; use `models.NewPhysicalAddress()` to create
- Contact birthday: `GetBirthday()` / `SetBirthday()` takes `*time.Time`
- Message item request builders: `ItemMessagesMessageItemRequestBuilder*` (note double "Messages")
- Message rules: `Me().MailFolders().ByMailFolderId("inbox").MessageRules()` for CRUD; requires `MailboxSettings.ReadWrite` scope
- People API: `Me().People()` with `$search` query parameter; falls back to `/users` directory search (requires `ConsistencyLevel: eventual` header) when People API returns empty
- Message rules: `SetSequence()` must be >= 1 (Graph API rejects 0)
- FindMeetingTimes: `Me().FindMeetingTimes().Post()` returns `MeetingTimeSuggestionsResultable`
- Recurrence pattern: `event.GetRecurrence().GetPattern().GetTypeEscaped()` (uses `GetTypeEscaped` not `GetType`)
- ISODuration: use `serialization.NewDuration()` from `kiota-abstractions-go` for meeting duration
- Todo checklist items: `Me().Todo().Lists().ByTodoTaskListId(listID).Tasks().ByTodoTaskId(taskID).ChecklistItems()`
- Todo attachments: `TaskFileAttachment` type for upload; `ByAttachmentBaseId()` for get/delete
- Todo linked resources: `Me().Todo().Lists().ByTodoTaskListId(listID).Tasks().ByTodoTaskId(taskID).LinkedResources()`
- Drive: `Me().Drive()` for default drive, `Me().Drives()` for all drives, `Drives().ByDriveId(id)` for specific drive
- DriveItems: `Drives().ByDriveId(id).Items().ByDriveItemId(itemID)` for item operations; `.Children()` for folder contents; `.Content()` for file download/upload

MCP server deps:
- `github.com/modelcontextprotocol/go-sdk` v1.2.0 — `mcp.NewServer` / `Server.AddTool(*Tool, ToolHandler)` (raw handler, since schemas are built dynamically); `mcp.StdioTransport{}` for stdio; `mcp.NewStreamableHTTPHandler` for HTTP; `mcp.NewInMemoryTransports()` for tests. `ToolHandler = func(ctx, *CallToolRequest)(*CallToolResult, error)`; read args from `req.Params.Arguments` (a `json.RawMessage`)
- `github.com/google/jsonschema-go` — `jsonschema.Schema{Type, Properties, Required, Enum, Items, Description}` for tool input schemas
- Drive path-based access requires raw URL builders: `drives.NewItemItemsDriveItemItemRequestBuilder(rawURL, c.inner.GetAdapter())` with URL pattern `/drives/{id}/root:/{path}:`
- Drive sharing: `CreateLink().Post()` body uses `SetTypeEscaped()` not `SetType()` (same Go keyword collision as Attendee)
