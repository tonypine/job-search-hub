#!/bin/sh
# Tests plan.sh against a throwaway repository.

# shellcheck source=scripts/release/testlib.sh
. "$(dirname "$0")/testlib.sh"

new_repo
mkdir -p android/app
printf 'plugins {}\n\nval versionMajorMinor = "0.1"\n' >android/app/build.gradle.kts
git add android/app/build.gradle.kts
git commit --quiet -m "feat: the app"

expect "the first commit releases the app it holds" "mac_previous_tag=
mac_changed=false
android_previous_tag=
android_changed=true
version_code=1
version_name=0.1.1
android_tag=android-v0.1.1" "$("$here/plan.sh" 2>/dev/null)"

commit server/main.go "feat: a server change"
expect "a server change releases the Mac app alone" "mac_previous_tag=
mac_changed=true
android_previous_tag=
android_changed=false
version_code=2
version_name=0.1.2
mac_tag=mac-v0.1.2" "$("$here/plan.sh" 2>/dev/null)"
git tag mac-v0.1.2

commit macos/a.swift "feat: a Mac change"
expect "a Mac change releases the Mac app alone" "mac_previous_tag=mac-v0.1.2
mac_changed=true
android_previous_tag=
android_changed=false
version_code=3
version_name=0.1.3
mac_tag=mac-v0.1.3" "$("$here/plan.sh" 2>/dev/null)"
git tag mac-v0.1.3

commit android/app/a.kt "feat: an app change"
expect "an Android change releases the phone alone" "mac_previous_tag=mac-v0.1.3
mac_changed=false
android_previous_tag=
android_changed=true
version_code=4
version_name=0.1.4
android_tag=android-v0.1.4" "$("$here/plan.sh" 2>/dev/null)"
git tag android-v0.1.4

commit docs/a.md "docs: a note"
commit scripts/release/a.sh "chore: a script"
expect "docs and scripts release neither" "mac_previous_tag=mac-v0.1.3
mac_changed=false
android_previous_tag=android-v0.1.4
android_changed=false" "$("$here/plan.sh" 2>/dev/null)"
expect "and says why for each" "Nothing under server/ macos/ changed since mac-v0.1.3; no mac release.
Nothing under android/ changed since android-v0.1.4; no android release." "$("$here/plan.sh" 2>&1 >/dev/null)"

commit macos/a.swift "feat: a Mac change"
commit android/app/a.kt "fix: an app fix"
expect "both changed: both release with the same version" "mac_previous_tag=mac-v0.1.3
mac_changed=true
android_previous_tag=android-v0.1.4
android_changed=true
version_code=8
version_name=0.1.8
mac_tag=mac-v0.1.8
android_tag=android-v0.1.8" "$("$here/plan.sh" 2>/dev/null)"
git tag mac-v0.1.8
git tag android-v0.1.8

commit server/main.go "fix: a server fix whose release was dropped"
commit docs/a.md "docs: a later note"
expect "a change since the release still releases after a docs-only commit" "mac_previous_tag=mac-v0.1.8
mac_changed=true
android_previous_tag=android-v0.1.8
android_changed=false
version_code=10
version_name=0.1.10
mac_tag=mac-v0.1.10" "$("$here/plan.sh" 2>/dev/null)"

sed -i.bak 's/"0.1"/"0.2"/' android/app/build.gradle.kts && rm android/app/build.gradle.kts.bak
git commit --quiet -am "feat: version 0.2"
expect "reads versionMajorMinor from the app's build file, for both" "version_name=0.2.11
mac_tag=mac-v0.2.11
android_tag=android-v0.2.11" "$("$here/plan.sh" 2>/dev/null | grep -E '^(version_name|mac_tag|android_tag)=')"
git tag android-v0.2.11

git tag mac-v0.1.20 HEAD~1
git tag mac-v0.1.nope HEAD
commit macos/a.swift "feat: after a higher tag"
expect "refuses a version code that isn't above every release" \
	"::error::Version code 12 is not above mac-v0.1.20's 20; the app would refuse it as a downgrade." \
	"$("$here/plan.sh" 2>&1 >/dev/null | grep '^::error::' || true)"
if "$here/plan.sh" >/dev/null 2>&1; then
	expect "the refusal exits non-zero" "1" "0"
fi
git tag -d mac-v0.1.20 >/dev/null

sed -i.bak '/versionMajorMinor/d' android/app/build.gradle.kts && rm android/app/build.gradle.kts.bak
git commit --quiet -am "refactor: lose the version"
expect "fails without versionMajorMinor" \
	"::error::No 'val versionMajorMinor = \"<major>.<minor>\"' line in android/app/build.gradle.kts." \
	"$("$here/plan.sh" 2>&1 >/dev/null | grep '^::error::' || true)"

finish
