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
scope plus Exchange delegation. Writes remain scoped to the signed-in user.

## macOS Keychain

Tokens are stored in the macOS Keychain. After installing a freshly built or
upgraded binary, macOS may ask for Keychain access. Select **Always Allow**.

## Token storage

Refresh tokens are stored in the OS credential manager. Access tokens are held
in memory and are not persisted. Headless file-backend environments must set
`OLK_KEYRING_PASSWORD` and protect the config directory.
