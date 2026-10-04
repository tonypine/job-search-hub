#!/bin/bash
# Builds JobSearchHub.app. SwiftPM only emits a bare executable, so the bundle
# macOS needs is assembled here, then signed.
set -euo pipefail

cd "$(dirname "$0")/.."

APP_NAME="JobSearchHub"
BUNDLE_ID="com.tonypine.JobSearchHub"
VERSION="0.1.0"
APP_DIR="build/$APP_NAME.app"

echo "==> Building"
swift build -c release --product "$APP_NAME"
BINARY="$(swift build -c release --product "$APP_NAME" --show-bin-path)/$APP_NAME"

echo "==> Assembling $APP_DIR"
rm -rf "$APP_DIR"
mkdir -p "$APP_DIR/Contents/MacOS"
cp "$BINARY" "$APP_DIR/Contents/MacOS/$APP_NAME"

# The hub's command runs the company triage agent for "Add company"; the app
# starts it because the agent needs Claude Code on this Mac.
echo "==> Building the hub command"
mkdir -p "$APP_DIR/Contents/Resources"
(cd ../server && go build -o "../macos/$APP_DIR/Contents/Resources/hub" ./cmd/hub)

# Scripts/make-icon.swift draws the mark; iconutil packs its sizes.
echo "==> Drawing the icon"
ICON_TOOL="build/make-icon"
[ "$ICON_TOOL" -nt Scripts/make-icon.swift ] || swiftc -O Scripts/make-icon.swift -o "$ICON_TOOL"
rm -rf build/AppIcon.iconset
"$ICON_TOOL" iconset build/AppIcon.iconset
iconutil -c icns build/AppIcon.iconset -o "$APP_DIR/Contents/Resources/AppIcon.icns"

cat > "$APP_DIR/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key>
	<string>$APP_NAME</string>
	<key>CFBundleDisplayName</key>
	<string>Job Search Hub</string>
	<key>CFBundleIdentifier</key>
	<string>$BUNDLE_ID</string>
	<key>CFBundleExecutable</key>
	<string>$APP_NAME</string>
	<key>CFBundleIconFile</key>
	<string>AppIcon</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>CFBundleShortVersionString</key>
	<string>$VERSION</string>
	<key>CFBundleVersion</key>
	<string>$VERSION</string>
	<key>LSMinimumSystemVersion</key>
	<string>26.0</string>
	<!-- The hub is reached over plain HTTP on this machine or the local network. -->
	<key>NSAppTransportSecurity</key>
	<dict>
		<key>NSAllowsLocalNetworking</key>
		<true/>
	</dict>
</dict>
</plist>
PLIST
plutil -lint "$APP_DIR/Contents/Info.plist" >/dev/null

echo "==> Signing"
# An Apple-issued identity keeps the app's designated requirement stable across
# rebuilds, so its Keychain access survives them; a self-signed or ad-hoc
# signature makes macOS ask again after every build. CODESIGN_IDENTITY picks
# one; otherwise the first Apple Development identity in the keychain is used.
IDENTITIES="$(security find-identity -v -p codesigning)"
IDENTITY="${CODESIGN_IDENTITY:-$(awk -F'"' '/"Apple Development: /{print $2; exit}' <<<"$IDENTITIES")}"
if [ -z "$IDENTITY" ] || ! grep -qF "$IDENTITY" <<<"$IDENTITIES"; then
  echo "No signing identity matching \"${IDENTITY:-Apple Development}\". Create an Apple Development certificate in" >&2
  echo "Xcode > Settings > Accounts > Manage Certificates, or set CODESIGN_IDENTITY." >&2
  exit 1
fi
codesign --force --options runtime --sign "$IDENTITY" "$APP_DIR/Contents/Resources/hub"
codesign --force --options runtime --sign "$IDENTITY" "$APP_DIR"
codesign --verify --strict "$APP_DIR"

echo "==> Built $APP_DIR"
