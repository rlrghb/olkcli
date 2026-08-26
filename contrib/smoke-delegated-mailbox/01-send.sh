#!/usr/bin/env bash
# Stage 1 — send as the shared mailbox, then read back who it went out as and
# where the sent copy was filed.
#
# This stage sends real mail. Each send is gated behind a confirmation.

set -euo pipefail
# shellcheck source=lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

require_repo_build
section "Stage 1 — delegated send, run ${RUN_ID}"

send_and_verify() {
  local mailbox="$1" label="$2"
  local marker="[${TAG}] send ${label}"

  if ! confirm "Send as ${mailbox} to ${RECIPIENT}?"; then
    fact "send.${label}" "skipped by operator"
    return 0
  fi

  olk_as "${SEND_ACCOUNT}" "${mailbox}" mail send \
    -t "${RECIPIENT}" -s "${marker}" \
    -b "Smoke test for delegated send. Run ${RUN_ID}. Safe to delete."
  fact "send.${label}.exit" "${RUN_STATUS}"
  fact "send.${label}.output" "$(printf '%s' "${RUN_OUTPUT}" | head -2 | tr '\n' ' ')"

  if [[ "${RUN_STATUS}" -ne 0 ]]; then
    # A refusal is a result, not a failed test. Graph answers a missing scope
    # and a missing Exchange delegation almost identically, so what matters is
    # whether the error names all three grants the caller has to check.
    fact "send.${label}.result" "refused — record the full message from the transcript"
    return 0
  fi

  # The command reporting success proves only that Graph accepted the request.
  # The sending identity lives in the filed copy, so read that instead.
  local sent
  if sent="$(find_by_subject "${SEND_ACCOUNT}" "${mailbox}" SentItems "${marker}")"; then
    fact "send.${label}.shared_sentitems" "present"
    fact "send.${label}.from" "$(printf '%s' "${sent}" | jq -r '.from // "<absent>"')"
  else
    fact "send.${label}.shared_sentitems" "ABSENT after 60s — see Sent Items placement note"
  fi

  # The endpoint PR 89 chose, /users/{mailbox}/sendMail, is supposed to file the
  # copy in the shared mailbox rather than the sender's own. Confirm the second
  # half of that claim as well: a copy in both places is a different behaviour
  # from a copy in one, and mailbox configuration can produce it.
  if find_by_subject "${SEND_ACCOUNT}" "" SentItems "${marker}" >/dev/null; then
    fact "send.${label}.own_sentitems" "ALSO PRESENT in ${SEND_ACCOUNT} Sent Items"
  else
    fact "send.${label}.own_sentitems" "absent, as expected"
  fi

  # The recipient's copy is the only artefact that shows what a human sees.
  if sent="$(find_delivered "${RECIPIENT}" "${marker}")"; then
    fact "send.${label}.recipient_from" "$(printf '%s' "${sent}" | jq -r '.from // "<absent>"')"
    fact "send.${label}.recipient_id" "$(printf '%s' "${sent}" | jq -r '.id')"
  else
    fact "send.${label}.recipient_from" "not delivered within 60s"
  fi
}

send_and_verify "${SHARED_MAILBOX}" "primary"

# The control mailbox has an independently granted delegation. Running both is
# the only way to notice that one is Send As and the other Send on Behalf Of,
# since olk prints the same From line for either.
if [[ -n "${CONTROL_MAILBOX}" ]] &&
  confirm "Also run the same send against the control mailbox ${CONTROL_MAILBOX}?"; then
  send_and_verify "${CONTROL_MAILBOX}" "control"
fi

section "Stage 1 complete — facts in ${FACTS}"
