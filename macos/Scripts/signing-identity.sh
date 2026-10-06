#!/bin/bash
# Chooses the identity make-app.sh signs the bundle with, and prints it with
# its team: "<SHA-1> <team ID>", or "-" to sign ad hoc. Messages go to stderr.
#
# The bundle and every executable in it are signed by the owner's team, pinned
# by its ID, so another identity in the keychain, such as a work certificate,
# is never picked. An Apple-issued identity keeps the app's designated
# requirement stable across builds, so its Keychain item (the owner token)
# survives them.
#
#   - CODESIGN_TEAM_ID names the team, or ~/.config/job-search-hub/codesign-team-id
#     holds it. Only that team's identities will do; CODESIGN_IDENTITY can
#     narrow the choice to one of them.
#   - With nothing pinned and one Apple Development identity in the keychain,
#     its team is the owner's: it's written to codesign-team-id, so later
#     builds keep it after a second identity is added.
#   - With nothing pinned and two or more, it fails and lists them, rather than
#     guess.
#   - With no Apple Development identity at all, as in CI and Symphony's QA VM,
#     or with CODESIGN_IDENTITY=-, the bundle is signed ad hoc, and the
#     Keychain asks for access after every build.
set -euo pipefail

team_file="$HOME/.config/job-search-hub/codesign-team-id"

fail() {
  echo "$*" >&2
  exit 1
}

# team_of SHA1 NAME prints the team of the identity's certificate, its OU.
team_of() {
  security find-certificate -a -c "$2" -Z -p 2>/dev/null |
    awk -v hash="$1" '/^SHA-1 hash:/ { current = $3 } /BEGIN CERTIFICATE/ { printing = (current == hash) } printing { print } /END CERTIFICATE/ { printing = 0 }' |
    openssl x509 -noout -subject 2>/dev/null | sed -n 's/.*OU *= *\([A-Z0-9]*\).*/\1/p' || true
}

# identities prints the keychain's valid code-signing identities, one
# "<SHA-1> <name>" a line.
identities() {
  security find-identity -v -p codesigning 2>/dev/null |
    sed -n 's/^ *[0-9]*) \([0-9A-F]\{40\}\) "\(.*\)"$/\1 \2/p' || true
}

if [ "${CODESIGN_IDENTITY:-}" = "-" ]; then
  echo "-"
  exit 0
fi

team_id="${CODESIGN_TEAM_ID:-}"
if [ -z "$team_id" ] && [ -f "$team_file" ]; then
  team_id="$(tr -d '[:space:]' < "$team_file")"
fi

if [ -z "$team_id" ]; then
  candidates=()
  while read -r hash name; do
    case "$name" in "Apple Development:"*) ;; *) continue ;; esac
    case "$name" in *"${CODESIGN_IDENTITY:-}"*) ;; *) continue ;; esac
    candidates+=("$hash $name")
  done < <(identities)

  case ${#candidates[@]} in
  0)
    if [ -n "${CODESIGN_IDENTITY:-}" ]; then
      fail "No Apple Development identity matching \"$CODESIGN_IDENTITY\" in the keychain."
    fi
    echo "No Apple Development identity in the keychain, so signing ad hoc: the Keychain will ask for access after every build." >&2
    echo "Create one in Xcode > Settings > Accounts > Manage Certificates, and the next build signs with it." >&2
    echo "-"
    exit 0
    ;;
  1)
    read -r hash name <<<"${candidates[0]}"
    team_id="$(team_of "$hash" "$name")"
    [[ "$team_id" =~ ^[A-Z0-9]{10}$ ]] || fail "Couldn't read the team of \"$name\" from its certificate's Organizational Unit."
    mkdir -p "$(dirname "$team_file")"
    pin="$(mktemp "$team_file.XXXXXX")"
    echo "$team_id" > "$pin"
    mv "$pin" "$team_file"
    echo "Signing with \"$name\", the keychain's one Apple Development identity, of team $team_id; pinned in $team_file." >&2
    echo "$hash $team_id"
    exit 0
    ;;
  *)
    {
      echo "The keychain holds ${#candidates[@]} Apple Development identities, and no signing team is pinned:"
      for candidate in "${candidates[@]}"; do
        read -r hash name <<<"$candidate"
        echo "  \"$name\", team $(team_of "$hash" "$name")"
      done
      echo "Set CODESIGN_TEAM_ID to yours, or write it to $team_file."
    } >&2
    exit 1
    ;;
  esac
fi

if ! [[ "$team_id" =~ ^[A-Z0-9]{10}$ ]]; then
  fail "\"$team_id\" isn't a team ID, ten capital letters and digits, as Keychain Access shows in the certificate's Organizational Unit."
fi
while read -r hash name; do
  case "$name" in *"${CODESIGN_IDENTITY:-}"*) ;; *) continue ;; esac
  if [ "$(team_of "$hash" "$name")" = "$team_id" ]; then
    echo "Signing with \"$name\" of team $team_id" >&2
    echo "$hash $team_id"
    exit 0
  fi
done < <(identities)
echo "No signing identity of team $team_id${CODESIGN_IDENTITY:+ matching \"$CODESIGN_IDENTITY\"} in the keychain, and no other team's will do." >&2
fail "Create an Apple Development certificate in Xcode > Settings > Accounts > Manage Certificates."
