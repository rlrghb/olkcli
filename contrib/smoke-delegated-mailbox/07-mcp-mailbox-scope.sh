#!/usr/bin/env bash
# Stage 7 — the agent-facing half of PR 89.
#
# The pull request did two things: it made the four write paths honour
# --mailbox, and it gave the MCP server a mailbox policy — a launch-time
# --mailbox that every tool call inherits, and an --allow-mailbox list naming
# the mailboxes a caller may redirect to. Stages 1 to 5 cover the first half.
# This stage covers the second, which is the surface an agent actually reaches.
#
# Nothing here sends: every probe is a tools/list, or a call with --no-send set,
# so the server refuses before anything leaves the building.

set -euo pipefail
# shellcheck source=lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

require_repo_build
section "Stage 7 — MCP mailbox scoping, run ${RUN_ID}"

# One JSON-RPC exchange over stdio. The server speaks the protocol on stdin and
# stdout, so the request goes in on a pipe and the framed reply comes back out.
mcp_call() {
  local body="$1"
  shift
  # The pause matters: the server shuts down on stdin EOF, and without it the
  # pipe closes before the reply is written — which reads as an empty tool list
  # rather than as the timing problem it is.
  {
    printf '%s\n' \
      '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"smoke","version":"0"}}}' \
      '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
      "${body}"
    sleep "${MCP_SETTLE:-5}"
  } | "${OLK}" --account "${SEND_ACCOUNT}" --no-input mcp "$@" 2>/dev/null || true
}

# assert_tools_listed catches the failure mode above: every probe in this stage
# reads a tool list, and an empty one looks identical to a correct refusal.
assert_tools_listed() {
  local count="$1"
  if [[ "${count}" -eq 0 ]]; then
    log "FATAL: the server returned no tools at all, so nothing below can be"
    log "       distinguished from a refusal. Raise MCP_SETTLE and rerun."
    exit 1
  fi
}

section "Which mail tools does a default launch expose?"
# Read-only by default: the send tier should be absent until it is asked for.
OUT="$(mcp_call '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}')"
TOOLS="$(printf '%s' "${OUT}" | jq -rs '[.[] | select(.id==2) | .result.tools[]?.name] | sort | join(",")' 2>/dev/null || true)"
DEFAULT_COUNT="$(printf '%s' "${TOOLS}" | tr ',' '\n' | grep -c . || true)"
assert_tools_listed "${DEFAULT_COUNT}"
fact "mcp.default.tool_count" "${DEFAULT_COUNT}"
fact "mcp.default.mail_send_exposed" "$(printf '%s' "${TOOLS}" | grep -q 'mail_send' && echo 'EXPOSED — should be off by default' || echo 'no, as expected')"

section "Does --allow-send expose the send tier?"
OUT="$(mcp_call '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' --allow-send mail_send)"
TOOLS="$(printf '%s' "${OUT}" | jq -rs '[.[] | select(.id==2) | .result.tools[]?.name] | sort | join(",")' 2>/dev/null || true)"
assert_tools_listed "$(printf '%s' "${TOOLS}" | tr ',' '\n' | grep -c . || true)"
fact "mcp.allow_send.mail_send_exposed" "$(printf '%s' "${TOOLS}" | grep -q 'mail_send' && echo 'yes, as expected' || echo 'NO — --allow-send did not expose it')"

section "Does --no-send outrank --allow-send?"
# The capability guards are enforced in the graphapi client, so they should hold
# regardless of what the server was told to expose.
OUT="$(mcp_call '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' --allow-send mail_send --no-send)"
TOOLS="$(printf '%s' "${OUT}" | jq -rs '[.[] | select(.id==2) | .result.tools[]?.name] | sort | join(",")' 2>/dev/null || true)"
assert_tools_listed "$(printf '%s' "${TOOLS}" | tr ',' '\n' | grep -c . || true)"
fact "mcp.no_send_beats_allow_send" "$(printf '%s' "${TOOLS}" | grep -q 'mail_send' && echo 'EXPOSED — guard did not apply' || echo 'not exposed, as expected')"

section "A malformed --allow-mailbox is refused at launch"
# Refused outright rather than dropped with a warning: a mistyped mailbox would
# otherwise leave agents believing a mailbox is reachable when no call can name it.
# Validation happens in Run, so the server has to be started for real; --help
# would short-circuit before the flag is ever parsed and prove nothing.
OUT="$(mcp_call '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' --allow-mailbox "not-an-address")"
if printf '%s' "${OUT}" | jq -rs '[.[] | select(.id==2) | .result.tools[]?.name] | length' 2>/dev/null | grep -qv '^0$'; then
  fact "mcp.malformed_allow_mailbox" "STARTED ANYWAY — a mistyped mailbox was accepted"
else
  fact "mcp.malformed_allow_mailbox" "refused to start, as expected"
fi

section "Launch-time --mailbox reaches a tool call"
# With a launch mailbox set and no --allow-mailbox, every call should act on
# that mailbox. mail_list is read-only, so this is safe to run for real.
# The tool takes `top`, not the CLI's `-n`: MCP argument names follow the long
# flags. A wrong name comes back as invalid_input rather than as an empty
# result, so the reply is checked for an error before anything is concluded.
mcp_mail_list() {
  local out
  out="$(mcp_call '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"mail_list","arguments":{"top":3,"concise":true}}}' "$@")"
  printf '%s' "${out}" | jq -rs '[.[] | select(.id==2)] | first | (.result.content[0].text // .error.message // "no reply")' 2>/dev/null || echo "unparseable"
}

SCOPED="$(mcp_mail_list --mailbox "${SHARED_MAILBOX}")"
UNSCOPED="$(mcp_mail_list)"
printf 'scoped:\n%s\nunscoped:\n%s\n' "${SCOPED:0:800}" "${UNSCOPED:0:800}" >>"${LOG}"

if printf '%s' "${SCOPED}" | grep -q '"code"'; then
  fact "mcp.launch_mailbox_applied" "tool call errored: $(printf '%s' "${SCOPED}" | head -c 120)"
else
  # Two listings of two different mailboxes should not return the same message
  # ids. Identical ids would mean the launch mailbox was accepted and ignored —
  # the same silent failure on the agent surface that PR 89 fixed on the CLI.
  SCOPED_IDS="$(printf '%s' "${SCOPED}" | jq -rc '[.. | objects | .id? // empty] | sort' 2>/dev/null || echo '[]')"
  UNSCOPED_IDS="$(printf '%s' "${UNSCOPED}" | jq -rc '[.. | objects | .id? // empty] | sort' 2>/dev/null || echo '[]')"
  fact "mcp.scoped_senders" "$(printf '%s' "${SCOPED}" | jq -rc '[.. | objects | .from? // empty] | unique' 2>/dev/null || echo '?')"
  fact "mcp.unscoped_senders" "$(printf '%s' "${UNSCOPED}" | jq -rc '[.. | objects | .from? // empty] | unique' 2>/dev/null || echo '?')"
  if [[ "${SCOPED_IDS}" == "[]" ]]; then
    fact "mcp.launch_mailbox_applied" "inconclusive — no message ids parsed from the reply"
  elif [[ "${SCOPED_IDS}" == "${UNSCOPED_IDS}" ]]; then
    fact "mcp.launch_mailbox_applied" "NO — --mailbox at launch returned the caller's own mail"
  else
    fact "mcp.launch_mailbox_applied" "yes — the scoped listing differs from the unscoped one"
  fi
fi

section "Stage 7 complete — facts in ${FACTS}"
