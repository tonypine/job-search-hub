#!/bin/sh
# Refuses a Mac app bundle that isn't fit to publish: one that fails
# `codesign --verify --strict --deep`, holds an executable or library signed
# ad hoc, by another team or without the hardened runtime, links a library
# outside the system and the bundle (Homebrew's, say), or carries another
# version than planned in its Info.plist or in `hub-server --version`. It
# names every problem it finds, then fails.
#
#   verify-app.sh <app> <version> <team-id>

set -eu

if [ $# -ne 3 ]; then
	echo "usage: $0 <app> <version> <team-id>" >&2
	exit 2
fi
app=${1%/} version=$2 team=$3

problems=0
problem() {
	echo "::error::$*" >&2
	problems=$((problems + 1))
}

if [ ! -d "$app/Contents" ]; then
	echo "::error::No app bundle at $app." >&2
	exit 1
fi

if ! verify=$(codesign --verify --strict --deep "$app" 2>&1); then
	printf '%s\n' "$verify" >&2
	problem "$app fails codesign --verify --strict --deep."
fi

# check_signature <file> <name>
check_signature() {
	if ! details=$(codesign --display --verbose=2 "$1" 2>&1); then
		problem "$2 is not signed."
		return
	fi
	if printf '%s\n' "$details" | grep -qx 'Signature=adhoc'; then
		problem "$2 is signed ad hoc, not by team $team."
		return
	fi
	signer=$(printf '%s\n' "$details" | sed -n 's/^TeamIdentifier=//p')
	if [ "$signer" != "$team" ]; then
		problem "$2 is signed by team ${signer:-(none)}, not $team."
	fi
	if ! printf '%s\n' "$details" | grep -q '^CodeDirectory .*flags=0x[0-9a-f]*([^)]*runtime'; then
		problem "$2 lacks the hardened runtime."
	fi
}

# check_libraries <file> <name>: the system's libraries, and the bundle's
# through @rpath, @loader_path or @executable_path, are the only ones allowed.
check_libraries() {
	if ! libraries=$(otool -L "$1" 2>&1); then
		problem "$2 isn't a Mach-O file otool can read."
		return
	fi
	outside=$(printf '%s\n' "$libraries" |
		sed -n 's/^[[:space:]][[:space:]]*\(.*\) (compatibility version .*/\1/p' |
		grep -v -e '^/usr/lib/' -e '^/System/Library/' -e '^@rpath/' -e '^@loader_path/' -e '^@executable_path/' |
		sort -u) || true
	while IFS= read -r library; do
		[ -n "$library" ] || continue
		problem "$2 links $library, outside the system and the bundle."
	done <<EOF
$outside
EOF
}

executables=0
while IFS= read -r file; do
	[ -n "$file" ] || continue
	executables=$((executables + 1))
	name=${file#"$app/"}
	check_signature "$file" "$name"
	check_libraries "$file" "$name"
done <<EOF
$(find "$app/Contents" -type f \( -perm -100 -o -name '*.dylib' -o -name '*.so' \) | sort)
EOF
[ "$executables" -gt 0 ] || problem "$app holds no executable."

for key in CFBundleShortVersionString CFBundleVersion; do
	found=$(plutil -extract "$key" raw -o - "$app/Contents/Info.plist" 2>/dev/null) || found=
	[ "$found" = "$version" ] || problem "Info.plist's $key is ${found:-missing}, not $version."
done

server=$app/Contents/Helpers/bin/hub-server
if [ -x "$server" ]; then
	found=$("$server" --version 2>/dev/null | head -n 1) || found=
	[ "$found" = "hub-server $version" ] || problem "hub-server --version says \"$found\", not \"hub-server $version\"."
else
	problem "$app has no Contents/Helpers/bin/hub-server."
fi

if [ "$problems" -gt 0 ]; then
	echo "::error::$app is not fit to publish: $problems problem(s) above." >&2
	exit 1
fi
echo "$app is ready: $version, its $executables executables signed by team $team with the hardened runtime, linking only the system and the bundle."
