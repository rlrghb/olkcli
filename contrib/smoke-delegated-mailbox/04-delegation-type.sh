#!/usr/bin/env bash
# Stage 4 — decide, for each mailbox, whether the send used Send As or Send on
# Behalf Of.
#
# olk cannot answer this on its own. Both delegations put the shared mailbox in
# the message's `from` property; the only thing that separates them is `sender`,
# which carries the delegate under Send on Behalf Of and the mailbox itself
# under Send As. `sender` is already accepted in the CLI's $select allowlist but
# is never read into MailMessage or printed, so every olk output looks identical
# either way. This stage therefore prints what a human has to look at, and
# records the answer they come back with.

set -euo pipefail
# shellcheck source=lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

section "Stage 4 — Send As or Send on Behalf Of, run ${RUN_ID}"

cat <<TEXT | tee -a "${LOG}"

For each message this run produced, open the recipient's copy in Outlook on the
web as ${RECIPIENT} and read the sender line.

  Send As              the message reads simply as the mailbox:
                       "Information Security"

  Send on Behalf Of    Outlook renders the delegate too:
                       "Ben Collier on behalf of Information Security"

If the rendering is ambiguous, take the raw headers, which are not:

  1. Open the message, then the ... menu, then View, then View message source.
  2. Compare the two headers.
       From: ...        the mailbox under either delegation
       Sender: ...      absent or equal to From under Send As;
                        the delegate's address under Send on Behalf Of.

Search the mailbox for ${TAG} to find every message this run produced.

TEXT

# Only ask about a mailbox this run actually sent as. Recording an answer for a
# send that was skipped or refused would put an unsupported claim in the report
# alongside the measured ones, and nothing downstream would tell them apart.
sent_as() {
  local mailbox="$1"
  [[ -f "${FACTS}" ]] || return 1
  grep -qE "^(send\.[a-z]+|draft|reply|forward)\..*${mailbox}" "${FACTS}" ||
    grep -qE "^send\.(primary|control)\.from\t.*${mailbox}" "${FACTS}"
}

record() {
  local label="$1" reply=""
  if ! sent_as "${label}"; then
    fact "delegation.${label}" "not applicable — no successful send as this mailbox in run ${RUN_ID}"
    return 0
  fi
  printf '\n>>> %s — [a]s / [o]n behalf / [u]nknown: ' "${label}" >&2
  read -r reply
  case "${reply}" in
  a | A) fact "delegation.${label}" "observed Send As on this message" ;;
  o | O) fact "delegation.${label}" "observed Send on Behalf Of on this message" ;;
  *) fact "delegation.${label}" "not determined" ;;
  esac
}

record "${SHARED_MAILBOX}"
if [[ -n "${CONTROL_MAILBOX}" ]]; then
  record "${CONTROL_MAILBOX}"
fi

cat <<'TEXT' | tee -a "${LOG}"

What this establishes, and what it does not.

It establishes which delegation *this message* went out under. It does not
inventory which grants the mailbox holds, and the difference matters: Exchange
resolves Send As ahead of Send on Behalf Of when a delegate has both, so
observing Send As is equally consistent with "only Send As is granted" and with
"both are granted". Observing Send As therefore never proves Send on Behalf Of
is absent.

The authoritative inventory is Exchange-side, and needs Exchange Online
PowerShell rather than Graph, which does not expose mailbox permissions at all:

  Get-RecipientPermission <mailbox>            # Send As
  Get-Mailbox <mailbox> | fl GrantSendOnBehalfTo   # Send on Behalf Of
  Get-MailboxPermission <mailbox>              # Full Access

pwsh is not installed on this machine, so that has to be run by somebody who has
it, or read from the Exchange admin centre. Report the observed behaviour and the
granted permissions as two separate findings; presenting either as the other is
the mistake this section exists to prevent.

TEXT

section "Stage 4 complete — facts in ${FACTS}"
