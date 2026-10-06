#!/bin/sh
# Tests changelog.sh against a throwaway repository.

# shellcheck source=scripts/release/testlib.sh
. "$(dirname "$0")/testlib.sh"

new_repo
commit android/app/a.kt "feat: pair a phone from a link (#1)"
commit server/main.go "feat: a server change (#2)"
git tag android-v0.1.2
commit android/app/a.kt "feat(android): swipe a card to follow up (#3)"
commit server/main.go "fix: a server fix (#4)"
commit android/core/b.kt "fix!: keep the scroll position when folding (#5)"
commit android/app/a.kt "refactor: split the pipeline screen (#6)"
commit android/README.md "Update the README"
commit macos/c.swift "feat: a Mac feature (#7)"

expect "groups the commits since the tag that touch android/" "## Features

- swipe a card to follow up (#3)

## Fixes

- keep the scroll position when folding (#5)

## Other changes

- Update the README
- refactor: split the pipeline screen (#6)" "$("$here/changelog.sh" android-v0.1.2)"

expect "the first release lists every app commit" "## Features

- swipe a card to follow up (#3)
- pair a phone from a link (#1)

## Fixes

- keep the scroll position when folding (#5)

## Other changes

- Update the README
- refactor: split the pipeline screen (#6)" "$("$here/changelog.sh" "")"

expect "leaves out empty sections" "## Features

- pair a phone from a link (#1)" "$("$here/changelog.sh" "" android-v0.1.2)"

git tag android-v0.1.9
commit server/main.go "feat: another server change (#8)"
expect "says so when the app didn't change" "No changes to the app since android-v0.1.9." \
	"$("$here/changelog.sh" android-v0.1.9)"

finish
