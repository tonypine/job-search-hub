#!/bin/sh
# Decides which apps HEAD releases, and their one version. Run from the
# repository root with the whole history and the tags fetched. Prints
# key=value lines for $GITHUB_OUTPUT, for each platform:
#
#   <p>_previous_tag  the <p>-v* tag with the highest version code, or empty
#   <p>_changed       true when its paths changed since that release
#   <p>_tag           <p>-v<version_name> (only when changed)
#
# where <p> is mac (server/ and macos/, since the server ships inside the
# Mac app) or android (android/), and, when either changed:
#
#   version_code  git rev-list --count HEAD, the same for both
#   version_name  <versionMajorMinor>.<version_code>
#
# Each diff runs against that platform's previous release, not the previous
# commit, so a run that failed or that the concurrency group dropped is picked
# up by the next one. Before a platform's first release it runs against HEAD^.

set -eu

gradle_file=android/app/build.gradle.kts
empty_tree=4b825dc642cb6eb9a060e54bf8d69288fbee4904

fail() {
	echo "::error::$*" >&2
	exit 1
}

version_code=$(git rev-list --count HEAD)
released=

# plan <platform> <paths>...: prints the platform's previous tag and whether
# it changed, and adds it to $released when it did.
plan() {
	platform=$1
	shift

	# The release with the highest version code, which needn't be the newest.
	previous_tag=
	previous_code=0
	for tag in $(git tag --list "$platform-v*"); do
		code=${tag##*.}
		case $code in
		'' | *[!0-9]*) continue ;;
		esac
		if [ "$code" -gt "$previous_code" ]; then
			previous_code=$code
			previous_tag=$tag
		fi
	done

	if [ -n "$previous_tag" ]; then
		base=$previous_tag
	elif git rev-parse --verify --quiet 'HEAD^' >/dev/null; then
		base=HEAD^
	else
		base=$empty_tree
	fi

	echo "${platform}_previous_tag=$previous_tag"

	if git diff --quiet "$base" HEAD -- "$@"; then
		echo "Nothing under $* changed since ${previous_tag:-the previous commit}; no $platform release." >&2
		echo "${platform}_changed=false"
		return
	fi
	echo "${platform}_changed=true"

	if [ "$version_code" -le "$previous_code" ]; then
		fail "Version code $version_code is not above $previous_tag's $previous_code; the app would refuse it as a downgrade."
	fi
	released="$released $platform"
}

plan mac server/ macos/
plan android android/

[ -n "$released" ] || exit 0

major_minor=$(sed -n 's/^val versionMajorMinor = "\([0-9][0-9]*\.[0-9][0-9]*\)"$/\1/p' "$gradle_file")
[ -n "$major_minor" ] || fail "No 'val versionMajorMinor = \"<major>.<minor>\"' line in $gradle_file."

version_name=$major_minor.$version_code
echo "version_code=$version_code"
echo "version_name=$version_name"
for platform in $released; do
	echo "${platform}_tag=$platform-v$version_name"
done
