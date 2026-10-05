#!/bin/sh
# Posts a Linear project update for an Android release: its version, a link to
# the APK and the release's changelog.
#
#   linear-update.sh <tag>
#
# Needs GH_TOKEN and GITHUB_REPOSITORY to read the release, LINEAR_API_KEY (a
# key kept for releases alone) and LINEAR_PROJECT, the project's name.

# shellcheck disable=SC2016 # the $s in the queries are GraphQL variables
set -eu

if [ $# -ne 1 ]; then
	echo "usage: $0 <tag>" >&2
	exit 2
fi
tag=$1

fail() {
	echo "::error::$*" >&2
	exit 1
}

case $tag in
android-v*) ;;
*) fail "$tag is not an Android release tag (android-v…)." ;;
esac

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

release=$(gh release view "$tag" --repo "$GITHUB_REPOSITORY" --json name,url,body,assets)
name=$(printf '%s' "$release" | jq -r '.name')
url=$(printf '%s' "$release" | jq -r '.url')
notes=$(printf '%s' "$release" | jq -r '.body')
apk_name=$(printf '%s' "$release" | jq -r '[.assets[] | select(.name | endswith(".apk"))][0].name // empty')
apk_url=$(printf '%s' "$release" | jq -r '[.assets[] | select(.name | endswith(".apk"))][0].url // empty')
[ -n "$apk_url" ] || fail "Release $tag has no APK."

projects=$(linear 'query($name: String!) { projects(filter: { name: { eq: $name } }) { nodes { id } } }' \
	"$(jq -n --arg name "$LINEAR_PROJECT" '{name: $name}')")
count=$(printf '%s' "$projects" | jq '.projects.nodes | length')
[ "$count" -eq 1 ] || fail "Expected one Linear project named $LINEAR_PROJECT, found $count."
project_id=$(printf '%s' "$projects" | jq -r '.projects.nodes[0].id')

body="**[$name]($url)** is out: install [$apk_name]($apk_url) on the phone.

$notes"

update=$(linear 'mutation($input: ProjectUpdateCreateInput!) { projectUpdateCreate(input: $input) { success projectUpdate { url } } }' \
	"$(jq -n --arg projectId "$project_id" --arg body "$body" '{input: {projectId: $projectId, body: $body, health: "onTrack"}}')")
[ "$(printf '%s' "$update" | jq -r '.projectUpdateCreate.success')" = true ] || fail "Linear didn't create the update: $update"
echo "Posted $(printf '%s' "$update" | jq -r '.projectUpdateCreate.projectUpdate.url')"
