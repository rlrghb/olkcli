#!/usr/bin/env bash
# Create a self-signed code-signing certificate for local olk builds.
#
# One-time setup for scripts/macos-dev-sign.sh. The certificate and its key go
# into the login keychain, with codesign allowed to use the key. macOS then
# asks once for your password to trust the certificate for code signing.
#
# Usage: scripts/macos-dev-cert.sh [certificate name]   (default olk-dev-signer)
set -euo pipefail

name="${1:-olk-dev-signer}"
keychain="$HOME/Library/Keychains/login.keychain-db"

die() {
	printf 'macos-dev-cert: %s\n' "$*" >&2
	exit 1
}

[[ "$(uname -s)" == Darwin ]] ||
	die "this creates a macOS keychain certificate; nothing to do on $(uname -s)."
command -v openssl >/dev/null || die "openssl not found. Install it with: brew install openssl"
if security find-certificate -c "$name" "$keychain" >/dev/null 2>&1; then
	die "a certificate named '$name' already exists." \
		"Use it with OLK_CODESIGN_IDENTITY='$name', or pass another name."
fi

workdir="$(mktemp -d)"
cleanup() {
	rm -f "$workdir/key.pem" "$workdir/cert.pem" "$workdir/identity.p12"
	rmdir "$workdir"
}
trap cleanup EXIT

openssl req -x509 -newkey rsa:2048 -nodes -days 3650 \
	-subj "/CN=$name" \
	-addext "basicConstraints=critical,CA:false" \
	-addext "keyUsage=critical,digitalSignature" \
	-addext "extendedKeyUsage=critical,codeSigning" \
	-keyout "$workdir/key.pem" -out "$workdir/cert.pem" 2>/dev/null ||
	die "openssl could not create the certificate."

# The bundle only carries the key into the keychain, so its password is
# throwaway. OpenSSL 3 needs -legacy for encryption that `security` can read;
# LibreSSL, which macOS ships as /usr/bin/openssl, has no such flag. The
# guarded expansion below keeps an empty array legal under bash 3.2 and set -u.
legacy=()
if openssl version | grep -q '^OpenSSL 3'; then
	legacy=(-legacy)
fi
bundle_password="$(openssl rand -hex 16)"
openssl pkcs12 -export ${legacy[@]+"${legacy[@]}"} -name "$name" \
	-inkey "$workdir/key.pem" -in "$workdir/cert.pem" \
	-out "$workdir/identity.p12" -passout "pass:$bundle_password" ||
	die "openssl could not bundle the key and certificate."

security import "$workdir/identity.p12" -k "$keychain" -P "$bundle_password" \
	-T /usr/bin/codesign ||
	die "security import failed; is the login keychain unlocked?"
security add-trusted-cert -r trustRoot -p codeSign -k "$keychain" "$workdir/cert.pem" ||
	die "the certificate was imported but not trusted." \
		"Trust '$name' for code signing in Keychain Access."

printf "Created '%s'. Sign development builds with:\n" "$name"
printf "  OLK_CODESIGN_IDENTITY='%s' make build sign\n" "$name"
