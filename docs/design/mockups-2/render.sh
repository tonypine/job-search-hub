#!/bin/sh
# Draws the second redesign's mockups with SwiftUI, offscreen, into a folder:
#
#     docs/design/mockups-2/render.sh docs/design/mockups-2
#
# Every company, job and person in them is made up. The views sketch the
# proposal in docs/design/ui-redesign-2.md; they are not app code.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
out=${1:-$here}
build=${TMPDIR:-/tmp}/hub-mockups-2
mkdir -p "$build"
CLANG_MODULE_CACHE_PATH="$build/module-cache" swiftc -O -module-cache-path "$build/module-cache" \
    -o "$build/render" "$here"/Sources/*.swift
shift $(( $# > 0 ? 1 : 0 ))
"$build/render" "$out" "$@"
