#!/bin/sh
# The hub's one version, for every part a build stamps: the Mac app,
# hub-server, hub, hub-update and hub-cvprint. Run inside the repository.
#
#   version.sh               the version
#   version.sh --commit      the commit it is built from, or "unknown"
#   version.sh --go-ldflags  the -ldflags that stamp both into a Go command
#
# A release passes HUB_VERSION_CODE, the commit count on main, as release.yml
# passes -PversionCode to the Android build, and gets
# <versionMajorMinor>.<HUB_VERSION_CODE>, the same number the APK of that
# commit carries. Without it, a local build gets
# <versionMajorMinor>.0-dev.<short commit>.

set -eu

buildinfo=github.com/tonypine/job-search-hub/server/internal/buildinfo

fail() {
	echo "version.sh: $*" >&2
	exit 1
}

root=$(git rev-parse --show-toplevel 2>/dev/null || (cd "$(dirname "$0")/../.." && pwd))
gradle_file=$root/android/app/build.gradle.kts
major_minor=$(sed -n 's/^val versionMajorMinor = "\([0-9][0-9]*\.[0-9][0-9]*\)"$/\1/p' "$gradle_file" 2>/dev/null || true)
[ -n "$major_minor" ] || fail "no 'val versionMajorMinor = \"<major>.<minor>\"' line in $gradle_file"

commit=$(git -C "$root" rev-parse HEAD 2>/dev/null || echo unknown)

version_code=${HUB_VERSION_CODE:-}
case $version_code in
'')
	if [ "$commit" = unknown ]; then
		version=$major_minor.0-dev
	else
		version=$major_minor.0-dev.$(git -C "$root" rev-parse --short HEAD)
	fi
	;;
*[!0-9]*) fail "HUB_VERSION_CODE must be a number, the commit count on main; got '$version_code'" ;;
*) version=$major_minor.$version_code ;;
esac

case ${1:-} in
'') echo "$version" ;;
--commit) echo "$commit" ;;
--go-ldflags) echo "-X $buildinfo.version=$version -X $buildinfo.commit=$commit" ;;
*) fail "unknown option '$1'; use --commit or --go-ldflags" ;;
esac
