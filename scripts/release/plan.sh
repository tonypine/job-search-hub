#!/bin/sh
# Decides whether HEAD gets an Android release, and its version. Run from the
# repository root with the whole history and the tags fetched. Prints
# key=value lines for $GITHUB_OUTPUT:
#
#   changed       true when android/ changed since the previous release
#   previous_tag  the newest android-v* tag, or empty
#   version_code  git rev-list --count HEAD (only when changed)
#   version_name  <versionMajorMinor>.<version_code>
#   tag           android-v<version_name>
#
# The diff runs against the previous release, not the previous commit, so a
# run that failed or that the concurrency group dropped is picked up by the
# next one. Before the first release it runs against HEAD^.

set -eu

gradle_file=android/app/build.gradle.kts
empty_tree=4b825dc642cb6eb9a060e54bf8d69288fbee4904

fail() {
	echo "::error::$*" >&2
	exit 1
}

# The release with the highest version code, which needn't be the newest.
previous_tag=
previous_code=0
for tag in $(git tag --list 'android-v*'); do
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

echo "previous_tag=$previous_tag"

if git diff --quiet "$base" HEAD -- android/; then
	echo "Nothing under android/ changed since ${previous_tag:-the previous commit}; no release." >&2
	echo "changed=false"
	exit 0
fi
echo "changed=true"

major_minor=$(sed -n 's/^val versionMajorMinor = "\([0-9][0-9]*\.[0-9][0-9]*\)"$/\1/p' "$gradle_file")
[ -n "$major_minor" ] || fail "No 'val versionMajorMinor = \"<major>.<minor>\"' line in $gradle_file."

version_code=$(git rev-list --count HEAD)
if [ "$version_code" -le "$previous_code" ]; then
	fail "versionCode $version_code is not above $previous_tag's $previous_code; a phone would refuse it as a downgrade."
fi

version_name=$major_minor.$version_code
echo "version_code=$version_code"
echo "version_name=$version_name"
echo "tag=android-v$version_name"
