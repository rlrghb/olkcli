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

## CI

Pull requests run module tidy checks, vet, build, race tests, and
golangci-lint. The lint workflow currently uses golangci-lint v2.11.4.

## Releases

A `vX.Y.Z` tag publishes Homebrew, npm, and the MCP Registry through the
release workflow. npm and registry publishing use OIDC rather than long-lived
publish tokens. The ClawHub skill is published separately from a directory
containing only `SKILL.md`.

Never commit OAuth tokens, client secrets, generated binaries, or account data.
