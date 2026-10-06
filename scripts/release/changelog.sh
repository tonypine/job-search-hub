#!/bin/sh
# Prints a release's notes in Markdown: the subjects of the commits since the
# previous release that touch the platform's paths, newest first, under New,
# Fixed and Other changes. The apps read this format to show what's new, so
# changelog_test.sh pins it:
#
#   ## New
#
#   - Mac: Suggest a second route for unanswered applications (#67)
#
#   ## Fixed
#
#   - Mac, Server: Retry board polls that time out (#71)
#   - Phone: Took out: swipe a card to follow up (#64) (#70)
#
#   ## Other changes
#
#   - Server: chore: pin staticcheck in CI (#72)
#
# Each line starts with the parts of the hub its commit touched, whichever
# release lists it: Mac for macos/, Server for server/, Phone for android/.
# Features (feat) and fixes (fix) lose their `type(scope):` prefix and start
# with a capital; a revert reads "Took out: …" under Fixed; other subjects keep
# theirs, or have none if they aren't conventional. A squash merge's subject
# ends with its pull request, "(#71)", which stays.
#
#   changelog.sh <previous-tag> <head> <path>...
#
# The Mac release passes server/ and macos/, the Android one android/. An empty
# previous tag means the platform's first release, which lists every commit.

set -eu

if [ $# -lt 3 ]; then
	echo "usage: $0 <previous-tag> <head> <path>..." >&2
	exit 2
fi
previous_tag=$1
head=$2
shift 2

if [ -n "$previous_tag" ]; then
	range=$previous_tag..$head
else
	range=$head
fi

# Each commit is a \037-marked subject line, then every file it touched, not
# only those under the paths, so a commit's tags name all its parts.
log=$(git -c core.quotePath=false log --no-merges --full-diff --name-only --format='%x1f%s' "$range" -- "$@")

if [ -z "$log" ]; then
	echo "No changes under $* since $previous_tag."
	exit 0
fi

printf '%s\n' "$log" | awk '
	function add(section, text) {
		line = "- " (tags != "" ? tags ": " : "") text "\n"
		if (section == "new") new = new line
		else if (section == "fixed") fixed = fixed line
		else other = other line
	}
	function capitalize(s) {
		return toupper(substr(s, 1, 1)) substr(s, 2)
	}
	# Sets type and rest from a conventional subject; type is empty otherwise.
	function split_type(s) {
		type = ""
		rest = s
		if (match(s, /^[a-z]+(\([^)]*\))?!?: /)) {
			type = substr(s, 1, RLENGTH)
			sub(/[(!:].*/, "", type)
			rest = substr(s, RLENGTH + 1)
		}
	}
	function flush() {
		if (!seen) return
		tags = ""
		if (mac) tags = "Mac"
		if (server) tags = tags (tags != "" ? ", " : "") "Server"
		if (phone) tags = tags (tags != "" ? ", " : "") "Phone"

		# GitHub reverts read: Revert "<subject>" (#<pull request>).
		body = subject
		pr = ""
		if (match(body, / \(#[0-9]+\)$/)) {
			pr = substr(body, RSTART)
			body = substr(body, 1, RSTART - 1)
		}
		if (body ~ /^Revert ".*"$/) {
			split_type(substr(body, 9, length(body) - 9))
			add("fixed", "Took out: " rest pr)
			return
		}

		split_type(subject)
		if (type == "feat") add("new", capitalize(rest))
		else if (type == "fix") add("fixed", capitalize(rest))
		else add("other", subject)
	}
	substr($0, 1, 1) == "\037" {
		flush()
		seen = 1
		subject = substr($0, 2)
		mac = server = phone = 0
		next
	}
	/^macos\// { mac = 1 }
	/^server\// { server = 1 }
	/^android\// { phone = 1 }
	END {
		flush()
		separator = ""
		if (new != "") { printf "%s## New\n\n%s", separator, new; separator = "\n" }
		if (fixed != "") { printf "%s## Fixed\n\n%s", separator, fixed; separator = "\n" }
		if (other != "") { printf "%s## Other changes\n\n%s", separator, other }
	}
'
