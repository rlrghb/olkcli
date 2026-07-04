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

> **Validating on macOS:** running a freshly built `./bin/olk` against a real account triggers a macOS Keychain access prompt (each build is a new identity). A human must click **"Always Allow"** — you (an agent) can't dismiss the dialog, so don't treat a first-run hang as a bug; ask the user to approve it.

## Architecture

- **CLI framework**: `github.com/alecthomas/kong` — commands are Go structs with `Run(ctx *RunContext) error`
- **Auth**: Raw OAuth2 against `login.microsoftonline.com` — no MSAL. Two flows: device code (default) and authorization-code + PKCE via system browser with a loopback listener (`--browser`, in `internal/msauth/authcode.go`) for tenants whose Conditional Access requires a compliant device or blocks device code. Both use PKCE (RFC 7636). Scopes defined in `internal/msauth/scopes.go`. Enterprise-only scopes (`MailboxSettings.ReadWrite`, `User.ReadBasic.All`) are only requested with `--enterprise` flag — personal accounts cannot consent to them. Token refresh is serialized per-email via `sync.Map` of mutexes to prevent race conditions
- **API**: Official `msgraph-sdk-go` wrapped in `internal/graphapi/` for ergonomic access
- **Secrets**: OS keyring via `github.com/99designs/keyring` (macOS Keychain, Linux Secret Service, Windows WinCred). File-backend password prompt writes to stderr (not stdout) to avoid corrupting piped output. Set `OLK_KEYRING_PASSWORD` for headless/non-interactive use
- **Output**: JSON envelope (`--json`), aligned table (default), TSV (`--plain`)
- **MCP server**: `olk mcp` (in `internal/cmd/mcp*.go`) runs a stdio Model Context Protocol server exposing a **curated** allowlist of read-first tools (`curatedTools` in `mcp_server.go`) — NOT the whole command tree. Tool calls reparse a rebuilt argv and run in-process with stdout captured under a mutex (`mcp_capture.go`). Read-only by default; `--allow-write <tool>` exposes a named curated safe-write tool (per-tool opt-in). No HTTP transport
- **Capability guards**: `--no-write`/`--no-send` are enforced once at the `graphapi.Client` layer (`ensureWritable`/`ensureMaySend`), so the guarantee holds across CLI, MCP, and scripts. `--enable-commands[-exact]`/`--disable-commands` gate dispatch via `commandAllowed()` (`commands.go`), checked in `Execute()` and reused to filter the MCP registry. `--wrap-untrusted` wraps `untrusted:"true"`-tagged struct fields in JSON/plain output (`internal/outfmt/untrusted.go`)
- **Timezone**: Display-layer conversion via `outfmt.ConvertTime()`. Resolved once per command via `RunContext.Timezone()` (flag > env > config > Local). JSON output emits UTC timestamps as RFC3339 with a `Z` suffix (normalized via `normalizeGraphUTC` — Graph's `DateTimeTimeZone.dateTime` strings lack a zone); envelope includes `timezone` field. IANA db embedded via `import _ "time/tzdata"`

## Key Patterns

- `RunContext` (in `internal/cmd/root.go`) lazily initializes the Graph client — auth commands skip it
- Graph SDK uses pointer types everywhere — always nil-check: `if x.GetFoo() != nil { *x.GetFoo() }`
- Each command is in its own file: `mail_list.go`, `mail_get.go`, etc.
- Desire paths in `desire_paths.go` delegate to real commands (e.g. `SendCmd` creates `MailSendCmd`)
- Config lives under `os.UserConfigDir()` (`~/.config/olk/` on Linux, `~/Library/Application Support/olk/` on macOS), tokens in OS keyring keyed by `olk:token:<email>`

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

### Adding a new flag to all commands
Add it to `RootFlags` in `internal/cmd/root.go` with `env:"OLK_*"` tag.

### Exposing a command as an MCP tool
Add an entry to `curatedTools` in `internal/cmd/mcp_server.go` (`{name, path, write}`). Only do this for safe read or non-destructive/non-send write commands — destructive and send commands must never be added. A test (`TestCuratedToolsResolve`) fails if the path doesn't resolve; another (`TestCuratedRegistry_NoDestructiveOrSend`) fails if a forbidden verb sneaks in.

### Marking a field as agent-untrusted
Add `untrusted:"true"` to the struct tag of any externally-controlled free-text field on a `graphapi` result struct. `--wrap-untrusted` (forced on under MCP) wraps it in markers.

### Adding timezone conversion to a new command
1. Get the location: `loc, _ := ctx.Timezone()`
2. Wrap time fields: `outfmt.ConvertTime(field, loc)`
3. Only convert for table/plain output — JSON keeps RFC3339 UTC strings (`...Z`). When pulling a value from Graph's `DateTimeTimeZone.GetDateTime()` into a JSON-tagged field, wrap the deref with `normalizeGraphUTC(...)` so the emitted string has a zone suffix.

### Changing Graph API calls
Edit files in `internal/graphapi/` — these wrap the verbose SDK calls into simple methods returning plain structs.

## Release & distribution

Pushing a `vX.Y.Z` tag triggers `.github/workflows/release.yml`, which fans out to three channels:

- **Homebrew** — goreleaser builds the binaries and updates the `rlrghb/tap` cask (`homebrew_casks` in `.goreleaser.yaml`); a `hooks.post.install` strips the macOS quarantine xattr so the unsigned binary launches.
- **npm** — the CLI also ships as an npm package for `npx`/cross-platform installs. `npm/olk/` is the main package **`olkcli`** (an esbuild-style launcher `bin/olk.js` that execs the matching `npm/olk-<os>-<arch>/` per-platform binary via `optionalDependencies`). `scripts/build-npm.mjs` stamps the tag version across all 7 packages and publishes them (idempotent — skips already-published versions). Publishing uses **npm Trusted Publishing (OIDC)** — no stored token: the `npm-publish` job has `id-token: write`, upgrades npm to ≥ 11.5.1, and each package has a trusted publisher configured (this repo + `release.yml`); every package gets a SLSA provenance attestation. The npm package is `olkcli`; the installed binary is `olk`.
- **MCP Registry** — `server.json` (name `io.github.rlrghb/outlook`, `registryType: npm` → `olkcli`, `packageArguments: [{positional "mcp"}]`) is published by the `registry-publish` job via `mcp-publisher` using GitHub OIDC (no secret). `version` fields are placeholders CI stamps from the tag; the `mcpName` in `npm/olk/package.json` must equal the `server.json` name. The official registry has **no in-place edit**, so `server.json` description/metadata changes only take effect on the **next release**.

Both `npm-publish` and `registry-publish` are gated on the `PUBLISH_NPM` repo variable. There are no long-lived publish secrets — npm and the registry both authenticate via OIDC.

### ClawHub (OpenClaw skill) — manual publish

olk is also listed on **ClawHub** (for the OpenClaw assistant) as a **skill** built from `SKILL.md`. This is a **separate, manual** step — it is **not** part of the tag-triggered `release.yml` pipeline. Update it after a release so the skill tracks the binary.

- **CLI / auth:** `clawhub` (installed via Homebrew). Publisher is `rlrghb`; check `clawhub whoami` (run `clawhub login` if the token is missing — interactive browser login).
- **Always publish from a folder containing only `SKILL.md`** (never the repo root):
  ```bash
  mkdir -p /tmp/olk-skill && cp SKILL.md /tmp/olk-skill/
  clawhub skill publish /tmp/olk-skill --slug olk --name Outlook --version <X.Y.Z> \
    --tags calendar,contacts,drive,latest,mail,microsoft,onedrive,outlook,tasks \
    --changelog '<summary of changes>'
  ```
- **slug** is `olk`; **display name** is `Outlook` — pass `--name Outlook` explicitly. (`SKILL.md`'s `name: olk` is only the slug; omitting `--name` would *rename* the live entry from "Outlook" to "olk".)
- The skill **version is independent** of the binary/git tag — align it to the released binary version (e.g. `1.9.6`).
- **Re-publish all tags** (the 9 above) so every topic tag moves to the new version; the default `--tags latest` leaves the others stale on the old version.
- The clawhub **summary** is the `SKILL.md` frontmatter `description:` — keep it in sync and commit description edits to the repo.
- **Category** (e.g. "DATA & APIS") is set in the clawhub **web UI** only — there is no `--category` flag/field, and `clawhub inspect` doesn't show it.
- There is **no `--dry-run`** for `skill publish` (that's `package publish` only) — the run is the real publish and `latest` moves immediately. **Verify the live entry first with `clawhub inspect olk`, confirm the exact command before running, then re-inspect to confirm.**

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
- Drive path-based access requires raw URL builders: `drives.NewItemItemsDriveItemItemRequestBuilder(rawURL, c.inner.GetAdapter())` with URL pattern `/drives/{id}/root:/{path}:`
- Drive sharing: `CreateLink().Post()` body uses `SetTypeEscaped()` not `SetType()` (same Go keyword collision as Attendee)
