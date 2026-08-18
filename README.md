# olk — Microsoft Outlook in Your Terminal

`olk` is a scriptable CLI and MCP server for Outlook mail, calendars, contacts,
tasks, and OneDrive through Microsoft Graph. It works with personal Outlook.com
accounts and enterprise Microsoft 365 accounts.

## Install

```bash
brew install rlrghb/tap/olk
# or
go install github.com/rlrghb/olkcli/cmd/olk@latest
```

The npm package is `olkcli`; the installed command is `olk`:

```bash
npm install -g olkcli
```

See [macOS notes](docs/authentication.md#macos-keychain) if macOS asks for
Keychain access after an upgrade.

## Quick start

```bash
olk auth login
olk mail list
olk mail get <message-id>
olk calendar events
olk calendar create --subject "Standup" \
  --start 2026-08-18T09:00 --end 2026-08-18T09:30
olk contacts list
```

For work/school accounts and enterprise-only features:

```bash
olk auth login --enterprise
```

## Common commands

| Need | Command |
| --- | --- |
| Search mail | `olk mail search "from:person@example.com subject:urgent"` |
| Send mail | `olk mail send --to person@example.com --subject Hi --body Hello` |
| Read calendar view | `olk calendar view --days 7` |
| Request text/HTML event bodies | `olk calendar events --body-format text` |
| Create retry-safe event | `olk calendar create ... --transaction-id ID` |
| Disable an event reminder | `olk calendar create ... --no-reminder` |
| Clear an event location | `olk calendar update <ID> --location none` |
| Convert event representation | `olk calendar update <ID> --all-day` or `--timed` |
| Synchronize changes | `olk changes --json` |
| Use JSON for scripts | `olk mail list --json --results-only` |
| Use as an MCP server | `olk mcp` |

See the [command reference](docs/commands.md) for all commands and flags.

## Output and scripting

Results are written to stdout; diagnostics and prompts go to stderr.

```bash
olk mail list --json --results-only | jq '.[].subject'
olk contacts list --plain
```

Output modes are JSON (`--json`), TSV (`--plain`), and aligned tables by
default. Use `--concise` to omit large bodies/previews and `--select` to limit
fields where supported. See [scripting and sync](docs/scripting.md).

## Safety defaults

- Use `--no-write --no-send --no-input` for unattended read-only runs.
- MCP exposes curated tools and requires explicit opt-in for mutations.
- External mail, calendar, contact, and file text can be wrapped with
  `--wrap-untrusted` for agent-safe consumption.
- Refresh tokens are stored in the OS keychain; access tokens are kept in
  memory only.

Read the [security and privacy guide](docs/security.md) before connecting an
agent or sharing logs.

## Documentation

- [Command reference](docs/commands.md)
- [Authentication and accounts](docs/authentication.md)
- [Scripting and synchronization](docs/scripting.md)
- [MCP and AI agents](docs/mcp.md)
- [Security and privacy](docs/security.md)
- [Enterprise app setup](docs/enterprise-setup.md)
- [Development and releases](docs/development.md)
- [Agent instructions](SKILL.md)

## Contributing

```bash
make build
make test
make lint
```

See [development and releases](docs/development.md) for repository structure,
validation, CI, and publishing details.

## License

See [LICENSE](LICENSE).
