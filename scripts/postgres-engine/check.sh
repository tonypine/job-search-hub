#!/bin/sh
# Refuses a Postgres engine tarball from build.sh that isn't fit to publish.
# It unpacks the tarball into a new folder, then checks that:
#
#   - it holds the seven programs, libpq, share/ and COPYRIGHT, and every
#     program reports the version in the tarball's name;
#   - every binary and library is arm64, signed, and loads only the system's
#     libraries and, through @loader_path, files in the folder (otool -L), with
#     no LC_RPATH;
#   - a cluster created there with the hub's initdb options starts on a Unix
#     socket alone, and a database with the types the hub uses survives
#     pg_dump and pg_restore into another database.
#
#   check.sh <postgres-<version>-macos-arm64.tar.gz>
#
# Runs with a bare environment, so nothing outside the folder can help it
# pass. Exits non-zero at the first failure.

set -eu
export LC_ALL=C

if [ $# -ne 1 ]; then
	echo "usage: $0 <postgres-<version>-macos-arm64.tar.gz>" >&2
	exit 2
fi
tarball=$1

fail() {
	echo "::error::$*" >&2
	exit 1
}

# The names in a folder, sorted, on one line.
names() {
	(cd "$1" && echo *)
}

PROGRAMS="initdb pg_ctl pg_dump pg_isready pg_restore postgres psql"

[ -f "$tarball" ] || fail "No tarball at $tarball."
version=$(basename "$tarball" | sed -n 's/^postgres-\([0-9][0-9]*\.[0-9][0-9]*\)-macos-arm64\.tar\.gz$/\1/p')
[ -n "$version" ] || fail "$tarball isn't named postgres-<major>.<minor>-macos-arm64.tar.gz."

# Short names: a Unix socket's path is limited to 103 characters on macOS.
root=$(mktemp -d "${TMPDIR:-/tmp}/pgcheck.XXXXXX")
engine=$root/postgres-${version%%.*}
data=$root/data
socket=$root/s

# Runs a program with nothing from the caller's environment.
run() {
	env -i HOME="$root" PATH=/usr/bin:/bin:/usr/sbin:/sbin TMPDIR="$root" "$@"
}

cleanup() {
	if [ -f "$data/postmaster.pid" ]; then
		"$engine/bin/pg_ctl" -D "$data" -m immediate stop >/dev/null 2>&1 || true
	fi
	rm -rf "$root"
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM

tar -xzf "$tarball" -C "$root"
echo "Unpacked into $engine"

echo "== Contents"
[ "$(names "$root")" = "$(basename "$engine")" ] ||
	fail "The tarball holds $(names "$root") rather than one folder $(basename "$engine")."
[ "$(names "$engine/bin")" = "$PROGRAMS" ] || fail "bin/ holds $(names "$engine/bin") rather than $PROGRAMS."
for file in COPYRIGHT lib/libpq.5.dylib lib/postgresql/plpgsql.dylib share/postgresql/postgres.bki \
	share/postgresql/postgresql.conf.sample share/postgresql/timezone/UTC; do
	[ -f "$engine/$file" ] || fail "$file is missing."
done
for program in $PROGRAMS; do
	reported=$(run "$engine/bin/$program" --version)
	echo "$reported"
	case $reported in
	*" $version") ;;
	*) fail "$program reports '$reported', not $version." ;;
	esac
done

echo "== Binaries and libraries"
find "$engine" -type f >"$root/files"
count=0
while IFS= read -r file; do
	file -b "$file" | grep -q Mach-O || continue
	count=$((count + 1))
	relative=${file#"$engine/"}
	dir=$(dirname "$file")

	archs=$(lipo -archs "$file")
	[ "$archs" = arm64 ] || fail "$relative is built for '$archs', not arm64 alone."
	codesign --verify --strict "$file" 2>/dev/null || fail "$relative's signature doesn't verify."
	if otool -l "$file" 2>/dev/null | grep -q LC_RPATH; then
		fail "$relative has an LC_RPATH."
	fi

	echo "$relative"
	otool -L "$file" 2>/dev/null | tail -n +2 | awk '{print $1}' >"$root/loads"
	while IFS= read -r load; do
		echo "  $load"
		case $load in
		/usr/lib/* | /System/Library/*) ;;
		@loader_path/*)
			[ -f "$dir/${load#@loader_path/}" ] || fail "$relative loads $load, which isn't in the folder."
			;;
		*) fail "$relative loads $load, which is neither the system's nor @loader_path." ;;
		esac
	done <"$root/loads"
done <"$root/files"
[ "$count" -ge 9 ] || fail "Only $count Mach-O files; expected the seven programs, libpq and the modules."
echo "$count binaries and libraries load only the system's libraries and @loader_path."

echo "== A cluster"
sql() {
	run "$engine/bin/psql" -X -q -A -t -v ON_ERROR_STOP=1 -h "$socket" -U hub "$@"
}

mkdir -m 0700 "$socket"
run "$engine/bin/initdb" -D "$data" --username=hub --auth-local=trust --encoding=UTF8 \
	--locale-provider=builtin --builtin-locale=C.UTF-8 >"$root/initdb.log" 2>&1 ||
	{ cat "$root/initdb.log"; fail "initdb failed."; }
run "$engine/bin/pg_ctl" -D "$data" -l "$root/postgres.log" -w -t 60 start \
	-o "-c listen_addresses='' -c unix_socket_directories=$socket -c unix_socket_permissions=0700" >/dev/null ||
	{ cat "$root/postgres.log"; fail "The cluster didn't start."; }
run "$engine/bin/pg_isready" -h "$socket" -U hub -d postgres

sql -d postgres -c 'CREATE DATABASE source' -c 'CREATE DATABASE restored'
# plpgsql, the english stemmer and a conversion each load a module from lib/.
sql -d source <<'SQL'
CREATE TABLE things (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	name text NOT NULL,
	tags text[] NOT NULL,
	details jsonb NOT NULL,
	created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX things_name ON things (lower(name));
INSERT INTO things (name, tags, details)
SELECT 'Thing ' || n, ARRAY['tag' || n % 7, 'tag' || n % 11], jsonb_build_object('n', n, 'even', n % 2 = 0)
FROM generate_series(1, 2000) AS n;
CREATE FUNCTION thing_count() RETURNS bigint LANGUAGE plpgsql AS $$
BEGIN
	RETURN (SELECT count(*) FROM things);
END
$$;
SELECT to_tsvector('english', 'running checks');
SELECT convert_to('café', 'LATIN1');
SQL

run "$engine/bin/pg_dump" -h "$socket" -U hub -Fc -f "$root/source.dump" source
run "$engine/bin/pg_restore" -h "$socket" -U hub -d restored --no-owner --no-privileges --exit-on-error "$root/source.dump"

digest="SELECT thing_count(), md5(string_agg(t::text, ',' ORDER BY id)) FROM things AS t"
source=$(sql -d source -c "$digest")
restored=$(sql -d restored -c "$digest")
echo "source:   $source"
echo "restored: $restored"
case $source in
2000\|*) ;;
*) fail "The source database holds '$source' rather than 2000 rows." ;;
esac
[ "$source" = "$restored" ] || fail "The restored database differs from the source."

run "$engine/bin/pg_ctl" -D "$data" -m fast -w stop >/dev/null
echo "Postgres $version from $tarball passes."
