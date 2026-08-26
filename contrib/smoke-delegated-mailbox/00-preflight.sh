#!/usr/bin/env bash
# Stage 0 — establishes the preconditions and records the starting state.
# Every call is a read or a --dry-run, with one honest exception: the two guard
# probes issue a real send and a real draft-create with --no-send and --no-write
# set, so they write something only if the guard they are testing is broken.
# Both are addressed to the operator, so the blast radius of that is one
# self-addressed message.

set -euo pipefail
# shellcheck source=lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

section "Stage 0 — preflight (read-only), run ${RUN_ID}"

require_repo_build
fact "run_id" "${RUN_ID}"
fact "binary" "$("${OLK}" version 2>&1 | head -1)"

section "Signed-in accounts"
run "${OLK}" auth list

section "Delegated read access — which mailboxes answer at all"
# Full Access on the mailbox and Mail.Read.Shared on the token fail differently,
# but both surface here. A mailbox that cannot be listed cannot be sent as
# either, so this decides which of the later stages are worth running.
for mb in "${SHARED_MAILBOX}" "${CONTROL_MAILBOX}"; do
  for acct in "${SEND_ACCOUNT}" "${RECIPIENT}"; do
    olk_as "${acct}" "${mb}" mail folders >/dev/null
    if [[ "${RUN_STATUS}" -eq 0 ]]; then
      fact "read.${acct}.${mb}" "ok"
    else
      fact "read.${acct}.${mb}" "denied: $(printf '%s' "${RUN_OUTPUT}" | head -1)"
    fi
  done
done

section "Dry runs — does the command name the sending mailbox before it sends?"
# The defect PR 89 fixes is silent by nature: a send from the wrong identity
# reports success and is visible only in the recipient's inbox. The dry run is
# the one place the caller can see the sending identity in advance, so check
# that it names the shared mailbox and not the signed-in account.
olk_as "${SEND_ACCOUNT}" "${SHARED_MAILBOX}" --dry-run \
  mail send -t "${RECIPIENT}" -s "[${TAG}] dry-run send" -b "dry run" --cc "${RECIPIENT}"
fact "dryrun.send.from" "$(printf '%s' "${RUN_OUTPUT}" | sed -n 's/^ *From: *//p')"

olk_as "${SEND_ACCOUNT}" "${SHARED_MAILBOX}" --dry-run \
  mail drafts create -t "${RECIPIENT}" -s "[${TAG}] dry-run draft" -b "dry run"
fact "dryrun.draft.in" "$(printf '%s' "${RUN_OUTPUT}" | sed -n 's/^ *In: *//p')"

section "Delegated item read — mail get against a shared-mailbox id"
# Folder and list reads are covered above; fetching one message by its
# mailbox-scoped id is a different request builder and worth its own check.
olk_as "${SEND_ACCOUNT}" "${SHARED_MAILBOX}" mail list -n 1 --concise --json --results-only >/dev/null
PROBE_ID="$(printf '%s' "${RUN_OUTPUT}" | jq -r 'if type=="array" then (first | .id // empty) else empty end' 2>/dev/null || true)"
if [[ -n "${PROBE_ID}" ]]; then
  olk_as "${SEND_ACCOUNT}" "${SHARED_MAILBOX}" mail get "${PROBE_ID}" --concise --json --results-only >/dev/null
  fact "read.mail_get.${SHARED_MAILBOX}" "exit=${RUN_STATUS}"
else
  fact "read.mail_get.${SHARED_MAILBOX}" "no message available to fetch"
fi

section "Capability guards still hold on the delegated paths"
# --no-send and --no-write are enforced in the graphapi client rather than per
# command, so they should refuse a delegated send exactly as they refuse an
# ordinary one. A guard that only covers /me would be a hole worth finding.
olk_as "${SEND_ACCOUNT}" "${SHARED_MAILBOX}" --no-send \
  mail send -t "${RECIPIENT}" -s "[${TAG}] guarded" -b "should be refused"
assert_refused "guard.no-send" "refused: sending is disabled"

olk_as "${SEND_ACCOUNT}" "${SHARED_MAILBOX}" --no-write \
  mail drafts create -t "${RECIPIENT}" -s "[${TAG}] guarded" -b "should be refused"
assert_refused "guard.no-write" "refused: writes are disabled"

section "Baseline — what is in each Sent Items before anything is sent"
for target in "${SHARED_MAILBOX}" ""; do
  label="${target:-own}"
  olk_as "${SEND_ACCOUNT}" "${target}" mail list --folder SentItems -n 3 --concise --plain
  fact "baseline.sentitems.${label}" "exit=${RUN_STATUS}"
done

section "Preflight complete"
log "Facts recorded in ${FACTS}"
log "Transcript in ${LOG}"
log ""
log "Read the two dryrun.* facts before going further. If either names"
log "${SEND_ACCOUNT} rather than ${SHARED_MAILBOX}, stop: the binary is not the"
log "one you think it is, and stage 1 would send from the wrong identity."
