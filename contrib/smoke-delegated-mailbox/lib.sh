#!/usr/bin/env bash
# Shared configuration and helpers for the olk delegated-mailbox smoke test.
# Sourced by every numbered stage; not runnable on its own.

set -euo pipefail

SMOKE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "${SMOKE_DIR}/../.." && pwd)"

# Pin the repository build by full path. A binary found on PATH is most likely
# the released one, and any build predating the delegated-write work accepts
# --mailbox on a send and ignores it, which turns every assertion here into a
# false pass. The expected commit follows the checkout rather than being frozen,
# so the guard keeps working as the repository moves.
OLK="${OLK:-${REPO_DIR}/bin/olk}"
EXPECTED_COMMIT="${EXPECTED_COMMIT:-$(git -C "${REPO_DIR}" rev-parse --short HEAD 2>/dev/null || true)}"

# require_env fails by name rather than falling back to somebody else's mailbox.
# These addresses decide who receives real mail, so a forgotten variable has to
# stop the run, not pick a default.
require_env() {
  local name="$1" purpose="$2"
  if [[ -z "${!name:-}" ]]; then
    printf 'FATAL: %s is not set (%s).\n' "${name}" "${purpose}" >&2
    printf '       Copy config.example.sh, fill it in, and source it first.\n' >&2
    exit 1
  fi
}

# Who signs in, which mailbox is targeted, and who receives the test mail.
require_env SEND_ACCOUNT "the signed-in account that holds the delegation"
require_env SHARED_MAILBOX "the shared mailbox to send as"
require_env RECIPIENT "the address the test messages are sent to"

# Optional. A second mailbox whose delegation was granted independently: sending
# as both is the only way to notice that one is Send As and the other Send on
# Behalf Of. Stages that need it skip themselves when it is unset.
CONTROL_MAILBOX="${CONTROL_MAILBOX:-}"

# Every subject carries the run identifier so that verification can match on it
# and a human reading the mailbox knows at a glance what the message is.
RUN_ID="${RUN_ID:-$(date -u +%Y%m%dT%H%M%SZ)}"
# shellcheck disable=SC2034  # read by the sourcing stage scripts, not here.
TAG="OLK-SMOKE-${RUN_ID}"

RESULTS_DIR="${SMOKE_DIR}/results/${RUN_ID}"
mkdir -p "${RESULTS_DIR}"

LOG="${RESULTS_DIR}/transcript.log"
FACTS="${RESULTS_DIR}/facts.tsv"

# Guards that belong on every call. --no-input turns a prompt into a failure
# rather than a hang; the send guards are lifted per-command, deliberately.
OLK_COMMON=(--no-input)

log() {
  printf '%s\n' "$*" | tee -a "${LOG}"
}

section() {
  log ""
  log "=============================================================="
  log "$*"
  log "=============================================================="
}

# fact records one observation as a tab-separated row, so the final report is
# assembled from recorded results rather than from memory.
fact() {
  local key="$1" value="$2"
  printf '%s\t%s\n' "${key}" "${value}" >>"${FACTS}"
  log "  [fact] ${key} = ${value}"
}

# run echoes a command before running it and captures both streams, so the
# transcript shows exactly what was issued even when a call fails.
run() {
  log "\$ $*"
  local out status=0
  out="$("$@" 2>&1)" || status=$?
  printf '%s\n' "${out}" | tee -a "${LOG}"
  # shellcheck disable=SC2034  # both are read by the sourcing stage scripts.
  RUN_OUTPUT="${out}"
  # shellcheck disable=SC2034
  RUN_STATUS="${status}"
  return 0
}

olk_as() {
  local account="$1" mailbox="$2"
  shift 2
  if [[ -n "${mailbox}" ]]; then
    run "${OLK}" --account "${account}" --mailbox "${mailbox}" "${OLK_COMMON[@]}" "$@"
  else
    run "${OLK}" --account "${account}" "${OLK_COMMON[@]}" "$@"
  fi
}

# confirm is the gate in front of anything that puts mail in front of a real
# person. Outbound mail cannot be recalled, so each send is agreed separately.
confirm() {
  local prompt="$1"
  if [[ "${SMOKE_ASSUME_YES:-}" == "1" ]]; then
    log "[confirm] ${prompt} -> auto-yes (SMOKE_ASSUME_YES=1)"
    return 0
  fi
  local reply=""
  printf '\n>>> %s [y/N] ' "${prompt}" >&2
  read -r reply
  case "${reply}" in
  y | Y | yes | YES)
    log "[confirm] ${prompt} -> yes"
    return 0
    ;;
  *)
    log "[confirm] ${prompt} -> declined, skipping"
    return 1
    ;;
  esac
}

# find_by_subject polls a folder for a message whose subject carries a marker.
# Exchange files a sent copy asynchronously, so a single immediate read reports
# a false absence; poll rather than sleep once and guess.
find_by_subject() {
  local account="$1" mailbox="$2" folder="$3" marker="$4"
  local attempt json err status
  err="$(mktemp)"
  for attempt in 1 2 3 4 5 6 7 8 9 10; do
    status=0
    if [[ -n "${mailbox}" ]]; then
      json="$("${OLK}" --account "${account}" --mailbox "${mailbox}" --no-input \
        mail list --folder "${folder}" -n 25 --concise --json --results-only 2>"${err}")" || status=$?
    else
      json="$("${OLK}" --account "${account}" --no-input \
        mail list --folder "${folder}" -n 25 --concise --json --results-only 2>"${err}")" || status=$?
    fi
    # A listing that failed outright is not the same as a message that has not
    # arrived, and silently retrying past an authentication or permission error
    # would report it as a false absence.
    if [[ -s "${err}" ]]; then
      log "  ! listing ${folder} in ${mailbox:-own mailbox} failed: $(head -1 "${err}")" >&2
    fi
    if [[ "${status}" -ne 0 ]]; then
      rm -f "${err}"
      return 2
    fi
    if [[ -n "${json}" ]]; then
      local hit
      hit="$(printf '%s' "${json}" | jq -c --arg m "${marker}" \
        'if type=="array" then (map(select(.subject != null and (.subject | contains($m)))) | first) else empty end' 2>/dev/null || true)"
      if [[ -n "${hit}" && "${hit}" != "null" ]]; then
        rm -f "${err}"
        printf '%s' "${hit}"
        return 0
      fi
    fi
    # Progress goes to stderr: this function's stdout is the JSON its callers
    # capture, and a retry line printed there would corrupt it.
    log "  ... not filed yet (attempt ${attempt}/10), waiting 6s" >&2
    sleep 6
  done
  rm -f "${err}"
  return 1
}

# assert_refused turns "the guard did not fire" into a loud failure rather than
# a recorded zero. A guard that silently stopped working would otherwise let the
# stage carry on having sent the message it was meant to prevent.
assert_refused() {
  local label="$1" expected="${2:-}"
  if [[ "${RUN_STATUS}" -eq 0 ]]; then
    fact "${label}" "DID NOT REFUSE — the guard is broken and the operation went through"
    log "FATAL: ${label} was expected to be refused and was not. Stopping before"
    log "       any further stage compounds it."
    exit 1
  fi
  if [[ -n "${expected}" && "${RUN_OUTPUT}" != *"${expected}"* ]]; then
    fact "${label}" "WRONG FAILURE (exit ${RUN_STATUS}): $(printf '%s' "${RUN_OUTPUT}" | head -1)"
    log "FATAL: ${label} failed for an unexpected reason; expected to see '${expected}'."
    exit 1
  fi
  fact "${label}" "refused (exit ${RUN_STATUS}): $(printf '%s' "${RUN_OUTPUT}" | head -1)"
}

# assert_ok stops the stage when a step every later step depends on has failed,
# which `run` alone will not do: it captures the status rather than raising it.
assert_ok() {
  local label="$1"
  if [[ "${RUN_STATUS}" -ne 0 ]]; then
    fact "${label}" "FAILED (exit ${RUN_STATUS}): $(printf '%s' "${RUN_OUTPUT}" | head -2 | tr '\n' ' ')"
    log "FATAL: ${label} failed; the rest of this stage depends on it."
    exit 1
  fi
  fact "${label}" "ok"
}

require_repo_build() {
  if [[ ! -x "${OLK}" ]]; then
    log "FATAL: ${OLK} is missing. Build it with 'make build' in ${REPO_DIR}."
    log "       A fresh build is a new code-signing identity, so macOS will raise"
    log "       a Keychain prompt that a human has to answer with Always Allow."
    exit 1
  fi
  local version
  version="$("${OLK}" version 2>&1)"
  log "binary: ${OLK}"
  log "version: ${version}"
  if [[ -z "${EXPECTED_COMMIT}" ]]; then
    log "FATAL: could not read the checkout's commit, and EXPECTED_COMMIT is unset."
    log "       Set it to the commit the binary was built from, or run this from a"
    log "       git checkout so that it can be derived."
    exit 1
  fi
  if [[ "${version}" != *"${EXPECTED_COMMIT}"* ]]; then
    log "FATAL: expected a build at commit ${EXPECTED_COMMIT}; got the line above."
    log "       Run 'make build', or set EXPECTED_COMMIT if you are deliberately"
    log "       testing another build. A binary older than the delegated-write"
    log "       work accepts --mailbox on a send and ignores it, which makes"
    log "       every assertion here pass for the wrong reason."
    exit 1
  fi
}

# find_delivered answers "did the recipient get it", which is not the same
# question as "is it in the Inbox". A server-side rule can file a message the
# moment it lands, and the 24 August run recorded a false absence for exactly
# that reason: the message arrived, a rule moved it to Archive, and a poll of
# the Inbox alone never saw it. Poll the Inbox first, because that is the common
# case and the listing is cheap, then fall back to a mailbox-wide search.
find_delivered() {
  local account="$1" marker="$2" query="${3:-$2}"
  local hit json status
  if hit="$(find_by_subject "${account}" "" Inbox "${marker}")"; then
    printf '%s' "${hit}"
    return 0
  fi
  log "  ... not in the Inbox; searching the whole mailbox" >&2
  status=0
  json="$("${OLK}" --account "${account}" --no-input \
    mail search "${query}" -n 25 --concise --json --results-only 2>/dev/null)" || status=$?
  if [[ "${status}" -ne 0 ]]; then
    return 2
  fi
  hit="$(printf '%s' "${json}" | jq -c --arg m "${marker}" \
    'if type=="array" then (map(select(.subject != null and (.subject | contains($m)))) | first) else empty end' 2>/dev/null || true)"
  if [[ -n "${hit}" && "${hit}" != "null" ]]; then
    printf '%s' "${hit}"
    return 0
  fi
  return 1
}

# count_by_subject reports how many messages in a folder carry a marker. Reply
# and reply-all produce copies with byte-identical subjects, so "a matching
# message exists" cannot tell the two apart; only the change in the count can.
count_by_subject() {
  local account="$1" mailbox="$2" folder="$3" marker="$4"
  local json status=0
  if [[ -n "${mailbox}" ]]; then
    json="$("${OLK}" --account "${account}" --mailbox "${mailbox}" --no-input \
      mail list --folder "${folder}" -n 25 --concise --json --results-only 2>/dev/null)" || status=$?
  else
    json="$("${OLK}" --account "${account}" --no-input \
      mail list --folder "${folder}" -n 25 --concise --json --results-only 2>/dev/null)" || status=$?
  fi
  if [[ "${status}" -ne 0 ]]; then
    log "  ! listing ${folder} in ${mailbox:-own mailbox} failed" >&2
    return 2
  fi
  printf '%s' "${json}" | jq --arg m "${marker}" \
    'if type=="array" then (map(select(.subject != null and (.subject | contains($m)))) | length) else 0 end' \
    2>/dev/null || printf '0'
}

# wait_for_count polls until the count reaches a target, and returns non-zero if
# it never does. Exchange files sent copies asynchronously, so the answer taken
# immediately after a send is not the answer.
wait_for_count() {
  local account="$1" mailbox="$2" folder="$3" marker="$4" target="$5"
  local attempt now
  for attempt in 1 2 3 4 5 6 7 8 9 10; do
    if ! now="$(count_by_subject "${account}" "${mailbox}" "${folder}" "${marker}")"; then
      return 2
    fi
    if [[ "${now}" -ge "${target}" ]]; then
      printf '%s' "${now}"
      return 0
    fi
    log "  ... ${now}/${target} matching '${marker}' in ${folder} (attempt ${attempt}/10), waiting 6s" >&2
    sleep 6
  done
  printf '%s' "${now}"
  return 1
}

# newest_id reports the identifier of the most recent message in a folder. It
# answers "did anything new arrive here" without depending on a subject, which
# matters for a reply: a reply inherits the original's subject and so cannot be
# recognised by one. It is also indifferent to how full the folder is, unlike a
# count taken from a listing capped at a page.
newest_id() {
  local account="$1" mailbox="$2" folder="$3"
  local json
  if [[ -n "${mailbox}" ]]; then
    json="$("${OLK}" --account "${account}" --mailbox "${mailbox}" --no-input \
      mail list --folder "${folder}" -n 1 --concise --json --results-only 2>/dev/null || true)"
  else
    json="$("${OLK}" --account "${account}" --no-input \
      mail list --folder "${folder}" -n 1 --concise --json --results-only 2>/dev/null || true)"
  fi
  printf '%s' "${json}" | jq -r 'if type=="array" then (first | .id // "") else "" end' 2>/dev/null || true
}
