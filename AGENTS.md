# Repository Guidelines

## Project Structure

- `cmd/olk/`: CLI entrypoint — minimal, delegates to `internal/cmd.Execute()`.
- `internal/cmd/`: Command implementations using kong structs. Each command group has its own file(s): `mail_*.go`, `calendar*.go`, `todo.go`, `todo_checklist.go`, `todo_attachments.go`, `todo_links.go`, `whoami.go`, etc.
- `internal/msauth/`: Microsoft OAuth2 implementation — device code flow, token refresh, credential bridge.
- `internal/graphapi/`: Microsoft Graph API wrapper — mail, calendar, contacts, todo (tasks, checklist items, attachments, linked resources), availability, mailbox settings, mail rules, people. Includes the `targetUser` helper that routes reads to `/me` or `/users/{id}` for delegated mailbox access, and error-shaping helpers (`graphErrorMessage`, `enterpriseError`, `scopeUpgradeError`) in `validate.go`.
- `internal/config/`: Configuration and XDG paths (`~/.config/olk/`).
- `internal/secrets/`: OS keyring integration via `99designs/keyring`.
- `internal/outfmt/`: Output formatting — JSON envelope, aligned tables, TSV, timezone conversion via `ConvertTime()`.
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

- Formatting: `gofmt` + `goimports` with local prefix `github.com/rlrghb/olkcli` (configured in `.golangci.yml`).
- Output: keep stdout parseable (`--json` / `--plain`); send human hints/progress to stderr.
- Graph API pointer types: always nil-check before dereferencing (`if x.GetFoo() != nil`).
- Kong commands: one struct per command, `Run(ctx *RunContext) error` method.
- File naming: `mail_list.go`, `mail_get.go` etc. for individual subcommands; `mail.go` for the parent struct.

## Testing Guidelines

- Unit tests: stdlib `testing` package. Test files go next to the code they test (`*_test.go`).
- Existing coverage: `internal/outfmt` (formatting + untrusted-wrapping), `internal/config`, `internal/msauth/scopes`, `internal/graphapi` (validate + capability guards), and `internal/cmd` (paging filters + mailbox-target validation, MCP server/registry, schema generation, argv building, output capture). Coverage is partial — many command and Graph-wrapper paths remain untested.
- Integration tests require a valid OAuth token + live Graph access — run manually, not in CI.
- **macOS validation:** each `make build` produces a fresh (ad-hoc) binary identity, so the first run of the new `./bin/olk` that reads stored tokens triggers a macOS Keychain access prompt — a human must click **"Always Allow"**. An automated/agent run can't dismiss the dialog, so a hang on the first post-build command is expected (not a code bug); surface it for manual approval.
- New tests should run cleanly under `go test -race -count=1 ./...` and pass `golangci-lint run`.

## CI

- `.github/workflows/ci.yml` runs on every pull request and push to `main`.
- The `test` job runs `go mod tidy` drift check → `go vet ./...` → `go build ./...` → `go test -race -count=1 ./...` on Ubuntu using the Go version pinned in `go.mod`.
- The `lint` job runs `golangci-lint` v2.5.0 against `.golangci.yml`.
- Both jobs use pinned action SHAs to match `release.yml` style.

## Release & Distribution

- A `vX.Y.Z` tag triggers `release.yml`, publishing to three channels: **Homebrew** (goreleaser → `rlrghb/tap` cask, with a quarantine-strip postflight), **npm** (the `olkcli` package + six `olk-<os>-<arch>` per-platform packages under `npm/`; `scripts/build-npm.mjs` stamps + publishes), and the **MCP Registry** (`server.json` → `io.github.rlrghb/outlook`, published via `mcp-publisher`).
- **No publish secrets:** npm uses **Trusted Publishing (OIDC)** (`id-token: write`, npm ≥ 11.5.1, SLSA provenance); the registry uses GitHub OIDC. The npm package is `olkcli`; the binary is `olk`.
- The official MCP registry has no in-place edit — `server.json` description/version changes apply on the **next release** (versions are CI-stamped from the tag). `npm-publish`/`registry-publish` are gated on the `PUBLISH_NPM` repo variable.
- **ClawHub (OpenClaw skill) — manual, separate from the tag pipeline.** olk is listed on ClawHub as a **skill** from `SKILL.md`. Publish with the `clawhub` CLI (publisher `rlrghb`; `clawhub whoami` / `clawhub login`) from a folder containing **only `SKILL.md`** — `mkdir -p /tmp/olk-skill && cp SKILL.md /tmp/olk-skill/`, then `clawhub skill publish /tmp/olk-skill --slug olk --name Outlook --version <X.Y.Z> --tags calendar,contacts,drive,latest,mail,microsoft,onedrive,outlook,tasks --changelog '…'`. The skill version is **independent of the binary** — align it to the release. Display name is `Outlook` (pass `--name`; `SKILL.md`'s `name: olk` is only the slug). The summary comes from `SKILL.md` `description:`; **category** ("DATA & APIS") is web-UI only. **No `--dry-run`** — verify the live entry with `clawhub inspect olk` and confirm before publishing.

## Key Design Decisions

- **Raw OAuth2**: Uses `net/http` directly against Microsoft's OAuth2 endpoints (no MSAL dependency). Refresh tokens stored in OS keyring.
- **Graph SDK**: Uses official `msgraph-sdk-go` for type safety despite verbose pointer types — wrapped in `graphapi/` layer.
- **Embedded Client ID**: `51e726d0-22a4-45f7-a71c-b472ff84c027`. Overridable via `--client-id` / `OLK_CLIENT_ID`.
- **Tenant `common`**: Default tenant accepts both personal and enterprise accounts.
- **Lazy client init**: `RunContext.GraphClient()` initializes on first call — auth commands don't need a Graph client.
- **Delegated mailbox routing**: read paths in `internal/graphapi/{mail,calendar,contacts}.go` take a `target string` first parameter and route through `c.targetUser(target)`. Empty target preserves `/me` behavior; a non-empty value hits `/users/{target}/…`. The CLI exposes this as the global `--mailbox` flag (env `OLK_MAILBOX`), validated once via `resolveMailboxTarget` in `internal/cmd/paging.go`. New read methods should follow the same shape; write paths intentionally stay on `/me` for now.
- **MCP server**: `olk mcp` (`internal/cmd/mcp*.go`) exposes a curated, read-first allowlist of tools over stdio — not the whole CLI. Tool calls reparse argv and run in-process with stdout captured. Read-only by default; `--allow-write <tool>` exposes a named curated safe-write tool (per-tool opt-in). No HTTP transport (deliberate scope choice). To expose a command, add it to `curatedTools` (read or non-destructive write only).
- **Capability guards**: `--no-write`/`--no-send` enforced once at the `graphapi.Client` layer so the guarantee covers CLI, MCP, and scripts; command allow/deny lists (`--enable-commands[-exact]`, `--disable-commands`) gate dispatch via `commandAllowed()`. `--wrap-untrusted` (forced on under MCP) wraps `untrusted:"true"`-tagged fields for prompt-injection defense.

## Commit & Pull Request Guidelines

- Follow Conventional Commits (e.g. `feat(mail): add --attach flag to send`).
- Group related changes; avoid bundling unrelated refactors.
- PRs should summarize scope, note testing performed, and mention user-facing changes.
- Attribution: when adapting or reworking someone else's PR/patch, keep them as a `Co-Authored-By:` trailer on the commit even if the code was substantially rewritten.

### Review vs. Land

- **Review mode is read-only.** Inspect with `gh pr view <n>` / `gh pr diff <n>`; don't push to a contributor's branch while reviewing.
- **Land mode**: never commit directly to `main` — branch, open a PR, and **wait for both CI jobs (`test`, `lint`) to pass** before merging. Merge with **squash** and delete the branch. Sync `main` and prune afterward.
- Verify behavior before landing user-facing changes (run the binary / a live smoke test), not just unit tests.

## Security & Configuration

- Never commit OAuth tokens or client secrets.
- Prefer OS keychain backends; the file fallback is for headless environments only. Set `OLK_KEYRING_PASSWORD` for non-interactive file-backend access.
- Config dir (`~/.config/olk/`) uses 0700 permissions; token files use 0600.
- Device code flow uses PKCE (RFC 7636) — `code_challenge` sent with device code request, `code_verifier` sent during token polling.
- Token refresh is serialized per-email via `sync.Map` of mutexes in `internal/msauth/auth.go` to prevent race conditions.
- KQL search queries are always wrapped in double quotes (Graph `$search` parameter requirement). Property restrictions (`from:`, `subject:`, etc.) and boolean operators work inside the quotes. Literal double-quote characters are stripped from user input to prevent breaking the wrapper. See `internal/graphapi/mail.go`.
- See `SECURITY.md` for vulnerability disclosure policy.
