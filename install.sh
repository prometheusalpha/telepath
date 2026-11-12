#!/usr/bin/env bash
# Builds the app, signed, and puts it in /Applications, replacing the copy
# there. SIGN_IDENTITY picks the certificate (security find-identity -v -p
# codesigning lists them). By default it is the self-signed "Telepath Local
# Signing" where the keychain has it, else ad hoc: a certificate keeps the
# app's signature the same from build to build, so the Keychain doesn't ask
# again for the secrets' key after each one.
set -euo pipefail

cd "$(dirname "$0")"
app=build/darwin-arm64/Telepath.app
dest=/Applications/Telepath.app
local_identity="Telepath Local Signing"
if [[ -z ${SIGN_IDENTITY:-} ]] && security find-identity -v -p codesigning | grep -q "\"$local_identity\""; then
	SIGN_IDENTITY=$local_identity
fi
identity=${SIGN_IDENTITY:--}

if pgrep -qf "$dest/"; then
	echo "Telepath is running: quit it first." >&2
	exit 1
fi

go tool mygo build -skip-dmg -sign "$identity"
# Removed first: ditto merges into what is there, and would leave files
# the new build no longer has.
rm -rf "$dest"
ditto "$app" "$dest"
codesign --verify --deep --strict "$dest"
codesign -dv "$dest" 2>&1 | grep -E '^(Signature|Authority)=' | head -1
echo "Installed $dest"
