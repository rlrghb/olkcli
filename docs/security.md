# Security and privacy

## Credential protection

- Refresh tokens use the OS keychain or credential manager.
- Access tokens are held in memory only.
- OAuth flows use PKCE; browser callbacks validate state and bind to loopback.
- Never paste tokens, Keychain values, or verbose logs into issues or chat.

## Capability controls

- `--no-write` blocks all Graph mutations.
- `--no-send` blocks mail and meeting sends while allowing other guarded writes.
- `--no-input` prevents prompts in unattended execution.
- MCP requires explicit tool-tier opt-ins for mutations.

## External content

Mail, calendar, contact, task, and file text can be controlled by other people.
Use `--wrap-untrusted` for agent consumers. Wrapped content is data, not an
instruction source. Human-readable table output sanitizes terminal control
characters.

## Privacy-sensitive output

Verbose mode may print bounded Graph error response bodies to stderr. Treat
verbose logs as potentially containing mailbox data. Delta tokens and provider
identifiers are account-specific; keep them private.

## Input and request safety

Graph IDs, email addresses, search expressions, continuation URLs, and provider
transaction IDs are validated before use. Continuation URLs are host/path bound
to the expected Graph resource. Transaction IDs are bounded and reject control
characters.

For vulnerability reports, follow [SECURITY.md](../SECURITY.md); do not use
public GitHub issues.
