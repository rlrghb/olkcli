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

`mail send --mailbox EMAIL` sends *as* that mailbox, which needs `Mail.Send.Shared`
on the token **and** Send As or Send on Behalf Of on the mailbox in Exchange —
a separate delegation from the Full Access that permits reading it. Being able to
read a shared mailbox therefore does not imply being able to send from it. Confirm
the sending address with `--dry-run`, which prints the mailbox a send will leave
from.

The draft commands honour `--mailbox` too, and are the lower-privilege path:
leaving a draft in a shared mailbox needs `Mail.ReadWrite.Shared` and Full
Access, but not Send As. Sending that draft afterwards does need Send As, since
creating a draft somewhere confers no right to send it.

`mail reply --mailbox EMAIL` and `mail forward --mailbox EMAIL` need the same two
grants, and read access besides: both read the original from that mailbox before
sending as it. The message ID must therefore be one listed from that mailbox,
since IDs are scoped to a mailbox and one taken from your own will not resolve.

Calendar and contact writes remain scoped to the signed-in user; they do not read
`--mailbox`.

## macOS Keychain

Tokens are stored in the macOS Keychain. After installing a freshly built or
upgraded binary, macOS may ask for Keychain access. Select **Always Allow**.

## Token storage

Refresh tokens are stored in the OS credential manager. Access tokens are held
in memory and are not persisted. Headless file-backend environments must set
`OLK_KEYRING_PASSWORD` and protect the config directory.
