#!/bin/sh
# Runs the server's Go tests against a throwaway Postgres from the hub's own
# engine, with no Docker. It creates a cluster in a new temporary folder,
# starts it on a Unix socket there alone, points HUB_TEST_DATABASE_URL at it,
# runs go test in server/ with the arguments given, and stops and deletes the
# cluster on exit: after the tests, a failure, or Ctrl-C.
#
#   server/scripts/test-with-postgres.sh                     go test ./...
#   server/scripts/test-with-postgres.sh ./internal/store    any go test arguments, as in server/
#
# The engine is postgres-18 in the installed hub's engines folder,
# ~/Library/Application Support/JobSearchHub/engines, or in
# HUB_POSTGRES_ENGINES. The run also gets HUB_TEST_POSTGRES_ENGINE, unless it
# is set, and the engine's bin/ first on PATH, so the lifecycle tests and the
# pg_dump backup test run against it too.

set -eu

engines=${HUB_POSTGRES_ENGINES:-"$HOME/Library/Application Support/JobSearchHub/engines"}
engine=$engines/postgres-18
if [ ! -x "$engine/bin/postgres" ]; then
	echo "No Postgres 18 engine: $engine/bin/postgres isn't there." >&2
	echo "Install the hub, or set HUB_POSTGRES_ENGINES to the folder holding postgres-18." >&2
	exit 1
fi

server=$(cd "$(dirname "$0")/.." && pwd)

# A short folder: the socket's path is limited to 103 characters on macOS.
root=$(mktemp -d "${TMPDIR:-/tmp}/hubtest.XXXXXX")
root=${root%/}
data=$root/data

cleanup() {
	if [ -f "$data/postmaster.pid" ]; then
		"$engine/bin/pg_ctl" -D "$data" -m fast -w -t 30 stop >/dev/null 2>&1 ||
			"$engine/bin/pg_ctl" -D "$data" -m immediate -w stop >/dev/null 2>&1 || true
	fi
	rm -rf "$root"
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM

"$engine/bin/initdb" -D "$data" --username=hub --auth-local=trust --encoding=UTF8 \
	--locale-provider=builtin --builtin-locale=C.UTF-8 >"$root/initdb.log" 2>&1 ||
	{
		cat "$root/initdb.log" >&2
		echo "initdb failed." >&2
		exit 1
	}
# pg_ctl starts postgres in a session of its own, so Ctrl-C reaches only the
# tests, and cleanup stops postgres. Durability is off: the cluster is
# deleted when the run ends.
"$engine/bin/pg_ctl" -D "$data" -l "$root/postgres.log" -w -t 60 start \
	-o "-c listen_addresses='' -c unix_socket_directories='$root' -c unix_socket_permissions=0700 -c fsync=off -c synchronous_commit=off -c full_page_writes=off" >/dev/null ||
	{
		cat "$root/postgres.log" >&2
		echo "The test Postgres didn't start." >&2
		exit 1
	}

# The socket's folder goes in the URL's host, percent-encoded where it could
# break the query string.
host=$(printf '%s' "$root" | sed -e 's/%/%25/g' -e 's/ /%20/g' -e 's/&/%26/g' -e 's/#/%23/g' -e 's/+/%2B/g')
HUB_TEST_DATABASE_URL="postgres:///postgres?host=$host&user=hub"
HUB_TEST_POSTGRES_ENGINE=${HUB_TEST_POSTGRES_ENGINE:-$engine}
PATH=$engine/bin:$PATH
export HUB_TEST_DATABASE_URL HUB_TEST_POSTGRES_ENGINE PATH
echo "Postgres $("$engine/bin/postgres" --version | awk '{print $NF}') from $engine, in $root"

if [ $# -eq 0 ]; then
	set -- ./...
fi
cd "$server"
status=0
go test "$@" || status=$?
exit "$status"
