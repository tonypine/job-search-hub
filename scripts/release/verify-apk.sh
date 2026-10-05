#!/bin/sh
# Refuses an APK that isn't fit to publish: unsigned, signed with the Android
# debug key, debuggable, or with another package or version than expected.
#
#   verify-apk.sh <apk> <package> <version-code> <version-name>
#
# Uses the newest build tools under $ANDROID_HOME (or $ANDROID_SDK_ROOT).

set -eu

if [ $# -ne 4 ]; then
	echo "usage: $0 <apk> <package> <version-code> <version-name>" >&2
	exit 2
fi
apk=$1 package=$2 version_code=$3 version_name=$4

fail() {
	echo "::error::$*" >&2
	exit 1
}

sdk=${ANDROID_HOME:-${ANDROID_SDK_ROOT:-}}
[ -n "$sdk" ] || fail "Set ANDROID_HOME to the Android SDK."
build_tools=$(find "$sdk/build-tools" -mindepth 1 -maxdepth 1 -type d | sort -V | tail -n 1)
[ -n "$build_tools" ] || fail "No build tools in $sdk/build-tools."
echo "Build tools: $build_tools"

[ -f "$apk" ] || fail "No APK at $apk."

if ! certs=$("$build_tools/apksigner" verify --verbose --print-certs "$apk" 2>&1); then
	printf '%s\n' "$certs" >&2
	fail "$apk does not verify; it is unsigned or its signature is broken."
fi
printf '%s\n' "$certs" | grep -E '^(Verified using|Signer #1 certificate (DN|SHA-256))'
if printf '%s\n' "$certs" | grep -q 'CN=Android Debug'; then
	fail "$apk is signed with the Android debug key."
fi

badging=$("$build_tools/aapt2" dump badging "$apk")
first=$(printf '%s\n' "$badging" | head -n 1)
echo "$first"
expected="package: name='$package' versionCode='$version_code' versionName='$version_name'"
case $first in
"$expected" | "$expected "*) ;;
*) fail "Expected $expected." ;;
esac
if printf '%s\n' "$badging" | grep -q '^application-debuggable'; then
	fail "$apk is debuggable."
fi

echo "$apk is signed and ready: $package $version_name ($version_code)."
