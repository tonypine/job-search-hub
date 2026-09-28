#!/bin/bash
# Opens the built app on one page, captures only its window, and quits it.
# Usage: Scripts/screenshot-page.sh <page> <output.png> [more app arguments, e.g. --job <id>]
set -euo pipefail

cd "$(dirname "$0")/.."
PAGE="$1"
OUTPUT="$2"
HELPER="build/window-id"

[ -x "$HELPER" ] || swiftc -O Scripts/window-id.swift -o "$HELPER"
pkill -x JobSearchHub || true
open -n build/JobSearchHub.app --args --page "$PAGE" "${@:3}"
sleep "${SCREENSHOT_WAIT_SECONDS:-3}"

WINDOW_ID="$("$HELPER" "Job Search Hub")"
if [ -z "$WINDOW_ID" ]; then
  echo "No Job Search Hub window found" >&2
  pkill -x JobSearchHub || true
  exit 1
fi
screencapture -x -o -l"$WINDOW_ID" "$OUTPUT"
pkill -x JobSearchHub
echo "$OUTPUT"
