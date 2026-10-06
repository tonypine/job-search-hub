#!/bin/bash
# Builds hub-cvprint, which the server prints CVs with, into the given path,
# stamped with the hub's version (scripts/release/version.sh, which reads
# HUB_VERSION_CODE). A bare executable has no bundle, so its Info.plist is
# linked into its __TEXT,__info_plist section, where Bundle.main reads it.
#
#   Scripts/build-cvprint.sh <output>
set -euo pipefail

[ $# -eq 1 ] || { echo "usage: $0 <output>" >&2; exit 2; }
# The output is relative to the caller, not to macos/.
case $1 in /*) output=$1 ;; *) output=$PWD/$1 ;; esac
cd "$(dirname "$0")/.."

VERSION="$(sh ../scripts/release/version.sh)"
mkdir -p .build
# SwiftPM doesn't track a file passed to the linker, so the version is in the
# plist's path: a new version changes the linker flags, which forces a relink.
rm -f .build/hub-cvprint-*Info.plist
PLIST="$PWD/.build/hub-cvprint-$VERSION-Info.plist"
cat > "$PLIST" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleIdentifier</key>
	<string>com.tonypine.JobSearchHub.cvprint</string>
	<key>CFBundleName</key>
	<string>hub-cvprint</string>
	<key>CFBundleShortVersionString</key>
	<string>$VERSION</string>
	<key>CFBundleVersion</key>
	<string>$VERSION</string>
</dict>
</plist>
PLIST
plutil -lint "$PLIST" >/dev/null

LINK=(-Xlinker -sectcreate -Xlinker __TEXT -Xlinker __info_plist -Xlinker "$PLIST")
swift build -c release --product hub-cvprint "${LINK[@]}" >/dev/null
mkdir -p "$(dirname "$output")"
cp "$(swift build -c release --product hub-cvprint "${LINK[@]}" --show-bin-path)/hub-cvprint" "$output"
