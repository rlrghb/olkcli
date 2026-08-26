#!/usr/bin/env bash
# Copy to config.sh, fill in, and source before running any stage:
#
#   cp config.example.sh config.sh
#   $EDITOR config.sh
#   source ./config.sh && ./run-all.sh
#
# config.sh is ignored by git. Nothing here is secret, but mailbox addresses
# name real people and belong in nobody's commit history but your own.

# Required. The signed-in account that holds the delegation. It must already be
# authenticated: `olk auth login --enterprise --scope Mail.Send.Shared`.
export SEND_ACCOUNT="delegate@example.com"

# Required. The shared mailbox to act as. The signed-in account needs all three
# of Mail.Send.Shared in the token, Send As or Send on Behalf Of in Exchange,
# and Full Access on the mailbox. Holding one implies nothing about the others,
# and Graph reports a missing Full Access grant as an unrelated-looking
# "ErrorItemNotFound".
export SHARED_MAILBOX="shared-mailbox@example.com"

# Required. Where the test messages are sent. Use an account you can read, since
# several checks confirm what actually arrived.
export RECIPIENT="recipient@example.com"

# Optional. A second shared mailbox whose delegation was granted separately.
# Worth setting if the two mailboxes differ in how they were delegated, because
# olk prints the same From line for Send As and Send on Behalf Of and only the
# received headers tell them apart. Stages skip it when unset.
# export CONTROL_MAILBOX="second-mailbox@example.com"

# Optional. A mailbox the signed-in account has no permission on at all, used to
# record what a refusal looks like. Stage 5 sends as it deliberately, so pick
# something inert: never a compliance, incident, payments or legal queue. The
# stage re-checks that the mailbox is genuinely unreachable before probing, and
# skips itself when unset.
# export NO_ACCESS_MAILBOX="no-access-mailbox@example.com"

# Optional. Defaults to bin/olk in the checkout. Point it elsewhere only if you
# know the binary was built from the commit you are testing.
# export OLK="/path/to/olk"

# Optional. Defaults to the checkout's HEAD. Set it when testing a binary built
# from a different commit; the preflight refuses to run against a mismatch.
# export EXPECTED_COMMIT="abc1234"

# Optional. Answers every send confirmation with yes. Think twice: the stages
# send real mail from a live identity to real people, and it cannot be recalled.
# export SMOKE_ASSUME_YES=1
