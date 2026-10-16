#!/bin/sh
# Tests changelog.sh against a throwaway repository. The apps parse its
# output, so these pin the format: the headings, the tags and the pull
# requests.

# shellcheck source=scripts/release/testlib.sh
. "$(dirname "$0")/testlib.sh"

# commit_both <path> <path> <subject>: one commit that changes two files.
commit_both() {
	mkdir -p "$(dirname "$1")" "$(dirname "$2")"
	echo "$3" >>"$1"
	echo "$3" >>"$2"
	git add "$1" "$2"
	git commit --quiet -m "$3"
}

new_repo
commit android/app/a.kt "feat: pair a phone from a link (#1)"
commit server/main.go "feat: list the companies (#2)"
git tag android-v0.1.2
git tag mac-v0.1.2
commit android/app/a.kt "feat(android): swipe a card to follow up (#3)"
commit server/main.go "fix: retry board polls that time out (#4)"
commit android/core/b.kt "fix!: keep the scroll position when folding (#5)"
commit android/app/a.kt "refactor: split the pipeline screen (#6)"
commit android/README.md "Update the README"
commit macos/c.swift "feat: suggest a second route (#7)"
commit_both macos/c.swift server/api.go "fix: keep the inspector's width (#8)"
commit_both server/api.go android/app/a.kt "feat: send a phone the new jobs (#9)"
commit docs/a.md "docs: a note (#10)"
commit .github/workflows/ci.yml "chore: pin staticcheck in CI (#11)"
commit macos/c.swift 'Revert "feat: suggest a second route (#7)" (#12)'
commit_both server/main.go .github/workflows/ci.yml "chore: build the server on Go 1.27 (#13)"

expect "the Mac release lists server/ and macos/ commits, tagged by part" "## New

- Server, Phone: Send a phone the new jobs (#9)
- Mac: Suggest a second route (#7)

## Fixed

- Mac: Took out: suggest a second route (#7) (#12)
- Mac, Server: Keep the inspector's width (#8)
- Server: Retry board polls that time out (#4)

## Other changes

- Server: chore: build the server on Go 1.27 (#13)" "$("$here/changelog.sh" mac-v0.1.2 HEAD server/ macos/)"

expect "the Android release lists android/ commits, tagged by part" "## New

- Server, Phone: Send a phone the new jobs (#9)
- Phone: Swipe a card to follow up (#3)

## Fixed

- Phone: Keep the scroll position when folding (#5)

## Other changes

- Phone: Update the README
- Phone: refactor: split the pipeline screen (#6)" "$("$here/changelog.sh" android-v0.1.2 HEAD android/)"

expect "a platform's first release lists every commit under its paths" "## New

- Server: List the companies (#2)" "$("$here/changelog.sh" "" mac-v0.1.2 server/ macos/)"

expect "leaves out empty sections" "## New

- Phone: Pair a phone from a link (#1)" "$("$here/changelog.sh" "" android-v0.1.2 android/)"

git tag android-v0.1.20
commit server/main.go "feat: another server change (#14)"
expect "says so when nothing under the paths changed" "No changes under android/ since android-v0.1.20." \
	"$("$here/changelog.sh" android-v0.1.20 HEAD android/)"

expect "refuses a call without paths" "usage: $here/changelog.sh <previous-tag> <head> <path>..." \
	"$("$here/changelog.sh" android-v0.1.20 HEAD 2>&1 || true)"

# The Mac's awk, under a UTF-8 locale, stopped on a feature whose first
# letter is more than one byte: "illegal byte sequence".
git tag mac-v0.1.20
commit macos/c.swift "feat: ⌘K opens the command palette (#15)"
commit server/main.go "fix: show 2×2 → 3×3 grids in the façade (#16)"
expect "keeps non-ASCII subjects as written, under a UTF-8 locale" "## New

- Mac: ⌘K opens the command palette (#15)

## Fixed

- Server: Show 2×2 → 3×3 grids in the façade (#16)" "$(LC_ALL=C.UTF-8 "$here/changelog.sh" mac-v0.1.20 HEAD server/ macos/ 2>&1)"

finish
