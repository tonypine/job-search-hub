# shellcheck shell=sh
# Shared by the *_test.sh scripts: a throwaway repository and assertions.

set -eu

# shellcheck disable=SC2034 # used by the tests that source this
here=$(cd "$(dirname "$0")" && pwd)
failures=0

# new_repo: a fresh repository in a temp directory, made the working directory.
new_repo() {
	repo=$(mktemp -d "${TMPDIR:-/tmp}/release-test.XXXXXX")
	trap 'rm -rf "$repo"' EXIT
	cd "$repo"
	git init --quiet --initial-branch=main
	git config user.name Test
	git config user.email test@example.com
	git config commit.gpgsign false
	git config tag.gpgsign false
}

# commit <path> <subject>: changes one file and commits it.
commit() {
	mkdir -p "$(dirname "$1")"
	echo "$2" >>"$1"
	git add "$1"
	git commit --quiet -m "$2"
}

# expect <name> <expected> <actual>
expect() {
	if [ "$2" = "$3" ]; then
		echo "ok   $1"
	else
		echo "FAIL $1"
		echo "  expected:"
		printf '%s\n' "$2" | sed 's/^/    /'
		echo "  got:"
		printf '%s\n' "$3" | sed 's/^/    /'
		failures=$((failures + 1))
	fi
}

finish() {
	[ "$failures" -eq 0 ] || {
		echo "$failures failed"
		exit 1
	}
}
