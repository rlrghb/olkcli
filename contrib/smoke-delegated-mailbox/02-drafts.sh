#!/usr/bin/env bash
# Stage 2 — draft create, list, send and delete against the shared mailbox.
#
# Drafting is the lower-privilege half of PR 89 and is worth testing on its own:
# creating a draft in a shared mailbox needs Mail.ReadWrite.Shared and Full
# Access but not Send As, so a tenant can perfectly well allow this stage and
# refuse stage 1. Sending the draft is the step that consumes the send grant.

set -euo pipefail
# shellcheck source=lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

require_repo_build
section "Stage 2 — delegated drafts, run ${RUN_ID}"

MARKER="[${TAG}] draft"

section "Create the draft in ${SHARED_MAILBOX}"
olk_as "${SEND_ACCOUNT}" "${SHARED_MAILBOX}" mail drafts create \
  -t "${RECIPIENT}" -s "${MARKER}" \
  -b "Smoke test for delegated drafts. Run ${RUN_ID}. Safe to delete."
fact "draft.create.exit" "${RUN_STATUS}"
fact "draft.create.output" "$(printf '%s' "${RUN_OUTPUT}" | head -1)"

if [[ "${RUN_STATUS}" -ne 0 ]]; then
  # Mail.ReadWrite.Shared is not in any default scope set and is not proven by
  # a working delegated read, so this is the likeliest place for the token to
  # come up short. Record the message rather than guessing which grant is missing.
  fact "draft.create.result" "refused — likely Mail.ReadWrite.Shared or Full Access"
  section "Stage 2 stopped: no draft to send"
  exit 0
fi

# `drafts create` prints the id in prose and ignores --json, so read the id back
# from a listing instead of parsing the success line.
section "Is the draft in the shared mailbox's Drafts, and not in the sender's?"
olk_as "${SEND_ACCOUNT}" "${SHARED_MAILBOX}" mail drafts list -n 25 --json --results-only >/dev/null
DRAFTS_JSON="${RUN_OUTPUT}"
DRAFT_ID="$(printf '%s' "${DRAFTS_JSON}" | jq -r --arg m "${MARKER}" \
  'if type=="array" then (map(select(.subject != null and (.subject | contains($m)))) | first | .id // empty) else empty end')"
fact "draft.in_shared_drafts" "${DRAFT_ID:-ABSENT}"

olk_as "${SEND_ACCOUNT}" "" mail drafts list -n 25 --json --results-only >/dev/null
OWN_HIT="$(printf '%s' "${RUN_OUTPUT}" | jq -r --arg m "${MARKER}" \
  'if type=="array" then (map(select(.subject != null and (.subject | contains($m)))) | length) else 0 end' 2>/dev/null || echo "?")"
fact "draft.in_own_drafts" "${OWN_HIT} match(es) — expect 0"

if [[ -z "${DRAFT_ID}" ]]; then
  section "Stage 2 stopped: draft id not found in ${SHARED_MAILBOX}"
  exit 0
fi

section "Send the draft"
if confirm "Send the draft from ${SHARED_MAILBOX} to ${RECIPIENT}?"; then
  olk_as "${SEND_ACCOUNT}" "${SHARED_MAILBOX}" mail drafts send "${DRAFT_ID}"
  fact "draft.send.exit" "${RUN_STATUS}"
  fact "draft.send.output" "$(printf '%s' "${RUN_OUTPUT}" | head -2 | tr '\n' ' ')"

  if [[ "${RUN_STATUS}" -eq 0 ]]; then
    if SENT="$(find_by_subject "${SEND_ACCOUNT}" "${SHARED_MAILBOX}" SentItems "${MARKER}")"; then
      fact "draft.send.from" "$(printf '%s' "${SENT}" | jq -r '.from // "<absent>"')"
    else
      fact "draft.send.from" "sent copy not filed in ${SHARED_MAILBOX} within 60s"
    fi

    # The same postconditions stage 1 checks, so that a difference between
    # sending directly and sending a draft would show up rather than hide in a
    # gap between the two stages.
    if find_by_subject "${SEND_ACCOUNT}" "" SentItems "${MARKER}" >/dev/null; then
      fact "draft.send.own_sentitems" "ALSO PRESENT in ${SEND_ACCOUNT} Sent Items"
    else
      fact "draft.send.own_sentitems" "absent, as expected"
    fi

    if RCV="$(find_delivered "${RECIPIENT}" "${MARKER}")"; then
      fact "draft.send.recipient_from" "$(printf '%s' "${RCV}" | jq -r '.from // "<absent>"')"
    else
      fact "draft.send.recipient_from" "not delivered within 60s"
    fi

    # Sending a draft should consume it: a copy left behind in Drafts would mean
    # the team sees an unsent message that has in fact gone out.
    olk_as "${SEND_ACCOUNT}" "${SHARED_MAILBOX}" mail drafts list -n 25 --json --results-only >/dev/null
    STILL="$(printf '%s' "${RUN_OUTPUT}" | jq -r --arg m "${MARKER}" \
      'if type=="array" then (map(select(.subject != null and (.subject | contains($m)))) | length) else 0 end' 2>/dev/null || echo "?")"
    fact "draft.send.removed_from_drafts" "${STILL} left in Drafts — expect 0"
  fi
else
  # An unsent draft is the honest fallback the docs describe, so leaving it in
  # place is a valid outcome — but say where it is rather than abandoning it.
  fact "draft.send" "declined; draft ${DRAFT_ID} left in ${SHARED_MAILBOX}"
  if confirm "Delete the unsent draft ${DRAFT_ID} from ${SHARED_MAILBOX}?"; then
    olk_as "${SEND_ACCOUNT}" "${SHARED_MAILBOX}" mail drafts delete "${DRAFT_ID}" --force
    fact "draft.delete.exit" "${RUN_STATUS}"
  fi
fi

section "Stage 2 complete — facts in ${FACTS}"
