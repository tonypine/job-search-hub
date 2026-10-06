#!/bin/bash
# Builds hub-cvprint, which the server prints CVs with, into the given path,
# stamped with the hub's version (scripts/release/version.sh, which reads
# HUB_VERSION_CODE). A bare executable has no bundle, so its Info.plist is
# linked into its __TEXT,__info_plist section, where Bundle.main reads it.
#
#   Scripts/build-cvprint.sh <output>
set -euo pipefail

[ $# -eq 1 ] || { echo "usage: $0 <output>" >&2; exit 2; }
output=$1
cd "$(dirname "$0")/.."

VERSION="$(sh ../scripts/release/version.sh)"
mkdir -p .build
# A fixed path keeps the linker flags the same from build to build.
PLIST="$PWD/.build/hub-cvprint-Info.plist"
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
