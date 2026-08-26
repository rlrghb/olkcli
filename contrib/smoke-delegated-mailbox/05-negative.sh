#!/usr/bin/env bash
# Stage 5 — the refusals.
#
# PR 89 rewrote these messages precisely because Graph's own answer names none of
# the three grants a reader has to check: a bare "Access is denied", or an
# ErrorSendAsDenied that speaks only to the Exchange delegation and says nothing
# about the token scope. The rewritten text is the artifact under test, and it
# only reaches a reader in a live refusal, so the wording is recorded verbatim.
#
# These probes are real sends and real draft-creates. They write nothing only so
# long as the refusal fires, which is the thing being tested — so the stage
# re-establishes that the mailbox is unreachable immediately before each probe
# rather than trusting a note written earlier, and it never aims at a mailbox
# whose contents anyone is obliged to keep clean.

set -euo pipefail
# shellcheck source=lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

require_repo_build
section "Stage 5 — refusal messages, run ${RUN_ID}"

# Pick a mailbox the signed-in account genuinely holds nothing on, and never a
# compliance, incident or payments queue: this stage sends as it on purpose, so
# if a probe unexpectedly succeeds the residue should be a stray test message
# rather than something that lands in an audit.
NO_ACCESS_MAILBOX="${NO_ACCESS_MAILBOX:-}"
if [[ -z "${NO_ACCESS_MAILBOX}" ]]; then
  log "NO_ACCESS_MAILBOX is unset, so the refusal messages are not exercised."
  log "Set it to a mailbox this account has no permission on to run this stage."
  exit 0
fi
fact "negative.mailbox" "${NO_ACCESS_MAILBOX}"

section "Confirm the premise before relying on it"
# Everything below assumes this mailbox is unreachable. Access changes, and a
# stale assumption here is what turns a negative test into an unintended write.
olk_as "${SEND_ACCOUNT}" "${NO_ACCESS_MAILBOX}" mail folders >/dev/null
if [[ "${RUN_STATUS}" -eq 0 ]]; then
  fact "negative.premise" "BROKEN — ${SEND_ACCOUNT} can now read ${NO_ACCESS_MAILBOX}"
  log "FATAL: this stage sends as a mailbox it expects to be refused, and that"
  log "       mailbox is now reachable. Set NO_ACCESS_MAILBOX to one this"
  log "       account genuinely holds nothing on, and do not point it at a"
  log "       compliance or incident queue."
  exit 1
fi
fact "negative.premise" "confirmed unreachable: $(printf '%s' "${RUN_OUTPUT}" | head -1)"
fact "negative.read.error" "$(printf '%s' "${RUN_OUTPUT}" | head -1)"

if ! confirm "Run the refusal probes against ${NO_ACCESS_MAILBOX}? Each is a real request that writes only if the refusal fails to fire."; then
  log "Skipped."
  exit 0
fi

section "Send as a mailbox with no delegation"
# This is the case the hint text was written for. It should name all three
# grants — Mail.Send.Shared, Send As or Send on Behalf Of, and Full Access —
# because holding one tells the reader nothing about the other two.
olk_as "${SEND_ACCOUNT}" "${NO_ACCESS_MAILBOX}" mail send \
  -t "${RECIPIENT}" -s "[${TAG}] should be refused" \
  -b "If this arrives, the refusal did not happen and that is the finding."
if [[ "${RUN_STATUS}" -eq 0 ]]; then
  fact "negative.send" "NOT REFUSED — a message went out as ${NO_ACCESS_MAILBOX}; clean it up"
else
  fact "negative.send.error" "$(printf '%s' "${RUN_OUTPUT}" | tr '\n' ' ')"
fi

section "Create a draft in a mailbox with no delegation"
# The draft path needs a different pair of grants from the send path, so its
# refusal should read differently. If both produce the same text, one of them is
# misleading.
olk_as "${SEND_ACCOUNT}" "${NO_ACCESS_MAILBOX}" mail drafts create \
  -t "${RECIPIENT}" -s "[${TAG}] should be refused" -b "should be refused"
if [[ "${RUN_STATUS}" -eq 0 ]]; then
  fact "negative.draft" "NOT REFUSED — a draft was left in ${NO_ACCESS_MAILBOX}; delete it"
else
  fact "negative.draft.error" "$(printf '%s' "${RUN_OUTPUT}" | tr '\n' ' ')"
fi

section "A malformed mailbox value"
# Validation happens before anything reaches Graph, so this one is safe to run
# unconditionally and --dry-run does not weaken it.
olk_as "${SEND_ACCOUNT}" "not-an-address" --dry-run \
  mail send -t "${RECIPIENT}" -s "[${TAG}] malformed" -b "x"
assert_refused "negative.malformed_mailbox" "invalid --mailbox"

section "Stage 5 complete — quote these messages verbatim in the report"
