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

`make build` produces `bin/olk` in the `olk-dev` storage namespace, with its
own config directory and credential-store entries; sign in once with
`./bin/olk auth login`. `make install` and release builds use `olk`. Override
with `make build NAMESPACE=<name>`.

### Signing development builds on macOS

`go build` gives each binary an ad-hoc signature that changes on every build,
so macOS asks for Keychain access again after each rebuild. Signing every
build with the same certificate and identifier keeps one **Always Allow**
grant for all of them:

```bash
scripts/macos-dev-cert.sh                     # once: creates olk-dev-signer
OLK_CODESIGN_IDENTITY=olk-dev-signer make build sign
```

`OLK_CODESIGN_IDENTITY` takes the name or SHA-1 hash of any code-signing
identity in your keychains, including a Developer ID certificate; any
environment manager can set it. `OLK_CODESIGN_IDENTIFIER` overrides the default
identifier, `com.rlrghb.olk.dev`. Local builds need no notarization, because
macOS checks it only for downloaded files. `make sign` refuses a binary that
does not report a development namespace, so a release build is never re-signed
with a local certificate.

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
