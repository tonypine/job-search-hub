#!/bin/sh
# Tests plan.sh against a throwaway repository.

# shellcheck source=scripts/release/testlib.sh
. "$(dirname "$0")/testlib.sh"

new_repo
mkdir -p android/app
printf 'plugins {}\n\nval versionMajorMinor = "0.1"\n' >android/app/build.gradle.kts
git add android/app/build.gradle.kts
git commit --quiet -m "feat: the app"

expect "the first commit releases" "previous_tag=
changed=true
version_code=1
version_name=0.1.1
tag=android-v0.1.1" "$("$here/plan.sh" 2>/dev/null)"

commit server/main.go "feat: a server change"
expect "a server change without a release yet doesn't release" "previous_tag=
changed=false" "$("$here/plan.sh" 2>/dev/null)"

commit android/app/a.kt "feat: an app change"
expect "an app change releases at the commit count" "previous_tag=
changed=true
version_code=3
version_name=0.1.3
tag=android-v0.1.3" "$("$here/plan.sh" 2>/dev/null)"

git tag android-v0.1.3
commit macos/b.swift "feat: a Mac change"
commit server/main.go "fix: a server fix"
expect "Mac and server changes after a release don't release" "previous_tag=android-v0.1.3
changed=false" "$("$here/plan.sh" 2>/dev/null)"

git checkout --quiet -b dropped android-v0.1.3
commit android/app/a.kt "fix: an app fix whose release was dropped"
git checkout --quiet main
git merge --quiet --no-edit dropped
expect "an app change since the release still releases after a server-only commit" "previous_tag=android-v0.1.3
changed=true
version_code=7
version_name=0.1.7
tag=android-v0.1.7" "$("$here/plan.sh" 2>/dev/null)"

sed -i.bak 's/"0.1"/"0.2"/' android/app/build.gradle.kts && rm android/app/build.gradle.kts.bak
git commit --quiet -am "feat: version 0.2"
expect "reads versionMajorMinor from the app's build file" "version_name=0.2.8" \
	"$("$here/plan.sh" 2>/dev/null | grep '^version_name=')"

git tag android-v0.1.20 HEAD~1
git tag android-v0.1.nope HEAD
commit android/app/a.kt "feat: after a higher tag"
expect "refuses a version code that isn't above every release" \
	"::error::versionCode 9 is not above android-v0.1.20's 20; a phone would refuse it as a downgrade." \
	"$("$here/plan.sh" 2>&1 >/dev/null || true)"
if "$here/plan.sh" >/dev/null 2>&1; then
	expect "the refusal exits non-zero" "1" "0"
fi

sed -i.bak '/versionMajorMinor/d' android/app/build.gradle.kts && rm android/app/build.gradle.kts.bak
git tag -d android-v0.1.20 >/dev/null
git commit --quiet -am "refactor: lose the version"
expect "fails without versionMajorMinor" \
	"::error::No 'val versionMajorMinor = \"<major>.<minor>\"' line in android/app/build.gradle.kts." \
	"$("$here/plan.sh" 2>&1 >/dev/null || true)"

finish
