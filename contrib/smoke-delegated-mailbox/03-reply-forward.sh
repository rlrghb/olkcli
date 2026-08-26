#!/usr/bin/env bash
# Stage 3 — reply, reply-all and forward as the shared mailbox.
#
# These three read the original from the target mailbox before sending as it, so
# they need read access on top of the send grants. Message ids are scoped to the
# mailbox that listed them, which is why the thread is seeded into the shared
# mailbox first rather than reusing an id from anywhere else.

set -euo pipefail
# shellcheck source=lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

require_repo_build
section "Stage 3 — delegated reply and forward, run ${RUN_ID}"

SEED_MARKER="[${TAG}] seed"

section "Seed a thread into ${SHARED_MAILBOX}"
# The seed carries a second recipient so that reply-all has somebody to widen
# to. Without one it addresses exactly the same people as a plain reply and
# demonstrates nothing. The sending account is used, because it is an automation
# account under the operator's control rather than a colleague.
if ! confirm "Send a seed message from ${RECIPIENT} to ${SHARED_MAILBOX}, cc ${SEND_ACCOUNT}?"; then
  log "Nothing to reply to. Stopping."
  exit 0
fi
olk_as "${RECIPIENT}" "" mail send \
  -t "${SHARED_MAILBOX}" --cc "${SEND_ACCOUNT}" -s "${SEED_MARKER}" \
  -b "Seed message for the delegated reply and forward smoke test. Run ${RUN_ID}."
assert_ok "seed.send"

section "Find the seed by its mailbox-scoped id"
if ! SEED="$(find_by_subject "${SEND_ACCOUNT}" "${SHARED_MAILBOX}" Inbox "${SEED_MARKER}")"; then
  fact "seed.delivered" "not visible in ${SHARED_MAILBOX} Inbox within 60s"
  log "Check whether an inbox rule filed it elsewhere before treating this as a failure."
  exit 0
fi
SEED_ID="$(printf '%s' "${SEED}" | jq -r '.id')"
SEED_CONV="$(printf '%s' "${SEED}" | jq -r '.conversationId // ""')"
fact "seed.id" "${SEED_ID:0:40}..."
fact "seed.conversationId" "${SEED_CONV:0:40}..."

section "Reply as the mailbox"
if confirm "Reply as ${SHARED_MAILBOX} to the seed?"; then
  olk_as "${SEND_ACCOUNT}" "${SHARED_MAILBOX}" mail reply "${SEED_ID}" \
    -b "Delegated reply. Run ${RUN_ID}."
  fact "reply.exit" "${RUN_STATUS}"
  fact "reply.output" "$(printf '%s' "${RUN_OUTPUT}" | head -2 | tr '\n' ' ')"
  if [[ "${RUN_STATUS}" -eq 0 ]]; then
    if SENT="$(find_by_subject "${SEND_ACCOUNT}" "${SHARED_MAILBOX}" SentItems "${SEED_MARKER}")"; then
      fact "reply.from" "$(printf '%s' "${SENT}" | jq -r '.from // "<absent>"')"
      # Threading is a comparison, not a value: the reply belongs to the seed's
      # conversation or it started a new one, and only the two together say which.
      REPLY_CONV="$(printf '%s' "${SENT}" | jq -r '.conversationId // ""')"
      if [[ -n "${SEED_CONV}" && "${REPLY_CONV}" == "${SEED_CONV}" ]]; then
        fact "reply.threaded" "yes — same conversationId as the seed"
      else
        fact "reply.threaded" "NO — reply ${REPLY_CONV:0:24} vs seed ${SEED_CONV:0:24}"
      fi
    else
      fact "reply.from" "sent copy not filed in ${SHARED_MAILBOX} within 60s"
    fi
    # The same two-sided placement check stage 1 makes: a copy in the delegate's
    # own Sent Items as well would be a different behaviour worth knowing about.
    if find_by_subject "${SEND_ACCOUNT}" "" SentItems "${SEED_MARKER}" >/dev/null; then
      fact "reply.own_sentitems" "ALSO PRESENT in ${SEND_ACCOUNT} Sent Items"
    else
      fact "reply.own_sentitems" "absent, as expected"
    fi
    if RCV="$(find_delivered "${RECIPIENT}" "RE: ${SEED_MARKER}" "${SEED_MARKER}")"; then
      fact "reply.recipient_from" "$(printf '%s' "${RCV}" | jq -r '.from // "<absent>"')"
    fi
  fi
fi

# Read the current number of filed "RE:" copies rather than assuming the reply
# above ran, so that skipping it does not make the reply-all check unfalsifiable.
REPLY_SENT_COUNT="$(count_by_subject "${SEND_ACCOUNT}" "${SHARED_MAILBOX}" SentItems "RE: ${SEED_MARKER}")"

section "Reply-all as the mailbox"
# Reply-all takes a different Graph endpoint from reply, so a delegation that
# works for one is not evidence for the other. What distinguishes it here is the
# recipient list: it should reach the cc'd address as well as the sender.
if confirm "Reply-all as ${SHARED_MAILBOX} to the seed (reaches ${RECIPIENT} and ${SEND_ACCOUNT})?"; then
  olk_as "${SEND_ACCOUNT}" "${SHARED_MAILBOX}" mail reply "${SEED_ID}" --reply-all \
    -b "Delegated reply-all. Run ${RUN_ID}."
  fact "replyall.exit" "${RUN_STATUS}"
  fact "replyall.output" "$(printf '%s' "${RUN_OUTPUT}" | head -2 | tr '\n' ' ')"
  if [[ "${RUN_STATUS}" -eq 0 ]]; then
    # A reply and a reply-all file copies with byte-identical subjects, so the
    # presence of a "RE:" copy proves nothing about which of the two produced
    # it. The count is what distinguishes them, which is why the plain reply
    # above recorded one before this step ran.
    if wait_for_count "${SEND_ACCOUNT}" "${SHARED_MAILBOX}" SentItems \
      "RE: ${SEED_MARKER}" "$((REPLY_SENT_COUNT + 1))" >/dev/null; then
      fact "replyall.shared_sentitems" "a second RE: copy filed in ${SHARED_MAILBOX}"
    else
      fact "replyall.shared_sentitems" "NO second RE: copy filed within 60s"
    fi
    # The widening is the point: the cc'd account should have received it, and
    # it holds no earlier "RE:" copy because a plain reply goes only to the
    # original sender.
    if wait_for_count "${SEND_ACCOUNT}" "" Inbox "RE: ${SEED_MARKER}" 1 >/dev/null; then
      fact "replyall.reached_cc" "yes — the cc'd address received it"
    else
      fact "replyall.reached_cc" "NO — the cc'd address did not receive it"
    fi
  fi
fi

section "Forward as the mailbox"
if confirm "Forward the seed from ${SHARED_MAILBOX} to ${RECIPIENT}?"; then
  olk_as "${SEND_ACCOUNT}" "${SHARED_MAILBOX}" mail forward "${SEED_ID}" \
    -t "${RECIPIENT}" -c "Delegated forward. Run ${RUN_ID}."
  fact "forward.exit" "${RUN_STATUS}"
  fact "forward.output" "$(printf '%s' "${RUN_OUTPUT}" | head -2 | tr '\n' ' ')"
  if [[ "${RUN_STATUS}" -eq 0 ]]; then
    if RCV="$(find_delivered "${RECIPIENT}" "FW: ${SEED_MARKER}" "${SEED_MARKER}")"; then
      fact "forward.recipient_from" "$(printf '%s' "${RCV}" | jq -r '.from // "<absent>"')"
    else
      fact "forward.recipient_from" "not delivered within 60s"
    fi
    if find_by_subject "${SEND_ACCOUNT}" "" SentItems "FW: ${SEED_MARKER}" >/dev/null; then
      fact "forward.own_sentitems" "ALSO PRESENT in ${SEND_ACCOUNT} Sent Items"
    else
      fact "forward.own_sentitems" "absent, as expected"
    fi
  fi
fi

section "Negative case — an id from the wrong mailbox"
# Ids are mailbox-scoped, so one taken from the caller's own mailbox should not
# resolve against the shared one. This cannot be run with --dry-run: reply
# returns before it reaches Graph in dry-run mode, so the check would pass
# whether or not the id resolved. It therefore runs for real, and is gated: if
# the id does resolve, a reply goes to whoever sent the caller that message.
if confirm "Run the id-scoping check for real? It sends nothing if the id is correctly rejected, and one reply if olk has the bug."; then
  olk_as "${SEND_ACCOUNT}" "" mail list -n 1 --concise --json --results-only >/dev/null
  OWN_ID="$(printf '%s' "${RUN_OUTPUT}" | jq -r 'if type=="array" then (first | .id // empty) else empty end')"
  if [[ -n "${OWN_ID}" ]]; then
    # A reply carries the original's subject, so the probe cannot be recognised
    # by one. Note what sits at the top of Sent Items before it runs instead, so
    # that a timeout can still be settled by looking at what changed.
    SENT_TOP_BEFORE="$(newest_id "${SEND_ACCOUNT}" "${SHARED_MAILBOX}" SentItems)"
    olk_as "${SEND_ACCOUNT}" "${SHARED_MAILBOX}" mail reply "${OWN_ID}" \
      -b "Smoke test ${RUN_ID}: this should never have been sent. Please ignore."
    if [[ "${RUN_STATUS}" -eq 0 ]]; then
      fact "wrong_mailbox_id" "ACCEPTED — an own-mailbox id resolved against ${SHARED_MAILBOX}"
    elif printf '%s' "${RUN_OUTPUT}" | grep -q 'context deadline exceeded'; then
      # A client-side timeout is not a refusal. The request reached Graph and its
      # fate is unknown, so the only honest answer comes from looking at what the
      # mailbox now holds rather than from the exit status.
      sleep 20
      SENT_TOP_AFTER="$(newest_id "${SEND_ACCOUNT}" "${SHARED_MAILBOX}" SentItems)"
      if [[ -n "${SENT_TOP_AFTER}" && "${SENT_TOP_AFTER}" != "${SENT_TOP_BEFORE}" ]]; then
        fact "wrong_mailbox_id" "TIMED OUT AND SENT — a new message is at the top of ${SHARED_MAILBOX} Sent Items"
      else
        fact "wrong_mailbox_id" "timed out with nothing sent: the request exceeded the client timeout and Sent Items is unchanged"
      fi
    else
      fact "wrong_mailbox_id" "rejected: $(printf '%s' "${RUN_OUTPUT}" | head -1)"
    fi
  fi
fi

section "Stage 3 complete — facts in ${FACTS}"
