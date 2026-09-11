#!/usr/bin/env bash
# Stage 5 — remove what can be removed, name what cannot, and assemble the
# report from the recorded facts.

set -euo pipefail
# shellcheck source=lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

section "Stage 6 — cleanup, run ${RUN_ID}"

# This stage removes only the drafts it created. The sent copies this run left
# in the shared mailbox are listed below for the operator to remove, either with
# `mail delete --mailbox` or by hand in Outlook.
section "Drafts left behind in ${SHARED_MAILBOX}"
olk_as_private "${SEND_ACCOUNT}" "${SHARED_MAILBOX}" mail drafts list -n 25 --json --results-only
if [[ "${RUN_STATUS}" -ne 0 ]]; then
  log "FATAL: draft listing failed (exit ${RUN_STATUS}); private command output was withheld from the transcript."
  exit 1
fi

# Collect before prompting: a `while read` fed from a pipe would take the
# operator's keystrokes for each confirmation out of the same stream as the list.
DRAFT_ROWS=()
while IFS= read -r row; do
  [[ -n "${row}" ]] && DRAFT_ROWS+=("${row}")
done < <(printf '%s' "${RUN_OUTPUT}" | jq -r --arg m "${TAG}" \
  'if type=="array" then (map(select(.subject != null and (.subject | contains($m)))) | .[] | "\(.id)\t\(.subject)") else empty end' 2>/dev/null || true)

log "drafts matching ${TAG}: ${#DRAFT_ROWS[@]}"
for row in ${DRAFT_ROWS[@]+"${DRAFT_ROWS[@]}"}; do
  id="${row%%$'\t'*}"
  subject="${row#*$'\t'}"
  log "draft: ${subject}"
  if confirm "Delete draft ${id:0:30}... from ${SHARED_MAILBOX}?"; then
    olk_as "${SEND_ACCOUNT}" "${SHARED_MAILBOX}" mail drafts delete "${id}" --force
  fi
done

section "Messages left for the operator to remove"
cat <<TEXT | tee -a "${LOG}"

Delete these by hand in Outlook on the web, searching for ${TAG}:

  ${SHARED_MAILBOX}   Sent Items, and the seed in Inbox
  ${CONTROL_MAILBOX:-(no control mailbox set)}  Sent Items, if the control send ran
  ${RECIPIENT}        Inbox, the delivered copies

Or remove the first two with "mail delete --mailbox", which acts on the named
mailbox; this stage does not delete sent copies for you.

TEXT

section "Recorded facts"
if [[ -f "${FACTS}" ]]; then
  column -t -s $'\t' "${FACTS}" | tee -a "${LOG}"
else
  log "No facts file at ${FACTS} — were the earlier stages run with this RUN_ID?"
fi

section "Report"
log "Transcript: ${LOG}"
log "Facts:      ${FACTS}"
log ""
log "Every stage has to be run with the same RUN_ID to share one results"
log "directory. Export it once and run the stages in order:"
log ""
log "  export RUN_ID=${RUN_ID}"
