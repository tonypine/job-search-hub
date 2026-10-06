#!/bin/sh
# Tests check-certificate.sh against certificates made here, each with its
# team in the Organizational Unit, as Apple's are. `openssl ca` makes the
# expired one, since it alone takes explicit dates in every OpenSSL and
# LibreSSL the runners and the Mac have.

# shellcheck source=scripts/release/testlib.sh
. "$(dirname "$0")/testlib.sh"

work=$(mktemp -d "${TMPDIR:-/tmp}/check-certificate-test.XXXXXX")
trap 'rm -rf "$work"' EXIT
owner="Apple Development: owner@example.com (AAAAAAAAAA)"
work_identity="Apple Development: Owner Name (BBBBBBBBBB)"

# certificate <file> <name> <team> <days>: a certificate valid for that many days.
certificate() {
	openssl req -x509 -newkey rsa:2048 -nodes -keyout "$work/key" -days "$4" \
		-subj "/CN=$2/OU=$3/O=Owner Name/C=US" -out "$1" 2>/dev/null
}

# expired_certificate <file> <name> <team>: one that ran out on 2025-02-01.
expired_certificate() {
	mkdir -p "$work/ca"
	: >"$work/ca/index.txt"
	echo 01 >"$work/ca/serial"
	cat >"$work/ca/ca.cnf" <<CONFIG
[ca]
default_ca = test
[test]
database = $work/ca/index.txt
serial = $work/ca/serial
new_certs_dir = $work/ca
default_md = sha256
policy = any
[any]
commonName = supplied
organizationalUnitName = optional
organizationName = optional
countryName = optional
CONFIG
	openssl req -new -newkey rsa:2048 -nodes -keyout "$work/key" \
		-subj "/CN=$2/OU=$3/O=Owner Name/C=US" -out "$work/csr" 2>/dev/null
	openssl ca -batch -config "$work/ca/ca.cnf" -selfsign -keyfile "$work/key" -in "$work/csr" \
		-startdate 20250101000000Z -enddate 20250201000000Z -out "$work/ca/out.pem" 2>/dev/null
	openssl x509 -in "$work/ca/out.pem" -out "$1"
}

# check <pem>: runs the check for OWNERTEAM1, setting status, out and err.
check() {
	status=0
	"$here/check-certificate.sh" OWNERTEAM1 "$1" >"$work/out" 2>"$work/err" || status=$?
	out=$(cat "$work/out")
	err=$(cat "$work/err")
}

# end_date <pem>: the certificate's end date, as the script prints it.
end_date() {
	openssl x509 -in "$1" -noout -enddate | sed 's/^notAfter=//'
}

renew="Renew it as the README's Releases › Renewing the Mac certificate says."

certificate "$work/valid.pem" "$owner" OWNERTEAM1 365
certificate "$work/other.pem" "$work_identity" WORKTEAM22 365
cat "$work/other.pem" "$work/valid.pem" >"$work/both.pem"
check "$work/both.pem"
expect "a valid certificate of the team: named in the summary, no annotation" "0
Signing with \"$owner\" of team OWNERTEAM1, valid until $(end_date "$work/valid.pem").
" "$status
$out
$err"

certificate "$work/soon.pem" "$owner" OWNERTEAM1 10
check "$work/soon.pem"
message="The signing certificate \"$owner\" of team OWNERTEAM1 expires on $(end_date "$work/soon.pem"), in under 30 days. $renew"
expect "a certificate expiring within 30 days: warns, and passes" "0
$message
::warning::$message" "$status
$out
$err"

expired_certificate "$work/expired.pem" "$owner" OWNERTEAM1
check "$work/expired.pem"
message="The signing certificate \"$owner\" of team OWNERTEAM1 expired on Feb  1 00:00:00 2025 GMT. $renew"
expect "an expired certificate: fails by name" "1
$message
::error::$message" "$status
$out
$err"

check "$work/other.pem"
message="MAC_SIGNING_CERTIFICATE_BASE64 holds no certificate of team OWNERTEAM1, the MAC_SIGNING_TEAM_ID."
expect "only another team's certificate: fails" "1
$message
::error::$message" "$status
$out
$err"

finish
