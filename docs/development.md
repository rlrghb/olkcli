# Development and releases

## Repository layout

- `cmd/olk/` — minimal CLI entrypoint
- `internal/cmd/` — Kong command definitions
- `internal/graphapi/` — Microsoft Graph wrapper and output models
- `internal/msauth/` — OAuth and token lifecycle
- `internal/outfmt/` — JSON, TSV, table, timezone, and untrusted wrapping
- `internal/secrets/` — OS keychain integrations

## Validation

```bash
make build
make test
make lint
go vet ./...
go mod verify
```

New tests should pass `go test -race -count=1 ./...`. Graph-wrapper changes
should include fixture tests for request projections and converted output.

### Delegated-mailbox changes

Whether a delegated send succeeds is decided by Exchange permissions and token
scopes rather than by anything in this repository, and where the sent copy is
filed is decided by a mailbox setting on the tenant, so the `--mailbox` write
paths cannot be covered by unit tests. `contrib/smoke-delegated-mailbox/` is a
live-tenant harness for them. It needs an enterprise account with a shared
mailbox delegated to it, and it sends real mail, so every send asks first. Its
read-only preflight is worth running on its own.

## CI

Pull requests run module tidy checks, vet, build, race tests, and
golangci-lint. The lint workflow currently uses golangci-lint v2.11.4.

## Releases

A `vX.Y.Z` tag publishes Homebrew, npm, and the MCP Registry through the
release workflow. npm and registry publishing use OIDC rather than long-lived
publish tokens. The ClawHub skill is published separately from a directory
containing only `SKILL.md`.

Never commit OAuth tokens, client secrets, generated binaries, or account data.
