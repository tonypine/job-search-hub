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
#   Contents/Library/LaunchAgents/com.tonypine.jobsearchhub.server.plist
#                                                  the server's agent, which the app registers
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
AGENT_LABEL="com.tonypine.jobsearchhub.server"

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

  # The app registers this agent with SMAppService, which reads it from the
  # bundle, so launchd runs whichever server the bundle at that path holds.
  # BundleProgram is relative to the bundle, and launchd can't expand a home
  # folder here, so the server opens the log HUB_LOG_FILE names itself. It
  # restarts after a crash, not after a clean exit, as when an update stops
  # it to swap the bundle. On SIGTERM the server waits up to 30 seconds for
  # the work still running, 5 for open requests, then up to 30 for its
  # Postgres to stop: ExitTimeOut leaves room for all of it.
  mkdir -p "$APP_DIR/Contents/Library/LaunchAgents"
  AGENT_PLIST="$APP_DIR/Contents/Library/LaunchAgents/$AGENT_LABEL.plist"
  cat > "$AGENT_PLIST" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>$AGENT_LABEL</string>
	<key>BundleProgram</key>
	<string>Contents/Helpers/bin/hub-server</string>
	<key>EnvironmentVariables</key>
	<dict>
		<key>HUB_LOG_FILE</key>
		<string>~/Library/Logs/JobSearchHub/server.log</string>
	</dict>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<dict>
		<key>SuccessfulExit</key>
		<false/>
	</dict>
	<key>ExitTimeOut</key>
	<integer>70</integer>
</dict>
</plist>
PLIST
  plutil -lint "$AGENT_PLIST" >/dev/null
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
# The bundle and every executable in it are signed by the owner's team, pinned
# by its ID, so another identity in the keychain, such as a work certificate,
# is never picked. An Apple-issued identity keeps the app's designated
# requirement stable across builds, so its Keychain item (the owner token)
# survives them. CODESIGN_TEAM_ID names the team, or
# ~/.config/job-search-hub/codesign-team-id holds it; CODESIGN_IDENTITY can
# narrow the choice to one of the team's identities. Without a team, as in CI
# and Symphony's QA VM, or with CODESIGN_IDENTITY=-, the bundle is signed ad
# hoc, without the hardened runtime, and the Keychain asks for access after
# every build.
TEAM_FILE="$HOME/.config/job-search-hub/codesign-team-id"
TEAM_ID="${CODESIGN_TEAM_ID:-}"
if [ -z "$TEAM_ID" ] && [ -f "$TEAM_FILE" ]; then
  TEAM_ID="$(tr -d '[:space:]' < "$TEAM_FILE")"
fi

# team_of SHA1 NAME prints the team of the identity's certificate, its OU.
team_of() {
  security find-certificate -a -c "$2" -Z -p 2>/dev/null |
    awk -v hash="$1" '/^SHA-1 hash:/ { current = $3 } /BEGIN CERTIFICATE/ { printing = (current == hash) } printing { print } /END CERTIFICATE/ { printing = 0 }' |
    openssl x509 -noout -subject 2>/dev/null | sed -n 's/.*OU *= *\([A-Z0-9]*\).*/\1/p'
}

if [ "${CODESIGN_IDENTITY:-}" = "-" ]; then
  TEAM_ID=""
  IDENTITY="-"
elif [ -z "$TEAM_ID" ]; then
  if [ -n "${CODESIGN_IDENTITY:-}" ]; then
    echo "CODESIGN_IDENTITY needs its team pinned too: set CODESIGN_TEAM_ID, or write it to $TEAM_FILE." >&2
    exit 1
  fi
  echo "No signing team pinned, so signing ad hoc: the Keychain will ask for access after every build. Set" >&2
  echo "CODESIGN_TEAM_ID to your Apple Development certificate's team, or write it to $TEAM_FILE." >&2
  IDENTITY="-"
else
  if ! [[ "$TEAM_ID" =~ ^[A-Z0-9]{10}$ ]]; then
    echo "\"$TEAM_ID\" isn't a team ID, ten capital letters and digits, as Keychain Access shows in the certificate's Organizational Unit." >&2
    exit 1
  fi
  IDENTITY=""
  while read -r hash name; do
    case "$name" in *"${CODESIGN_IDENTITY:-}"*) ;; *) continue ;; esac
    if [ "$(team_of "$hash" "$name")" = "$TEAM_ID" ]; then
      IDENTITY="$hash"
      echo "Signing with \"$name\" of team $TEAM_ID"
      break
    fi
  done < <(security find-identity -v -p codesigning | sed -n 's/^ *[0-9]*) \([0-9A-F]\{40\}\) "\(.*\)"$/\1 \2/p')
  if [ -z "$IDENTITY" ]; then
    echo "No signing identity of team $TEAM_ID${CODESIGN_IDENTITY:+ matching \"$CODESIGN_IDENTITY\"} in the keychain, and no other team's will do." >&2
    echo "Create an Apple Development certificate in Xcode > Settings > Accounts > Manage Certificates." >&2
    exit 1
  fi
fi
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
