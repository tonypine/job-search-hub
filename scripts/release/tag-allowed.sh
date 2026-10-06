#!/bin/sh
# Whether the workflow's token may tag <commit>, given the tree id of
# .github/workflows at main's tip (empty when main has none). Run inside the
# repository with <commit> fetched.
#
#   tag-allowed.sh <commit> <main's .github/workflows tree>
#
# Exits 0 when it may. Exits 1, and says why on stdout, when <commit>'s
# .github/workflows differs from main's: GitHub takes a new tag on such a
# commit for creating or updating workflows, which needs the workflows
# permission, and GITHUB_TOKEN can't have it, so the release API answers
# "HTTP 403: Resource not accessible by integration". That happens to a
# commit that a later merge changing .github/workflows left behind main's tip.
# The release for main's tip carries its changes instead, since plan.sh diffs
# against each app's previous release.

set -eu

fail() {
	echo "tag-allowed.sh: $*" >&2
	exit 2
}

[ $# -eq 2 ] || fail "usage: tag-allowed.sh <commit> <main's .github/workflows tree>"
commit=$1
main_tree=$2

git rev-parse --verify --quiet "$commit^{commit}" >/dev/null || fail "no commit $commit here"
tree=$(git rev-parse --verify --quiet "$commit:.github/workflows" || true)

[ "$tree" != "$main_tree" ] || exit 0

short=$(git rev-parse --short "$commit")
echo "main's .github/workflows has changed since $short, so GitHub won't let the workflow's token tag $short (that takes the workflows permission, which GITHUB_TOKEN can't have). No release from $short; the next release from main includes its changes."
exit 1
