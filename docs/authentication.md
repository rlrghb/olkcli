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

Full Access is on the list because of the endpoint this takes. `olk` sends through
`/users/{mailbox}/sendMail` rather than `/me/sendMail` with a `from` address, so
the sent copy lands in the shared mailbox's Sent Items where the rest of the team
can see it, rather than in yours. Microsoft requires Full Access for that form on
top of the sending delegation; see
[Send Outlook messages from another user](https://learn.microsoft.com/en-us/graph/outlook-send-mail-from-other-user).
Confirm the sending address with `--dry-run`, which prints the mailbox a send will
leave from.

The draft commands honour `--mailbox` too, and are the lower-privilege path:
leaving a draft in a shared mailbox needs `Mail.ReadWrite.Shared` and Full
Access, but not Send As. Sending that draft afterwards does need Send As, since
creating a draft somewhere confers no right to send it.

`mail reply --mailbox EMAIL` and `mail forward --mailbox EMAIL` need the same
three grants, and read access besides: both read the original from that mailbox
before sending as it. The message ID must therefore be one listed from that
mailbox, since IDs are scoped to a mailbox and one taken from your own will not
resolve.

Calendar writes, contact writes, folder writes, and the commands that organise
mail in place — move, flag, categorise, mark — remain scoped to the signed-in
user; they do not read `--mailbox`.

## macOS Keychain

Tokens are stored in the macOS Keychain. After installing a freshly built or
upgraded binary, macOS may ask for Keychain access. Select **Always Allow**.

## Token storage

Refresh tokens are stored in the OS credential manager. Access tokens are held
in memory and are not persisted. Headless file-backend environments must set
`OLK_KEYRING_PASSWORD` and protect the config directory.
