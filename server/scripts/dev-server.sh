#!/bin/sh
# Runs a hub with a made-up job search in it, for Symphony's QA pass, which
# points the app at it (WORKFLOW.md, verification.dev_server).
#
#   server/scripts/dev-server.sh build   builds hub-server and the seed into server/bin/dev-server,
#                                        and fetches the Postgres engine when none is installed
#   server/scripts/dev-server.sh start   runs it until stopped: Ctrl-C or TERM
#
# start creates a Postgres cluster in a new temporary folder and starts it on
# a Unix socket there alone, seeds it with dev-server-seed (a Pipeline board
# with overdue cards and the people to write to about them), then runs
# hub-server in the foreground on HUB_DEV_SERVER_ADDR (127.0.0.1:18190) with
# the owner token in HUB_DEV_SERVER_OWNER_TOKEN, or the QA token below, which
# is no secret. On exit it stops the cluster and deletes the folder.
#
# hub-server runs with HOME in that folder, so it never reads the owner's
# server.env, and with the pollers, the model work, Claude and CV printing
# off: nothing it does leaves the folder but the reads of public job data.
# Its prompts are the made-up ones the tests use.
#
# The engine is the folder in HUB_DEV_SERVER_POSTGRES_ENGINE (one holding
# bin/postgres, as CI's /usr/lib/postgresql/18), or postgres-18 in
# HUB_POSTGRES_ENGINES, in the installed hub's engines folder, or the one build
# fetched into server/bin/dev-server/engines from server/postgres-engine.lock,
# refused unless it matches the SHA-256 there.

set -eu

server=$(cd "$(dirname "$0")/.." && pwd)
out=$server/bin/dev-server
address=${HUB_DEV_SERVER_ADDR:-127.0.0.1:18190}
token=${HUB_DEV_SERVER_OWNER_TOKEN:-qa-dev-server-owner-token-not-a-secret}

fail() {
	echo "dev-server: $*" >&2
	exit 1
}

lock=$server/postgres-engine.lock
engine_version=$(sed -n 's/^version=//p' "$lock")
engine_url=$(sed -n 's/^url=//p' "$lock")
engine_sha256=$(sed -n 's/^sha256=//p' "$lock")
[ -n "$engine_version" ] && [ -n "$engine_url" ] && [ -n "$engine_sha256" ] || fail "$lock doesn't pin an engine."
engine_name=postgres-${engine_version%%.*}

# find_engine prints the first engine folder there is, or fails.
find_engine() {
	if [ -n "${HUB_DEV_SERVER_POSTGRES_ENGINE:-}" ]; then
		[ -x "$HUB_DEV_SERVER_POSTGRES_ENGINE/bin/postgres" ] || fail "HUB_DEV_SERVER_POSTGRES_ENGINE has no bin/postgres."
		echo "$HUB_DEV_SERVER_POSTGRES_ENGINE"
		return 0
	fi
	for engines in ${HUB_POSTGRES_ENGINES:+"$HUB_POSTGRES_ENGINES"} "$HOME/Library/Application Support/JobSearchHub/engines" "$out/engines"; do
		if [ -x "$engines/$engine_name/bin/postgres" ]; then
			echo "$engines/$engine_name"
			return 0
		fi
	done
	return 1
}

# find_go prints the go command: the PATH's, or the one Homebrew or Go's
# installer puts in place, which a launcher's bare PATH may leave out.
find_go() {
	for candidate in "$(command -v go 2>/dev/null || true)" /opt/homebrew/bin/go /usr/local/go/bin/go /usr/local/bin/go; do
		if [ -n "$candidate" ] && [ -x "$candidate" ]; then
			echo "$candidate"
			return 0
		fi
	done
	return 1
}

build() {
	go=$(find_go) || fail "No Go toolchain: install Go to build the dev server's hub-server."
	mkdir -p "$out"
	(cd "$server" && "$go" build -o "$out/" ./cmd/hub-server ./scripts/dev-server-seed ./scripts/remove-orphaned-segments)
	echo "Built hub-server, dev-server-seed and remove-orphaned-segments in $out"

	if engine=$(find_engine); then
		echo "Postgres engine: $engine"
		return
	fi
	echo "==> Downloading Postgres $engine_version"
	download=$out/engines/$engine_name.download
	rm -rf "$download"
	mkdir -p "$download"
	curl -fsSL --retry 3 -o "$download/engine.tar.gz" "$engine_url"
	if ! echo "$engine_sha256  $download/engine.tar.gz" | shasum -a 256 -c - >/dev/null; then
		rm -rf "$download"
		fail "The engine from $engine_url doesn't match the SHA-256 in $lock; not using it."
	fi
	tar -xzf "$download/engine.tar.gz" -C "$download"
	[ -x "$download/$engine_name/bin/postgres" ] || fail "The engine has no $engine_name/bin/postgres."
	rm -rf "${out:?}/engines/$engine_name"
	mv "$download/$engine_name" "$out/engines/$engine_name"
	rm -rf "$download"
	echo "Postgres $engine_version verified, in $out/engines/$engine_name"
}

start() {
	for command in hub-server dev-server-seed remove-orphaned-segments; do
		[ -x "$out/$command" ] || fail "No $out/$command: run $0 build first."
	done
	engine=$(find_engine) || fail "No Postgres engine: run $0 build first."

	# A short folder: the socket's path is limited to 103 characters on macOS.
	parent=${TMPDIR:-/tmp}
	parent=${parent%/}/hub-dev-server
	if [ ${#parent} -gt 60 ]; then
		parent=/tmp/hub-dev-server-$(id -u)
	fi
	mkdir -p "$parent"
	chmod 700 "$parent"

	# A run that was killed leaves its cluster running: it is stopped and its
	# folder deleted, unless the run that made it is still going.
	for stale in "$parent"/*; do
		[ -d "$stale" ] || continue
		owner=$(cat "$stale/dev-server.pid" 2>/dev/null || true)
		if [ -n "$owner" ] && kill -0 "$owner" 2>/dev/null; then
			continue
		fi
		if [ -f "$stale/data/postmaster.pid" ]; then
			"$engine/bin/pg_ctl" -D "$stale/data" -m immediate -w stop >/dev/null 2>&1 || true
		fi
		rm -rf "$stale"
	done
	"$out/remove-orphaned-segments" || echo "Couldn't remove every orphaned shared-memory segment; ipcs -m -a lists them." >&2

	root=$(mktemp -d "$parent/XXXXXX")
	root=${root%/}
	echo "$$" >"$root/dev-server.pid"
	data=$root/data
	server_pid=

	cleanup() {
		if [ -n "$server_pid" ]; then
			kill -TERM "$server_pid" 2>/dev/null || true
			wait "$server_pid" 2>/dev/null || true
		fi
		if [ -f "$data/postmaster.pid" ]; then
			"$engine/bin/pg_ctl" -D "$data" -m fast -w -t 30 stop >/dev/null 2>&1 ||
				"$engine/bin/pg_ctl" -D "$data" -m immediate -w stop >/dev/null 2>&1 || true
		fi
		"$out/remove-orphaned-segments" || true
		rm -rf "$root"
	}
	trap cleanup EXIT
	trap 'exit 143' HUP INT TERM

	"$engine/bin/initdb" -D "$data" --username=hub --auth-local=trust --encoding=UTF8 \
		--locale-provider=builtin --builtin-locale=C.UTF-8 >"$root/initdb.log" 2>&1 ||
		{
			cat "$root/initdb.log" >&2
			fail "initdb failed."
		}
	# Durability is off: the cluster is deleted when the run ends.
	"$engine/bin/pg_ctl" -D "$data" -l "$root/postgres.log" -w -t 60 start \
		-o "-c listen_addresses='' -c unix_socket_directories='$root' -c unix_socket_permissions=0700 -c fsync=off -c synchronous_commit=off -c full_page_writes=off" >/dev/null ||
		{
			cat "$root/postgres.log" >&2
			fail "Postgres didn't start."
		}

	# The socket's folder goes in the URL's host, percent-encoded where it
	# could break the query string.
	host=$(printf '%s' "$root" | sed -e 's/%/%25/g' -e 's/ /%20/g' -e 's/&/%26/g' -e 's/#/%23/g' -e 's/+/%2B/g')
	database_url="postgres:///postgres?host=$host&user=hub"
	HUB_DATABASE_URL=$database_url "$out/dev-server-seed"

	mkdir -p "$root/home"
	env HOME="$root/home" \
		HUB_ADDR="$address" \
		HUB_OWNER_TOKEN="$token" \
		HUB_DATABASE_URL="$database_url" \
		HUB_PUBLIC_URL="http://$address" \
		HUB_AGENT_PROMPTS_DIR="$server/internal/testdatabase/agentprompts" \
		HUB_BOARD_POLL_INTERVAL=0 \
		HUB_FEED_POLL_INTERVAL=0 \
		HUB_BOARD_SEARCH_INTERVAL=0 \
		HUB_BOARD_DISCOVERY_INTERVAL=0 \
		HUB_JOB_FACTS_MODEL_URL= \
		HUB_JOB_FACTS_INTERVAL=0 \
		HUB_CLAUDE_BIN="$root/no-claude" \
		HUB_LLAMA_SERVER="$root/no-llama-server" \
		HUB_RUNTIME_PORT=18195 \
		HUB_CV_PRINT_BIN="$root/no-hub-cvprint" \
		HUB_CV_FOLDER="$root/cvs" \
		HUB_BACKUPS_DIR="$root/backups" \
		HUB_PG_DUMP="$engine/bin/pg_dump" \
		"$out/hub-server" &
	server_pid=$!
	echo "Hub dev server at http://$address, owner token $token, Postgres $("$engine/bin/postgres" --version | awk '{print $NF}') in $root"
	status=0
	wait "$server_pid" || status=$?
	server_pid=
	exit "$status"
}

case ${1:-} in
build) build ;;
start) start ;;
*)
	echo "usage: $0 build|start" >&2
	exit 2
	;;
esac
