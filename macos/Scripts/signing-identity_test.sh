#!/bin/bash
# Tests signing-identity.sh against a fake keychain: a security command on
# PATH that prints synthetic find-identity output and test certificates, each
# with its team in the Organizational Unit, as Apple's are.
set -euo pipefail

script="$(cd "$(dirname "$0")" && pwd)/signing-identity.sh"
work="$(mktemp -d "${TMPDIR:-/tmp}/signing-identity-test.XXXXXX")"
trap 'rm -rf "$work"' EXIT
failures=0

mkdir -p "$work/bin"
cat > "$work/bin/security" <<'FAKE'
#!/bin/sh
case "$1" in
find-identity) cat "$KEYCHAIN/identities" ;;
find-certificate) cat "$KEYCHAIN/certificates" ;;
*) exit 1 ;;
esac
FAKE
chmod +x "$work/bin/security"

owner_hash=1111111111111111111111111111111111111111
work_hash=2222222222222222222222222222222222222222
distribution_hash=3333333333333333333333333333333333333333
owner="Apple Development: owner@example.com (AAAAAAAAAA)"
work_identity="Apple Development: Owner Name (BBBBBBBBBB)"
distribution="Apple Distribution: Owner Name (OWNERTEAM1)"

# certificate SHA1 CN TEAM appends a test certificate to the fake keychain.
certificate() {
  echo "SHA-256 hash: 0000000000000000000000000000000000000000000000000000000000000000"
  echo "SHA-1 hash: $1"
  openssl req -x509 -newkey rsa:2048 -nodes -keyout "$work/key" -days 1 \
    -subj "/CN=$2/OU=$3/O=Owner Name/C=US" 2>/dev/null
}

# keychain NAME LINE... makes a fake keychain whose find-identity prints each
# identity line.
keychain() {
  local dir="$work/$1"
  shift
  mkdir -p "$dir"
  {
    local n=0
    for line in "$@"; do
      n=$((n + 1))
      echo "  $n) $line"
    done
    echo "     $n valid identities found"
  } > "$dir/identities"
  {
    certificate "$owner_hash" "$owner" OWNERTEAM1
    certificate "$work_hash" "$work_identity" WORKTEAM22
    certificate "$distribution_hash" "$distribution" OWNERTEAM1
  } > "$dir/certificates"
}

keychain empty
keychain distribution-only "$distribution_hash \"$distribution\""
keychain one "$owner_hash \"$owner\"" "$distribution_hash \"$distribution\""
keychain two "$owner_hash \"$owner\"" "$work_hash \"$work_identity\""

# run KEYCHAIN [VAR=value...]: runs the script with a fresh home folder,
# setting status, out (stdout) and err (stderr).
run() {
  local keychain="$1"
  shift
  home="$work/home-$keychain-$RANDOM"
  mkdir -p "$home"
  status=0
  env -u CODESIGN_TEAM_ID -u CODESIGN_IDENTITY HOME="$home" KEYCHAIN="$work/$keychain" PATH="$work/bin:$PATH" "$@" \
    "$script" > "$work/out" 2> "$work/err" || status=$?
  out="$(cat "$work/out")"
  err="$(cat "$work/err")"
}

pin() {
  cat "$home/.config/job-search-hub/codesign-team-id" 2>/dev/null || echo "(none)"
}

expect() {
  if [ "$2" = "$3" ]; then
    echo "ok   $1"
  else
    echo "FAIL $1"
    echo "  expected: $2"
    echo "  got:      $3"
    failures=$((failures + 1))
  fi
}

contains() {
  case "$3" in
  *"$2"*) echo "ok   $1" ;;
  *)
    echo "FAIL $1"
    echo "  expected to contain: $2"
    echo "  got: $3"
    failures=$((failures + 1))
    ;;
  esac
}

run empty
expect "no identity: signs ad hoc" "0 -" "$status $out"
contains "no identity: says so" "signing ad hoc" "$err"
expect "no identity: pins nothing" "(none)" "$(pin)"

run distribution-only
expect "no Apple Development identity: signs ad hoc" "0 -" "$status $out"
expect "no Apple Development identity: pins nothing" "(none)" "$(pin)"

run one
expect "one identity: signs with it" "0 $owner_hash OWNERTEAM1" "$status $out"
contains "one identity: names it and its team" "\"$owner\", the keychain's one Apple Development identity, of team OWNERTEAM1" "$err"
expect "one identity: pins its team" "OWNERTEAM1" "$(pin)"

# The pin holds once a second identity, of another team, joins the keychain.
pinned_home="$home"
status=0
HOME="$pinned_home" KEYCHAIN="$work/two" PATH="$work/bin:$PATH" "$script" > "$work/out" 2> "$work/err" || status=$?
expect "pinned, then a second identity: keeps the pinned team's" "0 $owner_hash OWNERTEAM1" "$status $(cat "$work/out")"

run two
expect "two identities: refuses" "1 " "$status $out"
contains "two identities: lists the first and its team" "\"$owner\", team OWNERTEAM1" "$err"
contains "two identities: lists the second and its team" "\"$work_identity\", team WORKTEAM22" "$err"
expect "two identities: pins nothing" "(none)" "$(pin)"

run two CODESIGN_TEAM_ID=WORKTEAM22
expect "CODESIGN_TEAM_ID picks that team's identity" "0 $work_hash WORKTEAM22" "$status $out"
expect "CODESIGN_TEAM_ID pins nothing" "(none)" "$(pin)"

run two CODESIGN_IDENTITY="owner@example.com"
expect "CODESIGN_IDENTITY narrows two to one" "0 $owner_hash OWNERTEAM1" "$status $out"
expect "CODESIGN_IDENTITY's choice is pinned" "OWNERTEAM1" "$(pin)"

run one CODESIGN_TEAM_ID=OTHERTEAM3
expect "a pinned team with no identity in the keychain: refuses" "1 " "$status $out"
contains "a pinned team with no identity: says so" "No signing identity of team OTHERTEAM3" "$err"

run one CODESIGN_TEAM_ID=nope
expect "a malformed team ID: refuses" "1 " "$status $out"

run two CODESIGN_IDENTITY=-
expect "CODESIGN_IDENTITY=- signs ad hoc" "0 -" "$status $out"
expect "CODESIGN_IDENTITY=- pins nothing" "(none)" "$(pin)"

[ "$failures" -eq 0 ] || {
  echo "$failures failed"
  exit 1
}
