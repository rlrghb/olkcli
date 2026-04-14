# Repository Guidelines

## Project Structure

- `cmd/olk/`: CLI entrypoint — minimal, delegates to `internal/cmd.Execute()`.
- `internal/cmd/`: Command implementations using kong structs. Each command group has its own file(s): `mail_*.go`, `calendar*.go`, `todo.go`, `todo_checklist.go`, `todo_attachments.go`, `todo_links.go`, `whoami.go`, etc.
- `internal/msauth/`: Microsoft OAuth2 implementation — device code flow, token refresh, credential bridge.
- `internal/graphapi/`: Microsoft Graph API wrapper — mail, calendar, contacts, todo (tasks, checklist items, attachments, linked resources), availability, mailbox settings, mail rules, people.
- `internal/config/`: Configuration and XDG paths (`~/.config/olk/`).
- `internal/secrets/`: OS keyring integration via `99designs/keyring`.
- `internal/outfmt/`: Output formatting — JSON envelope, aligned tables, TSV, timezone conversion via `ConvertTime()`.
- `internal/errfmt/`: Graph API error mapping to actionable user messages.
- `SKILL.md`: [Agent Skills](https://agentskills.io) standard file — teaches AI assistants (Claude Code, OpenClaw, etc.) how to use `olk` commands.
- `bin/`: build outputs (gitignored).

## Build, Test, and Development Commands

- `make build`: build `bin/olk` with version ldflags.
- `make test`: run tests with race detector.
- `make lint`: run `golangci-lint`.
- `make install`: build and copy to `$GOPATH/bin`.
- `make clean`: remove `bin/`.
- `make version`: print current version/commit/date.

## Coding Style & Naming Conventions

- Formatting: `goimports` with local prefix `github.com/rlrghb/olkcli` + `gofumpt`.
- Output: keep stdout parseable (`--json` / `--plain`); send human hints/progress to stderr.
- Graph API pointer types: always nil-check before dereferencing (`if x.GetFoo() != nil`).
- Kong commands: one struct per command, `Run(ctx *RunContext) error` method.
- File naming: `mail_list.go`, `mail_get.go` etc. for individual subcommands; `mail.go` for the parent struct.

## Testing Guidelines

- Unit tests: stdlib `testing` package.
- Integration tests require a valid OAuth token — run manually, not in CI.
- Test files go next to the code they test (`*_test.go`).

## Key Design Decisions

- **Raw OAuth2**: Uses `net/http` directly against Microsoft's OAuth2 endpoints (no MSAL dependency). Refresh tokens stored in OS keyring.
- **Graph SDK**: Uses official `msgraph-sdk-go` for type safety despite verbose pointer types — wrapped in `graphapi/` layer.
- **Embedded Client ID**: `51e726d0-22a4-45f7-a71c-b472ff84c027`. Overridable via `--client-id` / `OLK_CLIENT_ID`.
- **Tenant `common`**: Default tenant accepts both personal and enterprise accounts.
- **Lazy client init**: `RunContext.GraphClient()` initializes on first call — auth commands don't need a Graph client.

## Commit & Pull Request Guidelines

- Follow Conventional Commits (e.g. `feat(mail): add --attach flag to send`).
- Group related changes; avoid bundling unrelated refactors.
- PRs should summarize scope, note testing performed, and mention user-facing changes.

## Security & Configuration

- Never commit OAuth tokens or client secrets.
- Prefer OS keychain backends; the file fallback is for headless environments only.
- Config dir (`~/.config/olk/`) uses 0700 permissions; token files use 0600.
- Device code flow uses PKCE (RFC 7636) — `code_challenge` sent with device code request, `code_verifier` sent during token polling.
- Token refresh is serialized per-email via `sync.Map` of mutexes in `internal/msauth/auth.go` to prevent race conditions.
- KQL search queries are always wrapped in double quotes (Graph `$search` parameter requirement). Property restrictions (`from:`, `subject:`, etc.) and boolean operators work inside the quotes. Literal double-quote characters are stripped from user input to prevent breaking the wrapper. See `internal/graphapi/mail.go`.
- See `SECURITY.md` for vulnerability disclosure policy.
