#!/bin/bash
# Builds JobSearchHub.app, the bundle one version of the hub is. SwiftPM only
# emits bare executables, so the bundle macOS needs is assembled here, then
# signed:
#
#   Contents/MacOS/JobSearchHub                    the app
#   Contents/Helpers/bin/hub-server                the server, which launchd runs from here
#   Contents/Helpers/bin/hub-cvprint               the CV printer, which the server finds beside itself
#   Contents/Helpers/bin/hub                       the hub command
#   Contents/Helpers/bin/hub-update                the installer
#   Contents/Helpers/bin/hub-install-steps         the window hub-update shows the steps in while the app is closed
#
# install-app.sh writes the server's agent to ~/Library/LaunchAgents, naming
# the installed bundle's hub-server.
#
# Contents/Helpers/engines/ is left for the owned database's engine, which
# the server looks for beside its own folder; until the bundle carries it, the
# server uses the one installed in ~/Library/Application Support/JobSearchHub.
#
# Every part carries the hub's one version, from scripts/release/version.sh:
# 0.1.<HUB_VERSION_CODE> for a release, 0.1.0-dev.<short commit> otherwise.
# The Go commands are built for darwin/arm64. macos/Scripts/install-app.sh
# installs the result in ~/Applications.
set -euo pipefail

cd "$(dirname "$0")/.."

APP_NAME="JobSearchHub"
BUNDLE_ID="com.tonypine.JobSearchHub"
VERSION="$(sh ../scripts/release/version.sh)"
APP_DIR="build/$APP_NAME.app"
HELPERS="$APP_DIR/Contents/Helpers/bin"

echo "==> Building"
swift build -c release --product "$APP_NAME"
BINARY="$(swift build -c release --product "$APP_NAME" --show-bin-path)/$APP_NAME"

echo "==> Assembling $APP_DIR"
rm -rf "$APP_DIR"
mkdir -p "$APP_DIR/Contents/MacOS"
cp "$BINARY" "$APP_DIR/Contents/MacOS/$APP_NAME"

mkdir -p "$APP_DIR/Contents/Resources"

# The server and its commands. The app also runs hub, for the company triage
# agent behind "Add company" and other work that needs Claude Code on this
# Mac. Without Go, as in Symphony's QA VM, the app is built alone: Settings ›
# Server says this build has no server, and the features that run hub say so.
if command -v go >/dev/null; then
  GO_LDFLAGS="$(sh ../scripts/release/version.sh --go-ldflags)"
  mkdir -p "$HELPERS"
  for command in hub-server hub hub-update; do
    echo "==> Building $command"
    (cd ../server && GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags "$GO_LDFLAGS" -o "../macos/$HELPERS/$command" "./cmd/$command")
  done
  echo "==> Building hub-cvprint"
  Scripts/build-cvprint.sh "$HELPERS/hub-cvprint"
  echo "==> Building hub-install-steps"
  swift build -c release --product hub-install-steps >/dev/null
  cp "$(swift build -c release --product hub-install-steps --show-bin-path)/hub-install-steps" "$HELPERS/hub-install-steps"
else
  echo "No Go toolchain: building the app without the server and the hub command; features that run them won't work in this build." >&2
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

# Scripts/signing-identity.sh picks the owner's team and its identity: pinned
# by CODESIGN_TEAM_ID or ~/.config/job-search-hub/codesign-team-id, else the
# keychain's one Apple Development identity, whose team it pins. Without one,
# as in CI and Symphony's QA VM, or with CODESIGN_IDENTITY=-, it's ad hoc,
# without the hardened runtime.
SIGNING="$(Scripts/signing-identity.sh)"
read -r IDENTITY TEAM_ID <<<"$SIGNING"

# A build with neither a team nor Go, as in Symphony's QA VM, is a QA build:
# the app runs in QA mode at every launch, starting from empty connection
# settings. HUB_QA_BUILD=1 or 0 decides it instead.
QA_BUILD="${HUB_QA_BUILD:-}"
if [ -z "$QA_BUILD" ]; then
  if [ "$IDENTITY" = "-" ] && ! command -v go >/dev/null; then QA_BUILD=1; else QA_BUILD=0; fi
fi
QA_BUILD_PLIST=""
if [ "$QA_BUILD" = "1" ]; then
  echo "==> QA build: the app starts from empty connection settings at every launch"
  QA_BUILD_PLIST="<key>HubQABuild</key>
	<true/>"
  # The stub release feed QA serves (README › Releases), since Symphony's
  # launcher passes the app no environment: HUB_RELEASES_URL at build time
  # goes in Info.plist, where the app reads it when the variable isn't set at
  # launch. QA's ports change on every pass, so the build that knows the port
  # names the URL; without one the build reads GitHub's releases.
  RELEASES_URL="${HUB_RELEASES_URL:-}"
  if [ -n "$RELEASES_URL" ]; then
    case "$RELEASES_URL" in
      http://*|https://*) ;;
      *) echo "HUB_RELEASES_URL must be an http(s) URL: $RELEASES_URL" >&2; exit 1 ;;
    esac
    echo "==> QA build: the app reads its releases from $RELEASES_URL"
    RELEASES_URL_XML="$(printf '%s' "$RELEASES_URL" | sed -e 's/&/\&amp;/g' -e 's/</\&lt;/g' -e 's/>/\&gt;/g')"
    QA_BUILD_PLIST="$QA_BUILD_PLIST
	<key>HubReleasesURL</key>
	<string>$RELEASES_URL_XML</string>"
  fi
fi

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
	$QA_BUILD_PLIST
</dict>
</plist>
PLIST
plutil -lint "$APP_DIR/Contents/Info.plist" >/dev/null

echo "==> Signing"
if [ "$IDENTITY" = "-" ]; then
  SIGN_OPTIONS=(--sign -)
else
  SIGN_OPTIONS=(--options runtime --sign "$IDENTITY")
fi

# Inside out: the helpers, then the bundle, which seals them.
for executable in "$HELPERS"/*; do
  if [ -f "$executable" ]; then
    codesign --force "${SIGN_OPTIONS[@]}" "$executable"
  fi
done
codesign --force "${SIGN_OPTIONS[@]}" "$APP_DIR"
codesign --verify --strict --deep "$APP_DIR"
if [ -n "$TEAM_ID" ]; then
  for executable in "$APP_DIR/Contents/MacOS/$APP_NAME" "$HELPERS"/*; do
    [ -f "$executable" ] || continue
    team="$(codesign --display --verbose=2 "$executable" 2>&1 | sed -n 's/^TeamIdentifier=//p')"
    if [ "$team" != "$TEAM_ID" ]; then
      echo "$executable is signed by team \"$team\", not $TEAM_ID." >&2
      exit 1
    fi
  done
fi

echo "==> Built $APP_DIR $VERSION"
