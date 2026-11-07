#!/usr/bin/env bash
# Builds the app, signed, and puts it in /Applications, replacing the copy
# there. SIGN_IDENTITY picks the certificate (security find-identity -v -p
# codesigning lists them); without one the build is signed ad hoc.
set -euo pipefail

cd "$(dirname "$0")/.."
app=build/darwin-arm64/telepath.app
dest=/Applications/telepath.app
identity=${SIGN_IDENTITY:--}

if pgrep -qf "$dest/"; then
	echo "telepath is running: quit it first." >&2
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
