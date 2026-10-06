#!/bin/bash
# Builds JobSearchHub.app. SwiftPM only emits a bare executable, so the bundle
# macOS needs is assembled here, then signed. The app and its hub command carry
# the hub's one version, from scripts/release/version.sh: 0.1.<HUB_VERSION_CODE>
# for a release, 0.1.0-dev.<short commit> otherwise.
set -euo pipefail

cd "$(dirname "$0")/.."

APP_NAME="JobSearchHub"
BUNDLE_ID="com.tonypine.JobSearchHub"
VERSION="$(sh ../scripts/release/version.sh)"
APP_DIR="build/$APP_NAME.app"

echo "==> Building"
swift build -c release --product "$APP_NAME"
BINARY="$(swift build -c release --product "$APP_NAME" --show-bin-path)/$APP_NAME"

echo "==> Assembling $APP_DIR"
rm -rf "$APP_DIR"
mkdir -p "$APP_DIR/Contents/MacOS"
cp "$BINARY" "$APP_DIR/Contents/MacOS/$APP_NAME"

# The hub's command runs the company triage agent for "Add company"; the app
# starts it because the agent needs Claude Code on this Mac. Without Go, as in
# Symphony's QA VM, the app is built without it, and those features say so.
mkdir -p "$APP_DIR/Contents/Resources"
if command -v go >/dev/null; then
  echo "==> Building the hub command"
  GO_LDFLAGS="$(sh ../scripts/release/version.sh --go-ldflags)"
  (cd ../server && go build -ldflags "$GO_LDFLAGS" -o "../macos/$APP_DIR/Contents/Resources/hub" ./cmd/hub)
else
  echo "No Go toolchain: building without the bundled hub CLI; features that run it won't work in this build." >&2
fi

# Scripts/make-icon.swift draws the mark; iconutil packs its sizes.
echo "==> Drawing the icon"
ICON_TOOL="build/make-icon"
[ "$ICON_TOOL" -nt Scripts/make-icon.swift ] || swiftc -O Scripts/make-icon.swift -o "$ICON_TOOL"
rm -rf build/AppIcon.iconset
"$ICON_TOOL" iconset build/AppIcon.iconset
iconutil -c icns build/AppIcon.iconset -o "$APP_DIR/Contents/Resources/AppIcon.icns"

# Hub Indigo as the app's accent color, which SwiftUI's tint doesn't reach:
# the sidebar selection, focus rings and default buttons. actool compiles the
# color set into Assets.car, which NSAccentColorName below names.
echo "==> Compiling the accent color"
xcrun actool Assets/Assets.xcassets --compile "$APP_DIR/Contents/Resources" \
  --platform macosx --minimum-deployment-target 26.0 --accent-color AccentColor \
  --output-partial-info-plist build/assets-info.plist >/dev/null

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
	<key>NSAccentColorName</key>
	<string>AccentColor</string>
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
# Without one, as in Symphony's QA VM, or with CODESIGN_IDENTITY=-, the app is
# signed ad hoc, and without the hardened runtime, which only notarization needs.
IDENTITIES="$(security find-identity -v -p codesigning)"
IDENTITY="${CODESIGN_IDENTITY:-$(awk -F'"' '/"Apple Development: /{print $2; exit}' <<<"$IDENTITIES")}"
if [ -z "$IDENTITY" ]; then
  echo "No Apple Development identity in the keychain, so signing ad hoc: the Keychain will ask for access after" >&2
  echo "every build. Create one in Xcode > Settings > Accounts > Manage Certificates, or set CODESIGN_IDENTITY." >&2
  IDENTITY="-"
fi
if [ "$IDENTITY" = "-" ]; then
  SIGN_OPTIONS=(--sign -)
elif grep -qF "$IDENTITY" <<<"$IDENTITIES"; then
  SIGN_OPTIONS=(--options runtime --sign "$IDENTITY")
else
  echo "No signing identity matching \"$IDENTITY\". Create an Apple Development certificate in" >&2
  echo "Xcode > Settings > Accounts > Manage Certificates, or set CODESIGN_IDENTITY (- signs ad hoc)." >&2
  exit 1
fi
if [ -f "$APP_DIR/Contents/Resources/hub" ]; then
  codesign --force "${SIGN_OPTIONS[@]}" "$APP_DIR/Contents/Resources/hub"
fi
codesign --force "${SIGN_OPTIONS[@]}" "$APP_DIR"
codesign --verify --strict "$APP_DIR"

echo "==> Built $APP_DIR $VERSION"
