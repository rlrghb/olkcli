# Enterprise setup: creating an olk Entra app registration (CLI-first, no click-ops)

Goal: a tenant-controlled **public client** app registration for `olk`, with delegated
Microsoft Graph permissions pre-approved by admin consent, optionally restricted to
assigned users or a group, usable as:

```bash
olk auth login --browser --enterprise --client-id <APP_ID> --tenant-id <TENANT_ID>
```

Terminology decoder (Microsoft uses three names for two objects):

| Object | Where it lives | What it holds |
|---|---|---|
| **App registration** (`application`) | App registrations blade / `az ad app` | Redirect URIs, requested permissions, public-client setting |
| **Enterprise application** (`servicePrincipal`) | Enterprise applications blade / `az ad sp` | Admin-consent grants, user/group assignment, the sign-in kill switch |

Creating the registration and then `az ad sp create` gives you both. "Roles" are not
involved — olk uses **delegated permissions (scopes)**, approved via **admin consent**.
App roles only apply to app-only/daemon identities, which olk deliberately does not use.

## 0. Install the Azure CLI

```bash
brew install azure-cli
az version
```

(A PowerShell alternative exists — see the bottom — but `az` covers everything here
with less ceremony.)

## 1. Sign in with an adequate role

```bash
az login --tenant <TENANT_ID>
```

Role requirements:

- **Create the app + service principal**: Application Administrator (or Cloud
  Application Administrator, or Global Administrator).
- **Grant admin consent** (`az ad app permission admin-consent`): Global
  Administrator (Application Administrator alone is not sufficient for Graph
  delegated consent via this command).

## 2. Run the script

Everything below is automated in [`contrib/create-entra-app.sh`](../contrib/create-entra-app.sh):

```bash
# Recommended: restrict the app to a group
TENANT_ID=<your-tenant-id> GROUP_OBJECT_ID=<group-object-id> \
  ./contrib/create-entra-app.sh

# Or restrict it to individual users
TENANT_ID=<your-tenant-id> ASSIGN_USERS="alice@example.com bob@example.com" \
  ./contrib/create-entra-app.sh
```

The script refuses to create an unrestricted app by accident. To intentionally
allow every tenant user to sign in, explicitly set `ALLOW_ALL_TENANT_USERS=1`:

```bash
TENANT_ID=<your-tenant-id> ALLOW_ALL_TENANT_USERS=1 \
  ./contrib/create-entra-app.sh
```

What it does, step by step (so you can audit or cherry-pick):

1. **Create the app registration** — single-tenant public client with the loopback
   redirect URI:

   ```bash
   az ad app create --display-name "olk CLI" \
     --sign-in-audience AzureADMyOrg \
     --public-client-redirect-uris "http://localhost/callback" \
     --is-fallback-public-client true
   ```

   - `--public-client-redirect-uris` = the portal's "Mobile and desktop applications"
     platform. Entra ignores the **port** when matching `http://localhost` redirect
     URIs, so the portless registration matches olk's random login-time port.
   - `--is-fallback-public-client true` = the portal's "Allow public client flows =
     Yes" (needed for the device-code fallback).
   - No secret, no certificate — a public client never has one.

2. **Resolve the Graph delegated-permission IDs dynamically** — permission GUIDs are
   looked up from the Microsoft Graph service principal
   (`00000003-0000-0000-c000-000000000000`) by scope name, so no hardcoded GUIDs:

   ```bash
   az ad sp show --id 00000003-0000-0000-c000-000000000000 \
     --query "oauth2PermissionScopes[?value=='Mail.ReadWrite'].id | [0]" -o tsv
   ```

3. **Add the delegated permissions** (`az ad app permission add ... "GUID=Scope"`).
   Scope set requested:

   - Core: `offline_access`, `User.Read`, `Mail.ReadWrite`, `Mail.Send`,
     `Calendars.ReadWrite`, `Contacts.ReadWrite`, `Tasks.ReadWrite`,
     `Files.ReadWrite`, `People.Read`
   - `--enterprise` extras: `User.ReadBasic.All`, `MailboxSettings.ReadWrite`

   For a read-only agent app, run the script with
   `SCOPES="offline_access User.Read Mail.Read Calendars.Read Contacts.Read Tasks.Read Files.Read People.Read User.ReadBasic.All MailboxSettings.Read"`
   and log in with `--read-only`.

4. **Create the service principal** (the "Enterprise application"):
   `az ad sp create --id <APP_ID>`.

5. **Grant admin consent** — the "approve" step:
   `az ad app permission admin-consent --id <APP_ID>`. All requested delegated
   scopes get a tenant-wide grant; users never see a consent prompt. Reminder:
   delegated tokens are always the *intersection* of these scopes and the signed-in
   user's own rights — this app grants nobody access to mailboxes they can't
   already touch.

6. **Require assignment** (who may use the app at all):
   `az ad sp update --id <APP_ID> --set appRoleAssignmentRequired=true`.
   Then assign principals — a **group or individual users, both work identically**
   (the Graph `appRoleAssignedTo` call takes any principal with the default access
   role, the all-zeros GUID):

   ```bash
   az rest --method POST \
     --url "https://graph.microsoft.com/v1.0/servicePrincipals/<SP_OBJECT_ID>/appRoleAssignedTo" \
     --body '{"principalId":"<USER_OR_GROUP_OBJECT_ID>","resourceId":"<SP_OBJECT_ID>","appRoleId":"00000000-0000-0000-0000-000000000000"}'
   ```

   The script automates both: `GROUP_OBJECT_ID=<guid>` for a group and/or
   `ASSIGN_USERS="alice@x.com bob@x.com"` for per-user assignment (UPNs are
   resolved automatically). Discovery commands:

   ```bash
   az ad signed-in-user show --query userPrincipalName -o tsv    # your own UPN
   az ad group show --group "<display name>" --query id -o tsv   # a group's ID
   az ad group list --query "[].{name:displayName,id:id}" -o table
   ```

   If you provide neither assignment variable, the script exits before creating
   any resources unless `ALLOW_ALL_TENANT_USERS=1` explicitly opts into an
   unrestricted app. In that mode, any tenant user can sign in (CA policies still
   apply). Per-user is fine to start; switch to a group later without recreating
   anything.

## 3. Verify and test

```bash
az ad app permission list-grants --id <APP_ID> --show-resource-name -o table   # consent landed
olk auth login --browser --enterprise --client-id <APP_ID> --tenant-id <TENANT_ID>
```

Success = browser opens → corporate sign-in (CA policies and all) → "Sign-in
complete" page → terminal prints `Logged in as …`. Then confirm Graph access:
`olk mail list -n 3`.

Afterwards, check **Entra → Sign-in logs** for the event: the client app should be
"olk CLI", and if the browser session came from a compliant device your
require-compliant-device CA policy should show **satisfied** (device code flow can
never do this — that's the point of `--browser`).

## Operational notes

- **Kill switch**: `az ad sp update --id <APP_ID> --set accountEnabled=false`
  blocks all sign-ins through the app instantly.
- **Revoke a user's tokens**: `az rest --method POST --url "https://graph.microsoft.com/v1.0/users/<UPN>/revokeSignInSessions"`
  (refresh tokens die immediately; access tokens age out within ~60–90 min).
- **Audit usage**: Sign-in logs filter → Application = "olk CLI".
- The app registration is inert without a user: no secret exists, and the client ID
  is not a credential.

## Portal fallback (if you must)

Entra admin center → **App registrations** → New registration → name `olk CLI`,
single tenant, platform **Public client/native (mobile & desktop)** with URI
`http://localhost/callback` → Register. Then: **Authentication** → Advanced →
Allow public client flows = Yes → Save. **API permissions** → Add a permission →
Microsoft Graph → Delegated → add the scope list above → **Grant admin consent
for <tenant>**. **Enterprise applications** → olk CLI → Properties → Assignment
required = Yes → Users and groups → add your group.

## PowerShell alternative (for completeness)

```powershell
Install-Module Microsoft.Graph.Applications -Scope CurrentUser
Connect-MgGraph -Scopes "Application.ReadWrite.All","DelegatedPermissionGrant.ReadWrite.All","AppRoleAssignment.ReadWrite.All"
```

Then `New-MgApplication` (with `PublicClient.RedirectUris` and
`RequiredResourceAccess`), `New-MgServicePrincipal`, and
`New-MgOauth2PermissionGrant` for the consent. It works, but it's three times the
code for the same result — use `az` unless your org standardizes on Graph PowerShell.
