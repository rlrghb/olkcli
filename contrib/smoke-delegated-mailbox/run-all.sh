#!/usr/bin/env bash
# Run the stages in order under one run identifier, so every result lands in one
# directory. Each stage that sends asks first; SMOKE_ASSUME_YES=1 answers yes in
# advance, which is worth thinking twice about — these send from a live team
# identity to real people.
#
#   source ./config.sh && ./run-all.sh
#
# Stage 0 is read-only, is safe to run on its own, and is the right first move.
# See README.md for the variables and the Exchange permissions they assume.

set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RUN_ID="${RUN_ID:-$(date -u +%Y%m%dT%H%M%SZ)}"
export RUN_ID

# 07 is read-only and independent of the mail stages, so it runs early: a
# failure there is worth knowing before anything sends.
STAGES=(00-preflight 07-mcp-mailbox-scope 01-send 02-drafts 03-reply-forward 05-negative 04-delegation-type 06-cleanup-and-report)

for stage in "${STAGES[@]}"; do
  printf '\n################ %s\n' "${stage}"
  if ! "${HERE}/${stage}.sh"; then
    printf '\n%s failed; stopping. Tidy up with:\n  RUN_ID=%s %s/06-cleanup-and-report.sh\n' \
      "${stage}" "${RUN_ID}" "${HERE}" >&2
    exit 1
  fi
done

printf '\n################ done\n'
printf 'results: %s/results/%s/\n' "${HERE}" "${RUN_ID}"
