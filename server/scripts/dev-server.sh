#!/bin/sh
# Runs the real hub-server with a made-up job search in it, for Symphony's QA
# pass, which runs it in the QA VM beside the Mac app.
#
#   server/scripts/dev-server.sh build   builds hub-server and dev-server-seed into server/bin/dev-server/,
#                                        and fetches the Postgres engine when none is on the PATH
#   server/scripts/dev-server.sh start   runs the hub in the foreground until TERM or Ctrl-C
#
# start creates a Postgres cluster in $TMPDIR/hub-dev-server/<pid>/ and
# starts it on a Unix socket there alone, migrates and seeds it with
# dev-server-seed, then runs hub-server on 127.0.0.1:$SYMPHONY_VERIFICATION_PORT
# (8090, the Mac app's default hub URL) with the owner token below, a QA
# value and no secret. On exit it stops the cluster and deletes the folder;
# a later start clears what a killed run left there.
#
# hub-server runs with HOME in that folder, so it never reads the owner's
# server.env, and with the pollers, the model work, Claude, CV printing and
# backups off. Its prompts are the made-up ones the tests use.
#
# The Postgres engine is the one on the PATH (initdb, pg_ctl and postgres, of
# the major version server/postgres-engine.lock pins), or else the one build
# downloads from the lock into server/bin/dev-server/engine/, refused unless
# it matches the lock's SHA-256.

set -eu

server=$(cd "$(dirname "$0")/.." && pwd)
out=$server/bin/dev-server
port=${SYMPHONY_VERIFICATION_PORT:-8090}
address=127.0.0.1:$port
token=qa-dev-server-owner-token-not-a-secret

fail() {
	echo "dev-server: $*" >&2
	exit 1
}

lock=$server/postgres-engine.lock
engine_version=$(sed -n 's/^version=//p' "$lock")
engine_url=$(sed -n 's/^url=//p' "$lock")
engine_sha256=$(sed -n 's/^sha256=//p' "$lock")
[ -n "$engine_version" ] && [ -n "$engine_url" ] && [ -n "$engine_sha256" ] || fail "$lock doesn't pin an engine."
engine_major=${engine_version%%.*}

# path_engine prints the bin folder of the engine on the PATH: the folder
# initdb really lives in, holding pg_ctl and postgres of the pinned major
# version. A client-only install, such as Homebrew's libpq, has no postgres.
path_engine() {
	initdb=$(command -v initdb 2>/dev/null) || return 1
	command -v pg_ctl >/dev/null 2>&1 || return 1
	bin=$(dirname "$(realpath "$initdb" 2>/dev/null || echo "$initdb")")
	[ -x "$bin/pg_ctl" ] && [ -x "$bin/postgres" ] || return 1
	version=$("$bin/postgres" --version 2>/dev/null | awk '{print $NF}')
	[ "${version%%.*}" = "$engine_major" ] || return 1
	echo "$bin"
}

# find_engine prints the bin folder of the engine start uses: the PATH's, or
# the one build downloaded.
find_engine() {
	if path_engine; then
		return 0
	fi
	if [ -x "$out/engine/bin/postgres" ] && [ -x "$out/engine/bin/initdb" ] && [ -x "$out/engine/bin/pg_ctl" ]; then
		echo "$out/engine/bin"
		return 0
	fi
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
	go=$(find_go) || fail "Go isn't installed: install Go $(sed -n 's/^go //p' "$server/go.mod") or later (https://go.dev/dl/) to build the dev server."
	mkdir -p "$out"
	GOCACHE=${TMPDIR:-/tmp}
	GOCACHE=${GOCACHE%/}/hub-dev-server-go-build
	export GOCACHE
	(cd "$server" && "$go" build -o "$out/" ./cmd/hub-server ./scripts/dev-server-seed)
	echo "Built hub-server and dev-server-seed in $out"

	if bin=$(path_engine); then
		echo "Postgres engine on the PATH: $bin"
		return
	fi
	if [ -x "$out/engine/bin/postgres" ] && [ "$(cat "$out/engine/.sha256" 2>/dev/null || true)" = "$engine_sha256" ]; then
		echo "Postgres engine: $out/engine"
		return
	fi
	echo "No Postgres $engine_major on the PATH; downloading Postgres $engine_version from $engine_url"
	download=$out/engine.download
	rm -rf "$download"
	mkdir -p "$download/unpacked"
	curl -fsSL --retry 3 -o "$download/engine.tar.gz" "$engine_url" || fail "Couldn't download $engine_url."
	if ! echo "$engine_sha256  $download/engine.tar.gz" | shasum -a 256 -c - >/dev/null 2>&1; then
		rm -rf "$download"
		fail "The engine from $engine_url doesn't match the SHA-256 in $lock; not using it."
	fi
	tar -xzf "$download/engine.tar.gz" -C "$download/unpacked"
	# The archive holds the engine's folder, or its contents.
	unpacked=$download/unpacked
	if [ ! -x "$unpacked/bin/postgres" ]; then
		unpacked=$(dirname "$(dirname "$(find "$download/unpacked" -path '*/bin/postgres' -type f | head -n 1)")")
	fi
	[ -x "$unpacked/bin/postgres" ] && [ -x "$unpacked/bin/initdb" ] && [ -x "$unpacked/bin/pg_ctl" ] ||
		fail "The engine from $engine_url has no bin/postgres, bin/initdb and bin/pg_ctl."
	rm -rf "$out/engine"
	mv "$unpacked" "$out/engine"
	echo "$engine_sha256" >"$out/engine/.sha256"
	rm -rf "$download"
	echo "Postgres $engine_version verified, in $out/engine"
}

# stop_cluster stops the cluster in data, gently when it can.
stop_cluster() {
	data=$1
	[ -f "$data/postmaster.pid" ] || return 0
	"$engine/pg_ctl" -D "$data" -m fast -w -t 30 stop >/dev/null 2>&1 ||
		"$engine/pg_ctl" -D "$data" -m immediate -w -t 30 stop >/dev/null 2>&1 || true
}

# clear_stale stops what a killed run left in its folder, the hub-server and
# the cluster, and deletes the folder. A folder whose run is still going is
# left alone: it is named after that run's process.
clear_stale() {
	for stale in "$parent"/*; do
		[ -d "$stale" ] || continue
		run=$(basename "$stale")
		if [ "$run" != "$$" ] && kill -0 "$run" 2>/dev/null; then
			continue
		fi
		hub_pid=$(cat "$stale/hub-server.pid" 2>/dev/null || true)
		if [ -n "$hub_pid" ] && [ "$(ps -o comm= -p "$hub_pid" 2>/dev/null | xargs basename 2>/dev/null)" = hub-server ]; then
			# It lets go of the port once it has stopped.
			kill -TERM "$hub_pid" 2>/dev/null || true
			waited=0
			while kill -0 "$hub_pid" 2>/dev/null && [ "$waited" -lt 30 ]; do
				sleep 1
				waited=$((waited + 1))
			done
			kill -KILL "$hub_pid" 2>/dev/null || true
		fi
		stop_cluster "$stale/data"
		rm -rf "$stale"
		echo "Cleared $stale, which a killed run left."
	done
}

start() {
	for command in hub-server dev-server-seed; do
		[ -x "$out/$command" ] || fail "No $out/$command: run $0 build first."
	done
	engine=$(find_engine) || fail "No Postgres $engine_major engine on the PATH or in $out/engine: run $0 build first."

	parent=${TMPDIR:-/tmp}
	parent=${parent%/}/hub-dev-server
	# The socket's path is limited to 103 characters on macOS.
	if [ ${#parent} -gt 70 ]; then
		parent=/tmp/hub-dev-server-$(id -u)
	fi
	mkdir -p "$parent"
	chmod 700 "$parent"
	clear_stale

	root=$parent/$$
	mkdir -m 700 "$root"
	data=$root/data
	server_pid=

	cleanup() {
		trap - EXIT HUP INT TERM
		if [ -n "$server_pid" ]; then
			kill -TERM "$server_pid" 2>/dev/null || true
			wait "$server_pid" 2>/dev/null || true
		fi
		stop_cluster "$data"
		rm -rf "$root"
	}
	trap cleanup EXIT
	trap 'exit 129' HUP
	trap 'exit 130' INT
	trap 'exit 143' TERM

	"$engine/initdb" -D "$data" --username=hub --auth-local=trust --encoding=UTF8 \
		--locale-provider=builtin --builtin-locale=C.UTF-8 >"$root/initdb.log" 2>&1 ||
		{
			cat "$root/initdb.log" >&2
			fail "initdb failed."
		}
	# Durability is off: the cluster is deleted when the run ends.
	"$engine/pg_ctl" -D "$data" -l "$root/postgres.log" -w -t 60 start \
		-o "-c listen_addresses='' -c unix_socket_directories='$root' -c unix_socket_permissions=0700 -c fsync=off -c synchronous_commit=off -c full_page_writes=off" >/dev/null ||
		{
			cat "$root/postgres.log" >&2
			fail "Postgres didn't start."
		}

	# The socket's folder goes in the URL's host, percent-encoded where it
	# could break the query string.
	host=$(printf '%s' "$root" | sed -e 's/%/%25/g' -e 's/ /%20/g' -e 's/&/%26/g' -e 's/#/%23/g' -e 's/+/%2B/g')
	database_url="postgres:///postgres?host=$host&user=hub"
	prompts=$server/internal/testdatabase/agentprompts
	HUB_DATABASE_URL=$database_url HUB_AGENT_PROMPTS_DIR=$prompts "$out/dev-server-seed"

	mkdir "$root/home"
	env HOME="$root/home" \
		HUB_ADDR="$address" \
		HUB_OWNER_TOKEN="$token" \
		HUB_DATABASE_URL="$database_url" \
		HUB_PUBLIC_URL="http://$address" \
		HUB_AGENT_PROMPTS_DIR="$prompts" \
		HUB_BOARD_POLL_INTERVAL=0 \
		HUB_FEED_POLL_INTERVAL=0 \
		HUB_BOARD_SEARCH_INTERVAL=0 \
		HUB_BOARD_DISCOVERY_INTERVAL=0 \
		HUB_JOB_FACTS_INTERVAL=0 \
		HUB_JOB_FACTS_MODEL_URL= \
		HUB_LLAMA_SERVER="$root/no-llama-server" \
		HUB_CLAUDE_BIN="$root/no-claude" \
		HUB_CV_PRINT_BIN="$root/no-hub-cvprint" \
		HUB_CV_FOLDER="$root/cvs" \
		HUB_PG_DUMP="$root/no-pg-dump" \
		HUB_BACKUPS_DIR="$root/backups" \
		"$out/hub-server" &
	server_pid=$!
	echo "$server_pid" >"$root/hub-server.pid"
	echo "Hub dev server at http://$address, owner token $token, Postgres $("$engine/postgres" --version | awk '{print $NF}') in $root"
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
