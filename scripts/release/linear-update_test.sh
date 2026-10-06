#!/bin/sh
# Tests linear-update.sh with stand-ins for gh and curl: gh prints a release
# from a file, and curl records each Linear request and answers it. These pin
# the update's body for each platform and both, and that a run makes one post.

# shellcheck source=scripts/release/testlib.sh
. "$(dirname "$0")/testlib.sh"

fake=$(mktemp -d "${TMPDIR:-/tmp}/linear-update-test.XXXXXX")
trap 'rm -rf "$fake"' EXIT
mkdir "$fake/bin" "$fake/releases"

# gh release view <tag> …: the release saved for the tag.
cat >"$fake/bin/gh" <<'EOF'
#!/bin/sh
cat "$FAKE/releases/$3.json"
EOF

# curl … --data @-: saves the request, one JSON a line, and answers the
# project lookup with $FAKE/initiatives and any mutation with a URL.
cat >"$fake/bin/curl" <<'EOF'
#!/bin/sh
request=$(jq -c .)
printf '%s\n' "$request" >>"$FAKE/requests"
case $(printf '%s' "$request" | jq -r .query) in
*projects\(*)
	jq -n --argjson initiatives "$(cat "$FAKE/initiatives")" \
		'{data: {projects: {nodes: [{id: "project-1", initiatives: {nodes: $initiatives}}]}}}'
	;;
*initiativeUpdateCreate*)
	echo '{"data": {"initiativeUpdateCreate": {"success": true, "initiativeUpdate": {"url": "https://linear.app/initiative-update"}}}}'
	;;
*projectUpdateCreate*)
	echo '{"data": {"projectUpdateCreate": {"success": true, "projectUpdate": {"url": "https://linear.app/project-update"}}}}'
	;;
esac
EOF
chmod +x "$fake/bin/gh" "$fake/bin/curl"

repo=https://github.com/owner/job-search-hub
jq -n --arg url "$repo/releases/tag/mac-v0.1.252" '{
	name: "Job Search Hub 0.1.252 for Mac",
	url: $url,
	body: "## New\n\n- Mac: Suggest a second route (#67)\n- Server, Phone: Send a phone the new jobs (#68)\n\n## Fixed\n\n- Mac: Keep the inspector'"'"'s width (#66)\n",
	assets: [{name: "Job-Search-Hub-0.1.252.zip", url: "\($url)/Job-Search-Hub-0.1.252.zip"}]
}' >"$fake/releases/mac-v0.1.252.json"
jq -n --arg url "$repo/releases/tag/android-v0.1.252" --arg download "$repo/releases/download/android-v0.1.252" '{
	name: "Android 0.1.252",
	url: $url,
	body: "## New\n\n- Server, Phone: Send a phone the new jobs (#68)\n\n## Other changes\n\n- Phone: refactor: split the pipeline screen (#65)\n",
	assets: [{name: "job-search-hub-0.1.252.apk", url: "\($download)/job-search-hub-0.1.252.apk"}]
}' >"$fake/releases/android-v0.1.252.json"
jq -n '{name: "Android 0.1.300", url: "u", body: "", assets: []}' >"$fake/releases/android-v0.1.300.json"

# post <initiatives-json> <tag>...: runs the script and prints its output.
post() {
	printf '%s' "$1" >"$fake/initiatives"
	: >"$fake/requests"
	shift
	FAKE=$fake PATH="$fake/bin:$PATH" GH_TOKEN=token GITHUB_REPOSITORY=owner/job-search-hub \
		LINEAR_API_KEY=key LINEAR_PROJECT="Job Search Hub" "$here/linear-update.sh" "$@" 2>&1
}

# The mutations the last run sent, one a line.
mutations() {
	jq -r 'select(.query | startswith("mutation")) | .query | capture("\\{ (?<name>[A-Za-z]+)\\(").name' "$fake/requests"
}

posted_body() {
	jq -r 'select(.query | startswith("mutation")) | .variables.input.body' "$fake/requests"
}

one_initiative='[{"id": "initiative-1", "name": "Job Search"}]'

expect "a Mac release posts on the initiative" "Posted https://linear.app/initiative-update" \
	"$(post "$one_initiative" mac-v0.1.252)"
expect "  once" "initiativeUpdateCreate" "$(mutations)"
expect "  on the project's initiative" "initiative-1" \
	"$(jq -r 'select(.query | startswith("mutation")) | .variables.input.initiativeId' "$fake/requests")"
expect "  with its link and changelog" "**Job Search Hub 0.1.252** is out.

- **Mac**: [Job Search Hub 0.1.252 for Mac]($repo/releases/tag/mac-v0.1.252). The app offers it in Settings › Version.

## New

- Mac: Suggest a second route (#67)
- Server, Phone: Send a phone the new jobs (#68)

## Fixed

- Mac: Keep the inspector's width (#66)" "$(posted_body)"

post "$one_initiative" android-v0.1.252 >/dev/null
expect "an Android release links the APK" "**Job Search Hub 0.1.252** is out.

- **Phone**: [job-search-hub-0.1.252.apk]($repo/releases/download/android-v0.1.252/job-search-hub-0.1.252.apk). The phone offers it on Today.

## New

- Server, Phone: Send a phone the new jobs (#68)

## Other changes

- Phone: refactor: split the pipeline screen (#65)" "$(posted_body)"

expect "a run that released both makes one post" "Posted https://linear.app/initiative-update" \
	"$(post "$one_initiative" android-v0.1.252 mac-v0.1.252)"
expect "  on the initiative" "initiativeUpdateCreate" "$(mutations)"
expect "  with both links, Mac first, and one changelog that lists a shared change once" "**Job Search Hub 0.1.252** is out.

- **Mac**: [Job Search Hub 0.1.252 for Mac]($repo/releases/tag/mac-v0.1.252). The app offers it in Settings › Version.
- **Phone**: [job-search-hub-0.1.252.apk]($repo/releases/download/android-v0.1.252/job-search-hub-0.1.252.apk). The phone offers it on Today.

## New

- Mac: Suggest a second route (#67)
- Server, Phone: Send a phone the new jobs (#68)

## Fixed

- Mac: Keep the inspector's width (#66)

## Other changes

- Phone: refactor: split the pipeline screen (#65)" "$(posted_body)"

expect "a project in no initiative gets a project update, with a warning" "::warning::The Linear project Job Search Hub belongs to no initiative, so mac-v0.1.252 android-v0.1.252 is posted as a project update instead.
Posted https://linear.app/project-update" "$(post '[]' mac-v0.1.252 android-v0.1.252)"
expect "  once, on the project" "projectUpdateCreate project-1" \
	"$(printf '%s %s' "$(mutations)" "$(jq -r 'select(.query | startswith("mutation")) | .variables.input.projectId' "$fake/requests")")"

expect "a project in two initiatives gets a project update too" "projectUpdateCreate" \
	"$(post '[{"id": "a", "name": "A"}, {"id": "b", "name": "B"}]' mac-v0.1.252 >/dev/null && mutations)"

expect "refuses two versions" "::error::mac-v0.1.252 android-v0.1.300 are different versions." \
	"$(post "$one_initiative" mac-v0.1.252 android-v0.1.300 || true)"
expect "refuses two tags of one platform" "::error::Two Mac release tags: mac-v0.1.252 and mac-v0.1.253." \
	"$(post "$one_initiative" mac-v0.1.252 mac-v0.1.253 || true)"
expect "refuses a tag that isn't a release's" "::error::v0.1.252 is not a release tag (mac-v… or android-v…)." \
	"$(post "$one_initiative" v0.1.252 || true)"
expect "refuses an Android release without an APK" "::error::Release android-v0.1.300 has no APK." \
	"$(post "$one_initiative" android-v0.1.300 || true)"
expect "  and posts nothing" "" "$(cat "$fake/requests")"
expect "refuses three tags" "usage: $here/linear-update.sh <tag> [<tag>]" \
	"$(post "$one_initiative" mac-v0.1.1 android-v0.1.1 mac-v0.1.2 || true)"

finish
