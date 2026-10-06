#!/bin/sh
# Tests tag-allowed.sh against a throwaway repository.

# shellcheck source=scripts/release/testlib.sh
. "$(dirname "$0")/testlib.sh"

# check <commit> <main tree>: the script's exit status and output.
check() {
	status=0
	output=$("$here/tag-allowed.sh" "$1" "$2" 2>&1) || status=$?
	printf '%s %s' "$status" "$output"
}

new_repo
commit android/app/a.kt "feat: the app"
first=$(git rev-parse HEAD)

expect "neither has workflows: allowed" "0 " "$(check "$first" "")"

commit .github/workflows/ci.yml "ci: a workflow"
workflows=$(git rev-parse HEAD:.github/workflows)
expect "main's tip itself: allowed" "0 " "$(check HEAD "$workflows")"

commit android/app/a.kt "feat: an app change"
app_change=$(git rev-parse HEAD)
short=$(git rev-parse --short HEAD)
commit server/main.go "feat: a later server change"
expect "a later merge left workflows alone: allowed" "0 " "$(check "$app_change" "$(git rev-parse HEAD:.github/workflows)")"

commit .github/workflows/ci.yml "ci: change the workflow"
expect "a later merge changed workflows: refused, and says why" \
	"1 main's .github/workflows has changed since $short, so GitHub won't let the workflow's token tag $short (that takes the workflows permission, which GITHUB_TOKEN can't have). No release from $short; the next release from main includes its changes." \
	"$(check "$app_change" "$(git rev-parse HEAD:.github/workflows)")"

expect "main gained workflows the commit lacks: refused" "1" "$(check "$first" "$workflows" | cut -d' ' -f1)"
expect "main has none, the commit has some: refused" "1" "$(check "$app_change" "" | cut -d' ' -f1)"

expect "an unknown commit is an error" \
	"2 tag-allowed.sh: no commit 0123456789abcdef0123456789abcdef01234567 here" \
	"$(check 0123456789abcdef0123456789abcdef01234567 "")"

finish
