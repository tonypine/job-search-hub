#!/bin/sh
# Announces a release run in Linear: one update for every platform the run
# released, with the version, a link per platform and the changelog. It goes
# on the initiative the project belongs to; a project in no initiative gets a
# project update instead, with a warning, so the release is announced either
# way. See docs/design/updates.md › The Linear initiative update.
#
#   linear-update.sh <tag> [<tag>]
#
# The tags are a Mac release (mac-v…), an Android one (android-v…), or one of
# each with the same version. Needs GH_TOKEN and GITHUB_REPOSITORY to read the
# releases, LINEAR_API_KEY (a key kept for releases alone) and LINEAR_PROJECT,
# the project's name.

# shellcheck disable=SC2016 # the $s in the queries are GraphQL variables
set -eu

if [ $# -lt 1 ] || [ $# -gt 2 ]; then
	echo "usage: $0 <tag> [<tag>]" >&2
	exit 2
fi

fail() {
	echo "::error::$*" >&2
	exit 1
}

mac_tag=
android_tag=
version=
for tag in "$@"; do
	case $tag in
	mac-v?*)
		[ -z "$mac_tag" ] || fail "Two Mac release tags: $mac_tag and $tag."
		mac_tag=$tag
		;;
	android-v?*)
		[ -z "$android_tag" ] || fail "Two Android release tags: $android_tag and $tag."
		android_tag=$tag
		;;
	*) fail "$tag is not a release tag (mac-v… or android-v…)." ;;
	esac
	[ -z "$version" ] || [ "$version" = "${tag#*-v}" ] || fail "$* are different versions."
	version=${tag#*-v}
done

# linear <query> <variables-json>: prints the response's data, or fails.
linear() {
	response=$(jq -n --arg query "$1" --argjson variables "$2" '{query: $query, variables: $variables}' |
		curl -sS --fail-with-body https://api.linear.app/graphql \
			-H "Authorization: $LINEAR_API_KEY" \
			-H 'Content-Type: application/json' \
			--data @-) || fail "Linear refused the request: $response"
	if printf '%s' "$response" | jq -e '.errors' >/dev/null; then
		fail "Linear answered with errors: $(printf '%s' "$response" | jq -c '.errors')"
	fi
	printf '%s' "$response" | jq '.data'
}

release() {
	gh release view "$1" --repo "$GITHUB_REPOSITORY" --json name,url,body,assets
}

links=
notes=
if [ -n "$mac_tag" ]; then
	mac=$(release "$mac_tag")
	links="- **Mac**: [$(printf '%s' "$mac" | jq -r '.name')]($(printf '%s' "$mac" | jq -r '.url')). The app offers it in Settings › Version."
	notes=$(printf '%s' "$mac" | jq -r '.body')
fi
if [ -n "$android_tag" ]; then
	android=$(release "$android_tag")
	apk_name=$(printf '%s' "$android" | jq -r '[.assets[] | select(.name | endswith(".apk"))][0].name // empty')
	apk_url=$(printf '%s' "$android" | jq -r '[.assets[] | select(.name | endswith(".apk"))][0].url // empty')
	[ -n "$apk_url" ] || fail "Release $android_tag has no APK."
	links="${links:+$links
}- **Phone**: [$apk_name]($apk_url). The phone offers it on Today."
	notes="${notes:+$notes
}$(printf '%s' "$android" | jq -r '.body')"
fi

# One changelog from both releases' notes: their sections in changelog.sh's
# order, each line once, since a commit that touched server/ and android/ is
# in both.
changes=$(printf '%s\n' "$notes" | awk '
	{ sub(/\r$/, "") }
	/^## / {
		section = substr($0, 4)
		if (!(section in lines)) { lines[section] = ""; order[++count] = section }
		next
	}
	section != "" && /^- / && !seen[section, $0]++ { lines[section] = lines[section] $0 "\n" }
	END {
		known["New"] = 1; known["Fixed"] = 1; known["Other changes"] = 1
		n = 0
		list[++n] = "New"; list[++n] = "Fixed"; list[++n] = "Other changes"
		for (i = 1; i <= count; i++) if (!(order[i] in known)) list[++n] = order[i]
		separator = ""
		for (i = 1; i <= n; i++) {
			if (lines[list[i]] == "") continue
			printf "%s## %s\n\n%s", separator, list[i], lines[list[i]]
			separator = "\n"
		}
	}
')

body="**Job Search Hub $version** is out.

$links${changes:+

$changes}"

projects=$(linear 'query($name: String!) { projects(filter: { name: { eq: $name } }) { nodes { id initiatives { nodes { id name } } } } }' \
	"$(jq -n --arg name "$LINEAR_PROJECT" '{name: $name}')")
count=$(printf '%s' "$projects" | jq '.projects.nodes | length')
[ "$count" -eq 1 ] || fail "Expected one Linear project named $LINEAR_PROJECT, found $count."
project_id=$(printf '%s' "$projects" | jq -r '.projects.nodes[0].id')
initiatives=$(printf '%s' "$projects" | jq '.projects.nodes[0].initiatives.nodes')

initiative_count=$(printf '%s' "$initiatives" | jq 'length')
case $initiative_count in
1)
	initiative_id=$(printf '%s' "$initiatives" | jq -r '.[0].id')
	update=$(linear 'mutation($input: InitiativeUpdateCreateInput!) { initiativeUpdateCreate(input: $input) { success initiativeUpdate { url } } }' \
		"$(jq -n --arg initiativeId "$initiative_id" --arg body "$body" '{input: {initiativeId: $initiativeId, body: $body, health: "onTrack"}}')")
	update=$(printf '%s' "$update" | jq '.initiativeUpdateCreate | {success, url: .initiativeUpdate.url}')
	;;
*)
	if [ "$initiative_count" -eq 0 ]; then
		echo "::warning::The Linear project $LINEAR_PROJECT belongs to no initiative, so $* is posted as a project update instead."
	else
		echo "::warning::The Linear project $LINEAR_PROJECT belongs to $initiative_count initiatives, not one, so $* is posted as a project update instead."
	fi
	update=$(linear 'mutation($input: ProjectUpdateCreateInput!) { projectUpdateCreate(input: $input) { success projectUpdate { url } } }' \
		"$(jq -n --arg projectId "$project_id" --arg body "$body" '{input: {projectId: $projectId, body: $body, health: "onTrack"}}')")
	update=$(printf '%s' "$update" | jq '.projectUpdateCreate | {success, url: .projectUpdate.url}')
	;;
esac
[ "$(printf '%s' "$update" | jq -r '.success')" = true ] || fail "Linear didn't create the update: $update"
echo "Posted $(printf '%s' "$update" | jq -r '.url')"
