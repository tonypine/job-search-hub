#!/bin/sh
# Prints a release's notes in Markdown: the subjects of the commits since the
# previous release that touch android/, newest first, under Features, Fixes
# and Other changes. Features and fixes lose their `type(scope):` prefix;
# other subjects keep theirs, or have none if they aren't conventional.
#
#   changelog.sh <previous-tag> [<head>]
#
# An empty previous tag means the first release, which lists every commit.

set -eu

if [ $# -lt 1 ] || [ $# -gt 2 ]; then
	echo "usage: $0 <previous-tag> [<head>]" >&2
	exit 2
fi
previous_tag=$1
head=${2:-HEAD}

if [ -n "$previous_tag" ]; then
	range=$previous_tag..$head
else
	range=$head
fi

subjects=$(git log --no-merges --format=%s "$range" -- android/)

if [ -z "$subjects" ]; then
	echo "No changes to the app since $previous_tag."
	exit 0
fi

# section <type, or "other"> <heading>: prints nothing when no subject fits.
section() {
	printf '%s\n' "$subjects" | awk -v want="$1" -v heading="$2" '
		{
			type = ""
			rest = $0
			if (match($0, /^[a-z]+(\([^)]*\))?!?: /)) {
				type = substr($0, 1, RLENGTH)
				sub(/[(!:].*/, "", type)
				rest = substr($0, RLENGTH + 1)
			}
			if (want == "other") {
				if (type == "feat" || type == "fix") next
				rest = $0
			} else if (type != want) {
				next
			}
			if (!printed) {
				print "## " heading
				print ""
				printed = 1
			}
			print "- " rest
		}
	'
}

separator=
for spec in "feat Features" "fix Fixes" "other Other changes"; do
	out=$(section "${spec%% *}" "${spec#* }")
	[ -n "$out" ] || continue
	printf '%s%s\n' "$separator" "$out"
	separator='
'
done
