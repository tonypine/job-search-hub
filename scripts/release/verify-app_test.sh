#!/bin/sh
# Tests verify-app.sh against a made-up bundle and fake codesign, otool and
# plutil commands on PATH, so it runs on Linux too. The fakes print what the
# real ones do (see `codesign --display --verbose=2` on a signed binary), from
# per-file answers this test writes in $FAKE.

# shellcheck source=scripts/release/testlib.sh
. "$(dirname "$0")/testlib.sh"

work=$(mktemp -d "${TMPDIR:-/tmp}/verify-app-test.XXXXXX")
trap 'rm -rf "$work"' EXIT
FAKE=$work/fake
export FAKE
mkdir -p "$work/bin"

# codesign --verify --strict --deep <app> | codesign --display --verbose=2 <file>
cat >"$work/bin/codesign" <<'TOOL'
#!/bin/sh
for file; do :; done
case $1 in
--verify)
	if [ -f "$FAKE/verify-fails" ]; then
		echo "$file: a sealed resource is missing or invalid" >&2
		exit 1
	fi
	;;
--display)
	answer=$FAKE/signatures/$(basename "$file")
	[ -f "$answer" ] || answer=$FAKE/signatures/default
	if [ "$(cat "$answer")" = unsigned ]; then
		echo "$file: code object is not signed at all" >&2
		exit 1
	fi
	echo "Executable=$file" >&2
	cat "$answer" >&2
	;;
*) exit 2 ;;
esac
TOOL

# otool -L <file>
cat >"$work/bin/otool" <<'TOOL'
#!/bin/sh
answer=$FAKE/libraries/$(basename "$2")
[ -f "$answer" ] || answer=$FAKE/libraries/default
echo "$2:"
sed 's/^/	/' "$answer"
TOOL

# plutil -extract <key> raw -o - <file>
cat >"$work/bin/plutil" <<'TOOL'
#!/bin/sh
awk -v key="<key>$2</key>" '
	found { sub(/^[[:space:]]*<string>/, ""); sub(/<\/string>[[:space:]]*$/, ""); print; ok = 1; exit }
	index($0, key) { found = 1 }
	END { exit !ok }
' "$6"
TOOL
chmod +x "$work/bin/codesign" "$work/bin/otool" "$work/bin/plutil"

# signature <team> <code-directory flags>: what codesign prints for a file
# signed by an Apple Development identity of that team.
signature() {
	cat <<SIGNATURE
Identifier=com.tonypine.JobSearchHub
Format=Mach-O thin (arm64)
CodeDirectory v=20500 size=268 flags=$2 hashes=2+2 location=embedded
Signature size=4790
Authority=Apple Development: owner@example.com (AAAAAAAAAA)
Authority=Apple Worldwide Developer Relations Certification Authority
Authority=Apple Root CA
TeamIdentifier=$1
Runtime Version=26.0.0
Sealed Resources=none
Internal requirements count=1 size=180
SIGNATURE
}

adhoc="Identifier=hub-server
Format=Mach-O thin (arm64)
CodeDirectory v=20500 size=268 flags=0x10002(adhoc,runtime) hashes=2+2 location=embedded
Signature=adhoc
Info.plist=not bound
TeamIdentifier=not set
Runtime Version=26.0.0
Sealed Resources=none"

system_libraries="/usr/lib/libSystem.B.dylib (compatibility version 1.0.0, current version 1351.0.0)
/System/Library/Frameworks/Security.framework/Versions/A/Security (compatibility version 1.0.0, current version 61901.0.0)
@rpath/libswiftCore.dylib (compatibility version 1.0.0, current version 6.2.0)"

app=$work/JobSearchHub.app

# fresh: a bundle at 0.1.42 whose every executable is signed by OWNERTEAM1
# with the hardened runtime, linking only the system.
fresh() {
	rm -rf "$app" "$FAKE"
	mkdir -p "$app/Contents/MacOS" "$app/Contents/Helpers/bin" "$app/Contents/Resources" \
		"$FAKE/signatures" "$FAKE/libraries"
	cat >"$app/Contents/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
	<key>CFBundleShortVersionString</key>
	<string>0.1.42</string>
	<key>CFBundleVersion</key>
	<string>0.1.42</string>
</dict>
</plist>
PLIST
	echo icon >"$app/Contents/Resources/AppIcon.icns"
	echo app >"$app/Contents/MacOS/JobSearchHub"
	echo hub >"$app/Contents/Helpers/bin/hub"
	server_version 0.1.42
	chmod +x "$app/Contents/MacOS/JobSearchHub" "$app/Contents/Helpers/bin/hub"
	signature OWNERTEAM1 '0x10000(runtime)' >"$FAKE/signatures/default"
	printf '%s\n' "$system_libraries" >"$FAKE/libraries/default"
}

server_version() {
	printf '#!/bin/sh\necho "hub-server %s"\necho "commit 0123abc"\n' "$1" >"$app/Contents/Helpers/bin/hub-server"
	chmod +x "$app/Contents/Helpers/bin/hub-server"
}

# verify: runs verify-app.sh for 0.1.42 and OWNERTEAM1, setting status, out
# and err.
verify() {
	status=0
	PATH="$work/bin:$PATH" "$here/verify-app.sh" "$app" 0.1.42 OWNERTEAM1 >"$work/out" 2>"$work/err" || status=$?
	out=$(cat "$work/out")
	err=$(cat "$work/err")
}

fresh
verify
expect "a bundle signed by the team, hardened, at the version: passes" "0
$app is ready: 0.1.42, its 3 executables signed by team OWNERTEAM1 with the hardened runtime, linking only the system and the bundle." \
	"$status
$out"

fresh
printf '%s\n' "$adhoc" >"$FAKE/signatures/default"
verify
expect "an ad hoc signature: refused, for every executable" "1
::error::Contents/Helpers/bin/hub is signed ad hoc, not by team OWNERTEAM1.
::error::Contents/Helpers/bin/hub-server is signed ad hoc, not by team OWNERTEAM1.
::error::Contents/MacOS/JobSearchHub is signed ad hoc, not by team OWNERTEAM1.
::error::$app is not fit to publish: 3 problem(s) above." "$status
$err"

fresh
signature WORKTEAM22 '0x10000(runtime)' >"$FAKE/signatures/hub-server"
verify
expect "a helper signed by another team: refused by name" "1
::error::Contents/Helpers/bin/hub-server is signed by team WORKTEAM22, not OWNERTEAM1.
::error::$app is not fit to publish: 1 problem(s) above." "$status
$err"

fresh
signature OWNERTEAM1 '0x0(none)' >"$FAKE/signatures/JobSearchHub"
verify
expect "no hardened runtime: refused" "1
::error::Contents/MacOS/JobSearchHub lacks the hardened runtime.
::error::$app is not fit to publish: 1 problem(s) above." "$status
$err"

fresh
sed 's/0\.1\.42/0.1.41/' "$app/Contents/Info.plist" >"$work/plist" && mv "$work/plist" "$app/Contents/Info.plist"
verify
expect "another version in Info.plist: refused" "1
::error::Info.plist's CFBundleShortVersionString is 0.1.41, not 0.1.42.
::error::Info.plist's CFBundleVersion is 0.1.41, not 0.1.42.
::error::$app is not fit to publish: 2 problem(s) above." "$status
$err"

fresh
server_version 0.1.0-dev.0123abc
verify
expect "another version in hub-server --version: refused" "1
::error::hub-server --version says \"hub-server 0.1.0-dev.0123abc\", not \"hub-server 0.1.42\".
::error::$app is not fit to publish: 1 problem(s) above." "$status
$err"

fresh
rm "$app/Contents/Helpers/bin/hub-server"
verify
expect "no server in the bundle: refused" "1
::error::$app has no Contents/Helpers/bin/hub-server.
::error::$app is not fit to publish: 1 problem(s) above." "$status
$err"

fresh
printf '%s\n%s\n' "$system_libraries" \
	"/opt/homebrew/opt/icu4c/lib/libicuuc.77.dylib (compatibility version 77.0.0, current version 77.1.0)" \
	>"$FAKE/libraries/hub"
verify
expect "a Homebrew library: refused" "1
::error::Contents/Helpers/bin/hub links /opt/homebrew/opt/icu4c/lib/libicuuc.77.dylib, outside the system and the bundle.
::error::$app is not fit to publish: 1 problem(s) above." "$status
$err"

fresh
echo unsigned >"$FAKE/signatures/hub"
verify
expect "an unsigned executable: refused" "1
::error::Contents/Helpers/bin/hub is not signed.
::error::$app is not fit to publish: 1 problem(s) above." "$status
$err"

fresh
touch "$FAKE/verify-fails"
verify
expect "a bundle codesign doesn't verify: refused" "1
$app: a sealed resource is missing or invalid
::error::$app fails codesign --verify --strict --deep.
::error::$app is not fit to publish: 1 problem(s) above." "$status
$err"

status=0
PATH="$work/bin:$PATH" "$here/verify-app.sh" "$work/nothing.app" 0.1.42 OWNERTEAM1 >/dev/null 2>"$work/err" || status=$?
expect "no bundle: refused" "1
::error::No app bundle at $work/nothing.app." "$status
$(cat "$work/err")"

finish
