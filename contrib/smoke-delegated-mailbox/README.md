# Delegated-mailbox smoke test

A live-tenant harness for the `--mailbox` write paths: send, draft create and
send, reply, reply-all and forward as a shared mailbox.

None of this can be covered by unit tests. Whether a delegated send succeeds is
decided by Exchange permissions and token scopes rather than by anything in this
repository, and where the sent copy is filed is decided by a mailbox setting on
the tenant. The only way to know is to send a message and look.

**These stages send real mail from a real identity to real people, and it cannot
be recalled.** Every send asks first. `SMOKE_ASSUME_YES=1` removes that gate and
is deliberately not the default.

## Setup

```bash
make build                       # the harness refuses any other binary
cd contrib/smoke-delegated-mailbox
cp config.example.sh config.sh
$EDITOR config.sh
source ./config.sh
./00-preflight.sh                # read-only; start here
```

`00-preflight.sh` sends nothing. It confirms the binary, proves which mailboxes
answer, and checks that `--dry-run` names the shared mailbox rather than the
signed-in account. If a dry run names the wrong identity, stop: the binary is
not the one you think it is, and the sending stages would send as the wrong
sender.

Then either walk the stages individually or run the lot:

```bash
./run-all.sh
```

Each run gets an identifier, every subject carries it as `OLK-SMOKE-<id>`, and
results land in `results/<id>/` as a transcript and a table of recorded facts.

## What each stage does

| Stage | Sends? | Covers |
|---|---|---|
| `00-preflight` | no | binary identity, delegated reads, dry runs, `--no-write` and `--no-send`, Sent Items baseline |
| `07-mcp-mailbox-scope` | no | tool exposure by tier, `--allow-send`, `--no-send` precedence, launch-time `--mailbox` |
| `01-send` | yes | `mail send` as the mailbox; `from`, Sent Items placement, delivery |
| `02-drafts` | yes | draft created in the mailbox's Drafts, sent from it, removed after |
| `03-reply-forward` | yes | reply, reply-all, forward; threading; identifier scoping |
| `05-negative` | attempts | what a refusal looks like on a mailbox with no delegation |
| `04-delegation-type` | no | guided reading of the received headers, by a human |
| `06-cleanup-and-report` | no | removes what it can, names what it cannot, prints the facts |

## The three grants

A delegated send needs all of the following, and holding one implies nothing
about the others:

1. **`Mail.Send.Shared`** in the token. It is not in the default scope set;
   request it at login with `--scope Mail.Send.Shared`.
2. **Send As** or **Send on Behalf Of** on the mailbox, granted in Exchange.
3. **Full Access** on the mailbox. Microsoft requires it for
   `/users/{mailbox}/sendMail` even though the name suggests otherwise.

A missing Full Access grant does not say so. It surfaces as:

```
ErrorItemNotFound: The specified object was not found in the store.
```

## Send As versus Send on Behalf Of

Stage 4 has a human read the received headers, because olk cannot answer this:
the `sender` field is not an available selector, there is no raw message output,
and Graph exposes no mailbox permissions at all.

It also cannot be answered by inference. **Exchange resolves Send As ahead of
Send on Behalf Of when a delegate holds both**, so observing Send As is equally
consistent with "only Send As is granted" and with "both are granted".
Observing Send As never proves Send on Behalf Of is absent.

Report what a message did and what a mailbox was granted as two separate
findings. The grants are Exchange-side:

```powershell
Get-RecipientPermission <mailbox>                  # Send As
Get-Mailbox <mailbox> | fl GrantSendOnBehalfTo     # Send on Behalf Of
Get-MailboxPermission <mailbox>                    # Full Access
```

## Reading the results

Three outcomes look alike in an exit status and are not alike at all:

- **Refused.** olk or Graph rejected the request; nothing was sent.
- **Timed out.** The request reached Graph and the client gave up waiting. What
  the server did is unknown, and only the mailbox can settle it. The harness
  detects this and re-reads Sent Items rather than calling it a refusal.
- **Not found where expected.** A message can be delivered and filed out of the
  Inbox by a rule within seconds. An Inbox-only check reports that as a
  non-delivery, so the delivery checks fall back to a mailbox-wide search.

## Cleanup

`06-cleanup-and-report` deletes the drafts it can reach and then names what it
cannot. `mail delete` and `mail move` are scoped to the signed-in user and
ignore `--mailbox`, so anything filed in a shared mailbox has to be removed by
hand. Search for the run identifier, which every subject carries.
