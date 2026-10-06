#!/bin/sh
# Checks the Mac signing certificate before a release signs with it. Of the
# PEM certificates given, the release keychain's, it takes the signing team's,
# by their Organizational Unit as Apple's certificates carry it, and prints a
# line on each for the run's summary. It warns 30 days before one expires, and
# fails, naming it, once it has, or when none is the team's.
#
#   check-certificate.sh <team-id> <certificates.pem>
#
# Annotations (::warning::, ::error::) go to stderr, the summary to stdout.

set -eu

if [ $# -ne 2 ]; then
	echo "usage: $0 <team-id> <certificates.pem>" >&2
	exit 2
fi
team=$1 pem=$2
warn_seconds=$((30 * 24 * 60 * 60))
renew="Renew it as the README's Releases › Renewing the Mac certificate says."

# say <level> <message>: a summary line, and an annotation unless level is -.
say() {
	echo "$2"
	[ "$1" = - ] || echo "::$1::$2" >&2
}

work=$(mktemp -d "${TMPDIR:-/tmp}/check-certificate.XXXXXX")
trap 'rm -rf "$work"' EXIT

# One file per certificate.
awk -v dir="$work" '
	/-----BEGIN CERTIFICATE-----/ { n++; file = dir "/" n ".pem" }
	file { print > file }
	/-----END CERTIFICATE-----/ { close(file); file = "" }
' "$pem"

found=0 expired=0
for certificate in "$work"/*.pem; do
	[ -f "$certificate" ] || continue
	subject=$(openssl x509 -in "$certificate" -noout -subject -nameopt multiline,-esc_msb,utf8)
	unit=$(printf '%s\n' "$subject" | sed -n 's/^ *organizationalUnitName *= *//p')
	[ "$unit" = "$team" ] || continue
	found=$((found + 1))
	name=$(printf '%s\n' "$subject" | sed -n 's/^ *commonName *= *//p')
	ends=$(openssl x509 -in "$certificate" -noout -enddate | sed 's/^notAfter=//')

	if ! openssl x509 -in "$certificate" -noout -checkend 0 >/dev/null; then
		say error "The signing certificate \"$name\" of team $team expired on $ends. $renew"
		expired=1
	elif ! openssl x509 -in "$certificate" -noout -checkend "$warn_seconds" >/dev/null; then
		say warning "The signing certificate \"$name\" of team $team expires on $ends, in under 30 days. $renew"
	else
		say - "Signing with \"$name\" of team $team, valid until $ends."
	fi
done

if [ "$found" -eq 0 ]; then
	say error "MAC_SIGNING_CERTIFICATE_BASE64 holds no certificate of team $team, the MAC_SIGNING_TEAM_ID."
	exit 1
fi
[ "$expired" -eq 0 ]
