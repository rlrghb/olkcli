#!/usr/bin/env bash
# Sign a development build of olk with a stable identity.
#
# macOS ties a "Always Allow" Keychain grant to the binary's designated
# requirement: its signing identifier plus its certificate. The ad-hoc
# signature that `go build` applies changes with every build, so each rebuild
# would ask for Keychain access again. Signing every build with the same
# certificate and identifier keeps one grant for all of them.
#
# Usage: OLK_CODESIGN_IDENTITY=<name or SHA-1> scripts/macos-dev-sign.sh [binary]
# Optional: OLK_CODESIGN_IDENTIFIER (default com.rlrghb.olk.dev).
set -euo pipefail

binary="${1:-bin/olk}"
identity="${OLK_CODESIGN_IDENTITY:-}"
identifier="${OLK_CODESIGN_IDENTIFIER:-com.rlrghb.olk.dev}"

die() {
	printf 'macos-dev-sign: %s\n' "$*" >&2
	exit 1
}

[[ "$(uname -s)" == Darwin ]] ||
	die "code signing applies to macOS only; nothing to do on $(uname -s)."
command -v codesign >/dev/null ||
	die "codesign not found. Install the Xcode Command Line Tools: xcode-select --install"
[[ -x "$binary" ]] || die "$binary is missing or not executable. Run make build first."

if [[ -z "$identity" ]]; then
	printf 'macos-dev-sign: OLK_CODESIGN_IDENTITY is not set. %s\n' \
		'Code-signing identities in your keychains:' >&2
	security find-identity -v -p codesigning >&2 || true
	die "set OLK_CODESIGN_IDENTITY to one of the names or SHA-1 hashes above," \
		"or create a local certificate with scripts/macos-dev-cert.sh."
fi

identities="$(security find-identity -v -p codesigning)" ||
	die "security find-identity failed; unlock the login keychain and run make sign again."

# Resolve the identity to one certificate hash. Matching the whole hash field
# or the whole quoted name keeps a prefix such as "olk" from passing, and
# signing by hash keeps two certificates that share a name from reaching
# codesign as an ambiguous name. The same certificate listed once per keychain
# has one hash and is accepted.
hashes="$(awk -v want="$identity" '
	$1 ~ /^[0-9]+\)$/ {
		name = $0
		sub(/^[^"]*"/, "", name)
		sub(/"[^"]*$/, "", name)
		if (toupper($2) == toupper(want) || name == want) print toupper($2)
	}' <<<"$identities" | sort -u)"
if [[ -z "$hashes" ]]; then
	die "no valid code-signing identity matches '$identity'." \
		"List them with: security find-identity -v -p codesigning"
fi
if [[ "$hashes" == *$'\n'* ]]; then
	die "several certificates are named '$identity'. Set OLK_CODESIGN_IDENTITY to one of" \
		"these SHA-1 hashes instead: ${hashes//$'\n'/ }"
fi
hash="$hashes"

# Refuse anything that is not a development-namespace build. A release binary
# signed with a local certificate would share the installed olk's Keychain
# items under a different identity, which is the prompt this script exists to
# avoid. Older releases print no namespace at all, so a missing value refuses
# too. `olk version` does not open the credential store.
if ! version_json="$("$binary" version --json 2>/dev/null)"; then
	die "$binary version --json failed; is $binary an olk binary?"
fi
namespace="$(plutil -extract namespace raw -o - - <<<"$version_json" 2>/dev/null)" ||
	namespace=""
case "$namespace" in
"")
	die "$binary does not report a storage namespace, so it may be a release build." \
		"Rebuild with make build."
	;;
olk)
	die "$binary uses the release namespace 'olk'." \
		"Only development builds (make build) may be signed locally."
	;;
esac

codesign --force --sign "$hash" --identifier "$identifier" "$binary" ||
	die "codesign failed. If macOS asked for access to the signing key," \
		"allow codesign and run make sign again."
codesign --verify --strict "$binary" ||
	die "the signature on $binary did not verify." \
		"Check that the certificate is trusted for code signing."

printf 'Signed %s (namespace %s) as %s.\n' "$binary" "$namespace" "$identifier"
printf 'Keychain grants match this designated requirement:\n'
requirement="$(codesign -d -r- "$binary" 2>&1)" ||
	die "the binary is signed, but codesign could not display its designated requirement:" \
		"$requirement"
sed -n 's/^designated => /  /p' <<<"$requirement"
