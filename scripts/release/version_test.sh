#!/bin/sh
# Tests version.sh against a throwaway repository.

# shellcheck source=scripts/release/testlib.sh
. "$(dirname "$0")/testlib.sh"

new_repo
mkdir -p android/app
printf 'plugins {}\n\nval versionMajorMinor = "0.1"\n' >android/app/build.gradle.kts
git add android/app/build.gradle.kts
git commit --quiet -m "feat: the app"
short=$(git rev-parse --short HEAD)
full=$(git rev-parse HEAD)

expect "a local build is a dev version of the commit" "0.1.0-dev.$short" "$("$here/version.sh")"
expect "a release build takes the version code" "0.1.252" "$(HUB_VERSION_CODE=252 "$here/version.sh")"
expect "the commit is the whole hash" "$full" "$("$here/version.sh" --commit)"
expect "the Go flags stamp both" \
	"-X github.com/tonypine/job-search-hub/server/internal/buildinfo.version=0.1.252 -X github.com/tonypine/job-search-hub/server/internal/buildinfo.commit=$full" \
	"$(HUB_VERSION_CODE=252 "$here/version.sh" --go-ldflags)"

mkdir -p macos
expect "any folder of the repository gives the same version" "0.1.7" "$(cd macos && HUB_VERSION_CODE=7 "$here/version.sh")"

sed -i.bak 's/"0.1"/"0.2"/' android/app/build.gradle.kts && rm android/app/build.gradle.kts.bak
expect "reads versionMajorMinor from the app's build file" "0.2.9" "$(HUB_VERSION_CODE=9 "$here/version.sh")"

expect "refuses a version code that isn't a number" \
	"version.sh: HUB_VERSION_CODE must be a number, the commit count on main; got '0.1.9'" \
	"$(HUB_VERSION_CODE=0.1.9 "$here/version.sh" 2>&1 >/dev/null || true)"
if HUB_VERSION_CODE=x "$here/version.sh" >/dev/null 2>&1; then
	expect "the refusal exits non-zero" "1" "0"
fi

finish
