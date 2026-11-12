# Authentication and accounts

## Personal accounts

The default device-code flow supports Outlook.com, Hotmail, and Live accounts:

```bash
olk auth login
```

## Enterprise accounts

Use `--enterprise` to request enterprise-only scopes for features such as
out-of-office, inbox rules, and directory search:

```bash
olk auth login --enterprise
```

For tenant-specific browser setup, see [enterprise setup](enterprise-setup.md).

## Browser login

Use authorization-code + PKCE when Conditional Access blocks device code:

```bash
olk auth login --browser
```

The flow uses a loopback redirect, validates a CSRF state value, and never
prints tokens or provider response text in the callback page.

## Accounts and scopes

```bash
olk auth accounts
olk auth status
olk auth login --scope Mail.Read.Shared --scope Calendars.Read.Shared
olk --account person@example.com mail list
```

Delegated mailbox reads use `--mailbox EMAIL` and require the matching `.Shared`
scope plus Exchange delegation.

`mail send --mailbox EMAIL` sends *as* that mailbox, which needs three separate
grants:

- `Mail.Send.Shared` on the token;
- Send As or Send on Behalf Of on the mailbox in Exchange; and
- Full Access on the mailbox.

Holding any one of them implies nothing about the others, and being able to read
a shared mailbox does not imply being able to send from it.

Full Access is on the list because Microsoft requires it for the endpoint this
takes — `/users/{mailbox}/sendMail`, over and above the sending delegation; see
[Send Outlook messages from another user](https://learn.microsoft.com/en-us/graph/outlook-send-mail-from-other-user).

That endpoint is chosen for where it files the sent copy: by default in the shared
mailbox's Sent Items, where the rest of the team can see it, rather than in yours
as `/me/sendMail` with a `from` address would. Mailbox configuration can override
either default, so treat it as the documented default rather than a guarantee.
Confirm the sending address with `--dry-run`, which prints the mailbox a send will
leave from.

The draft commands and `mail reply --draft` honour `--mailbox` too, and are the
lower-privilege path: leaving a standalone or true threaded reply draft in a
shared mailbox needs `Mail.ReadWrite.Shared` and Full Access, but not
`Mail.Send.Shared`, Send As, or Send on Behalf Of. Sending that draft afterwards
does need the sending grants, since creating a draft somewhere confers no right
to send it.

Immediate `mail reply --mailbox EMAIL` and `mail forward --mailbox EMAIL` need
the same three sending grants, and read access besides: both read the original
from that mailbox before sending as it. Reply-draft creation reads the same
mailbox-scoped original without sending. In every case, the message ID must be
one listed from that mailbox; one taken from your own mailbox will not resolve.

`mail move` and `mail delete` honour `--mailbox`; moving or deleting a
shared-mailbox message needs `Mail.ReadWrite.Shared` and Full Access, and the
message ID (and, for a move, the destination folder) must belong to that
mailbox. `mail folders create`, `rename` and `delete` honour it under the same
scope and grant, as do the commands that organise mail in place: `mail mark`,
`flag`, `categorize` and `importance`.

Calendar writes and event attachments honour `--mailbox` and need
`Calendars.ReadWrite.Shared` plus delegate or shared access to that calendar.
Contact writes need `Contacts.ReadWrite.Shared` plus access to the contacts
folder. To Do, inbox rules, categories and automatic replies are sent to the
named mailbox as well, but Microsoft documents no shared-mailbox access for
them: `Tasks.ReadWrite.Shared` exists with no Graph method published under it,
and mailbox settings have no shared scope at all. Graph may refuse those; `olk`
reports the refusal and never falls back to your own mailbox.

`people search`, `calendar availability` and `calendar find-times` do not read
`--mailbox`. Each asks on behalf of the signed-in user.

## macOS Keychain

Tokens are stored in the macOS Keychain. After installing a freshly built or
upgraded binary, macOS may ask for Keychain access. Select **Always Allow**.

A binary built with `make build` uses the `olk-dev` namespace: its own config
directory and its own Keychain items, so it needs its own `olk auth login` and
never reads or rewrites the tokens of an installed `olk`. `olk version` names
the namespace of a non-release build. Setting `OLK_CONFIG_DIR` to the same
directory for both builds shares the config and any file-backend tokens.

## Token storage

Access and refresh tokens, together with the access-token expiry, are stored in
the existing OS credential manager. Separate CLI processes reuse an access
token until it is within five minutes of expiry instead of contacting the token
endpoint for every command. Existing refresh-only entries refresh once on first
use and then retain the pair. Headless file-backend environments must set
`OLK_KEYRING_PASSWORD` and protect the config directory.

Failed refreshes leave stored credentials unchanged. JSON failures identify the
`token_refresh` stage and preserve the HTTP status, OAuth error name, numeric
AADSTS codes and `Retry-After` seconds when available. Response descriptions and
tokens are never included in these diagnostics. Refresh is attempted once; the
caller chooses when to retry.

### Concurrent processes on Linux

Credential reads, refreshes and writes use a per-account file lock. On Linux,
the lock directory follows `XDG_CACHE_HOME` (or defaults to `$HOME/.cache`).
Processes that share an OS credential-store entry must use the same cache
directory, even when they use different `OLK_CONFIG_DIR` values.

If, for example, an MCP server and a CLI command use different cache directories,
they take separate locks. Overlapping refreshes can overwrite stored tokens and
cause authentication failures; a refresh overlapping `olk auth logout` can
restore credentials after logout deletes them. Keep the cache environment
consistent across processes. If that is not possible, stop other processes
using the account before signing out and avoid concurrent credential operations.
