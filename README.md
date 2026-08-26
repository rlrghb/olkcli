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
olk auth status
olk mail list --json --results-only --select id,subject,from
olk mail search 'from:person@example.com subject:urgent'
olk calendar view --days 7
olk contacts search "Alex"
olk todo lists list
olk drive ls
```

For work/school accounts and enterprise-only features:

```bash
olk auth login --enterprise
```

## Common commands

| Need | Command |
| --- | --- |
| Check the signed-in account | `olk auth status` or `olk whoami` |
| Find message IDs and subjects | `olk mail list --json --results-only --select id,subject,from` |
| Search mail | `olk mail search "from:person@example.com subject:urgent"` |
| View the next week | `olk calendar view --days 7` |
| Search contacts | `olk contacts search "Alex"` |
| List task lists | `olk todo lists list` |
| Browse OneDrive | `olk drive ls` |
| Send mail | `olk mail send --to person@example.com --subject "Hi" --body "Hello"` |
| Draft HTML with an inline image | `olk mail drafts create --to person@example.com --subject "Steps" --html --body '<img src="cid:steps">' --inline steps=steps.png` |
| Synchronize changes | `olk changes --json` |
| Use JSON for scripts | `olk mail list --json --results-only` |
| Use as an MCP server | `olk mcp` |

See the [command reference](docs/commands.md) for all commands and flags.

### Inline images in HTML drafts

Use a `cid:` URL in the HTML and supply the matching image with repeatable
`--inline CID=PATH` flags. This works for new drafts and true threaded reply or
reply-all drafts:

```bash
olk mail reply <ID> --draft --html \
  --body '<p>See both images:</p><img src="cid:logo"><img src="cid:steps">' \
  --inline logo=logo.png --inline steps=steps.png

olk mail drafts create --to person@example.com --subject "Instructions" --html \
  --body '<p>Follow these steps:</p><img src="cid:steps">' \
  --inline steps=steps.png
```

Each CID must be unique and referenced by the HTML. Files must be images and
each must be under 3 MB, the Microsoft Graph simple-attachment limit. Draft
commands do not send mail; immediate sends and forwards do not accept
`--inline`.

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
