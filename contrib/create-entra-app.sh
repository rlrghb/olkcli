#!/usr/bin/env bash
set -euo pipefail

# Creates the "olk CLI" Entra app registration + service principal, adds the
# delegated Graph permissions, grants admin consent, and (optionally) requires
# and performs group assignment. See docs/enterprise-setup.md for the walkthrough.
#
# Usage:
#   TENANT_ID=<tenant-guid> ./contrib/create-entra-app.sh
#
# Optional environment overrides:
#   APP_NAME         display name             (default: "olk CLI")
#   SCOPES           space-separated scopes   (default: full olk --enterprise set)
#   GROUP_OBJECT_ID  group object ID to assign
#   ASSIGN_USERS     space-separated user UPNs (or object IDs) to assign
#   ALLOW_ALL_TENANT_USERS=1 to intentionally allow any tenant user to sign in
# Providing either assignment variable also sets appRoleAssignmentRequired=true,
# so ONLY the assigned principals can sign in through the app. Without an
# assignment, ALLOW_ALL_TENANT_USERS=1 is required as an explicit opt-in.
#
# Finding IDs:
#   your own UPN:      az ad signed-in-user show --query userPrincipalName -o tsv
#   a group's ID:      az ad group show --group "<display name>" --query id -o tsv
#   browse groups:     az ad group list --query "[].{name:displayName,id:id}" -o table
#
# Requires: az CLI, logged in (az login --tenant $TENANT_ID) as a user who can
# create applications; admin consent additionally needs Global Administrator.

TENANT_ID="${TENANT_ID:?Set TENANT_ID=<your directory (tenant) ID>}"
APP_NAME="${APP_NAME:-olk CLI}"
SCOPES="${SCOPES:-offline_access User.Read Mail.ReadWrite Mail.Send Calendars.ReadWrite Contacts.ReadWrite Tasks.ReadWrite Files.ReadWrite People.Read User.ReadBasic.All MailboxSettings.ReadWrite}"
GRAPH_SP="00000003-0000-0000-c000-000000000000"

if [ -z "${GROUP_OBJECT_ID:-}" ] &&
  [ -z "${ASSIGN_USERS:-}" ] &&
  [ "${ALLOW_ALL_TENANT_USERS:-}" != "1" ]; then
  echo "error: no user or group assignment configured." >&2
  echo "Set GROUP_OBJECT_ID or ASSIGN_USERS to restrict access." >&2
  echo "To intentionally allow every tenant user, set ALLOW_ALL_TENANT_USERS=1." >&2
  exit 1
fi

current_tenant=$(az account show --query tenantId -o tsv 2>/dev/null || true)
if [ "$current_tenant" != "$TENANT_ID" ]; then
  echo "error: az is logged into tenant '${current_tenant:-none}', expected '$TENANT_ID'." >&2
  echo "Run: az login --tenant $TENANT_ID" >&2
  exit 1
fi

echo "==> Creating app registration '$APP_NAME' (single-tenant public client)"
APP_ID=$(az ad app create \
  --display-name "$APP_NAME" \
  --sign-in-audience AzureADMyOrg \
  --public-client-redirect-uris "http://localhost/callback" \
  --is-fallback-public-client true \
  --query appId -o tsv)
echo "    appId: $APP_ID"

echo "==> Resolving Graph delegated-permission IDs for: $SCOPES"
perms=()
for scope in $SCOPES; do
  perm_id=$(az ad sp show --id "$GRAPH_SP" \
    --query "oauth2PermissionScopes[?value=='$scope'].id | [0]" -o tsv)
  if [ -z "$perm_id" ] || [ "$perm_id" = "None" ]; then
    echo "error: could not resolve Graph delegated scope '$scope'" >&2
    exit 1
  fi
  echo "    $scope = $perm_id"
  perms+=("$perm_id=Scope")
done

echo "==> Adding API permissions to the app registration"
az ad app permission add --id "$APP_ID" --api "$GRAPH_SP" --api-permissions "${perms[@]}" \
  --only-show-errors

echo "==> Creating service principal (Enterprise application)"
SP_ID=$(az ad sp create --id "$APP_ID" --query id -o tsv)
echo "    servicePrincipal objectId: $SP_ID"

echo "==> Granting admin consent (requires Global Administrator)"
# Directory writes propagate asynchronously; retry briefly before failing.
for attempt in 1 2 3 4 5; do
  if az ad app permission admin-consent --id "$APP_ID" --only-show-errors; then
    break
  fi
  if [ "$attempt" = 5 ]; then
    echo "error: admin consent failed after $attempt attempts." >&2
    echo "Retry manually: az ad app permission admin-consent --id $APP_ID" >&2
    exit 1
  fi
  echo "    not ready yet (attempt $attempt), retrying in 10s..."
  sleep 10
done

# assign_principal grants a user or group the default access role on the app.
assign_principal() {
  az rest --method POST \
    --url "https://graph.microsoft.com/v1.0/servicePrincipals/$SP_ID/appRoleAssignedTo" \
    --body "{\"principalId\":\"$1\",\"resourceId\":\"$SP_ID\",\"appRoleId\":\"00000000-0000-0000-0000-000000000000\"}" \
    --only-show-errors >/dev/null
}

if [ -n "${GROUP_OBJECT_ID:-}" ] || [ -n "${ASSIGN_USERS:-}" ]; then
  echo "==> Requiring assignment (only assigned principals may use the app)"
  az ad sp update --id "$APP_ID" --set appRoleAssignmentRequired=true

  if [ -n "${GROUP_OBJECT_ID:-}" ]; then
    echo "    assigning group $GROUP_OBJECT_ID"
    assign_principal "$GROUP_OBJECT_ID"
  fi
  for upn in ${ASSIGN_USERS:-}; do
    user_id=$(az ad user show --id "$upn" --query id -o tsv)
    echo "    assigning user $upn ($user_id)"
    assign_principal "$user_id"
  done
else
  echo "==> Explicitly allowing any tenant user to sign in through this app."
  echo "    Restrict later with GROUP_OBJECT_ID=<guid> or ASSIGN_USERS=\"a@x.com b@x.com\","
  echo "    or in one line per user:"
  echo "    az ad sp update --id $APP_ID --set appRoleAssignmentRequired=true"
fi

echo
echo "==> Done. Verify consent:"
echo "    az ad app permission list-grants --id $APP_ID --show-resource-name -o table"
echo
echo "==> Log in with:"
echo "    olk auth login --browser --enterprise --client-id $APP_ID --tenant-id $TENANT_ID"
