#!/bin/bash
# Opens a build of the app on one page, captures only its window, and quits it.
# Usage: Scripts/screenshot-page.sh <page> <output.png> [more app arguments, e.g. --job <id>]
#
# It runs make-app.sh's build/JobSearchHub.app, or the bundle SCREENSHOT_APP
# names, as a process of its own, and quits only that one: the installed app
# in ~/Applications stays open, with its sessions.
set -euo pipefail

cd "$(dirname "$0")/.."
PAGE="$1"
OUTPUT="$2"
APP="${SCREENSHOT_APP:-build/JobSearchHub.app}"
HELPER="build/window-id"

[ "$HELPER" -nt Scripts/window-id.swift ] || swiftc -O Scripts/window-id.swift -o "$HELPER"
EXECUTABLE="$(cd "$APP" && pwd)/Contents/MacOS/JobSearchHub"
pkill -f "^$EXECUTABLE" || true
open -n "$APP" --args --page "$PAGE" "${@:3}"
sleep "${SCREENSHOT_WAIT_SECONDS:-3}"

PID="$(pgrep -n -f "^$EXECUTABLE" || true)"
if [ -z "$PID" ]; then
  echo "$APP didn't start" >&2
  exit 1
fi
WINDOW_ID="$("$HELPER" "Job Search Hub" "$PID")"
if [ -z "$WINDOW_ID" ]; then
  echo "No Job Search Hub window found" >&2
  kill "$PID" || true
  exit 1
fi
screencapture -x -o -l"$WINDOW_ID" "$OUTPUT"
kill "$PID"
echo "$OUTPUT"
