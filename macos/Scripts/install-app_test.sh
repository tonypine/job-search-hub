#!/bin/bash
# Tests install-app.sh against a fake Mac: a home folder of its own, and
# launchctl, curl, codesign, pgrep, osascript, open and sleep on PATH that act
# out launchd, the server, the engine's download, the signature and the app.
# The script runs from a fake repository whose lock pins a fake engine, which
# the fake curl serves. The fake launchd keeps the program it runs in a file;
# the fake server answers as that program's version while one is loaded.
# Failures are injected by version:
#
#   FAKE_REFUSE_VERSION  launchd refuses to load an agent whose program is it
#   FAKE_STUCK_VERSION   a server of it doesn't stop when its agent is booted out
#   FAKE_DEAD_VERSION    a server of it never answers
#   FAKE_ANSWER_VERSION  every server says it is this version on /v1/version
#   FAKE_PRINT_PROGRAM   launchd says it runs this program, whatever it loaded
#   FAKE_MOVE_FAILS      the new server's move off Docker's Postgres fails
#   FAKE_NO_POSTGRES     /v1/health never reports the database the server owns
#
# The fake server moves a server.env off Docker's Postgres as the real one
# does, without the import: it drops HUB_DATABASE_URL and
# HUB_DATABASE_PASSWORD when the URL is compose.yaml's, and logs the import.
# The fake /v1/health reports the database the server owns while the run's
# server.env has no HUB_DATABASE_URL.
set -euo pipefail

work="$(mktemp -d "${TMPDIR:-/tmp}/install-app-test.XXXXXX")"
trap 'rm -rf "$work"' EXIT
failures=0
label=com.tonypine.jobsearchhub.server

mkdir -p "$work/repo/macos/Scripts" "$work/repo/server" "$work/engine/postgres-18/bin"
cp "$(cd "$(dirname "$0")" && pwd)/install-app.sh" "$work/repo/macos/Scripts/"
script="$work/repo/macos/Scripts/install-app.sh"
printf '#!/bin/sh\n' > "$work/engine/postgres-18/bin/postgres"
chmod +x "$work/engine/postgres-18/bin/postgres"
tar -czf "$work/engine.tar.gz" -C "$work/engine" postgres-18
engine_version=18.0
engine_sha256="$(shasum -a 256 "$work/engine.tar.gz" | cut -d ' ' -f 1)"
printf 'version=%s\nurl=https://engines.invalid/postgres-18.tar.gz\nsha256=%s\n' "$engine_version" "$engine_sha256" > "$work/repo/server/postgres-engine.lock"

mkdir -p "$work/bin"
cat > "$work/bin/launchctl" <<'FAKE'
#!/bin/sh
echo "$*" >> "$FAKE/launchctl.log"
case "$1" in
bootstrap)
  [ ! -f "$FAKE/loaded" ] || { echo "Bootstrap failed: 37: Operation already in progress" >&2; exit 37; }
  program="$(plutil -extract ProgramArguments.0 raw "$3")" || exit 5
  if [ "$("$program" --version | sed -n 's/^hub-server //p')" = "${FAKE_REFUSE_VERSION:-}" ]; then
    echo "Bootstrap failed: 5: Input/output error" >&2
    exit 5
  fi
  echo "$program" > "$FAKE/loaded"
  ;;
bootout)
  [ -f "$FAKE/loaded" ] || exit 3
  [ "$("$(cat "$FAKE/loaded")" --version | sed -n 's/^hub-server //p')" != "${FAKE_STUCK_VERSION:-}" ] || exit 0
  rm -f "$FAKE/loaded"
  ;;
print)
  [ -f "$FAKE/loaded" ] || { echo "Could not find service \"$2\"" >&2; exit 113; }
  printf '%s = {\n\tstate = running\n\n\tprogram = %s\n}\n' "$2" "${FAKE_PRINT_PROGRAM:-$(cat "$FAKE/loaded")}"
  ;;
*) exit 1 ;;
esac
FAKE
cat > "$work/bin/curl" <<'FAKE'
#!/bin/sh
output=
while [ $# -gt 1 ]; do
  [ "$1" != -o ] || output="$2"
  shift
done
url="$1"
if [ -n "$output" ]; then
  echo "$url" >> "$FAKE/downloads.log"
  cp "$FAKE_ENGINE" "$output"
  exit
fi
[ -f "$FAKE/loaded" ] || exit 7
version="$("$(cat "$FAKE/loaded")" --version | sed -n 's/^hub-server //p')"
[ "$version" != "${FAKE_DEAD_VERSION:-}" ] || exit 7
postgres=
if [ -n "${FAKE_ENV_FILE:-}" ] && [ -z "${FAKE_NO_POSTGRES:-}" ] && ! grep -q '^HUB_DATABASE_URL=' "$FAKE_ENV_FILE"; then
  postgres=',"postgres":{"major":18}'
fi
case "$url" in
*/v1/health) printf '{"status":"ok"%s}\n' "$postgres" ;;
*/v1/version) printf '{"version":"%s","commit":"abc1234","newest_migration":1}\n' "${FAKE_ANSWER_VERSION:-$version}" ;;
*) exit 22 ;;
esac
FAKE
cat > "$work/bin/codesign" <<'FAKE'
#!/bin/sh
case "$1" in
--verify) exit 0 ;;
--display) echo "TeamIdentifier=OWNERTEAM1" >&2 ;;
*) exit 1 ;;
esac
FAKE
# The app answers as open to the first $FAKE/app-open checks.
cat > "$work/bin/pgrep" <<'FAKE'
#!/bin/sh
left="$(cat "$FAKE/app-open" 2>/dev/null || echo 0)"
[ "$left" -gt 0 ] || exit 1
echo $((left - 1)) > "$FAKE/app-open"
FAKE
cat > "$work/bin/osascript" <<'FAKE'
#!/bin/sh
echo "$*" >> "$FAKE/osascript.log"
FAKE
cat > "$work/bin/open" <<'FAKE'
#!/bin/sh
echo "$*" >> "$FAKE/open.log"
FAKE
printf '#!/bin/sh\n' > "$work/bin/sleep"
chmod +x "$work/bin"/*

# fake_server PATH VERSION writes a hub-server that prints VERSION, and
# moves a server.env off Docker's Postgres.
fake_server() {
  mkdir -p "$(dirname "$1")"
  cat > "$1" <<'SERVER'
#!/bin/sh
if [ "$1 $2" = "database move-from-compose" ]; then
  echo "$3" >> "$FAKE/moves.log"
  if [ -n "${FAKE_MOVE_FAILS:-}" ]; then
    echo "hub-server database: the import failed: the imported database's row counts differ from the source's in companies" >&2
    exit 1
  fi
  grep -q '^HUB_DATABASE_URL=postgres://hub:[^@]*@localhost:5434/hub$' "$3" || exit 0
  echo "$3" >> "$FAKE/imports.log"
  grep -v -e '^HUB_DATABASE_URL=' -e '^HUB_DATABASE_PASSWORD=' "$3" > "$3.moving"
  mv "$3.moving" "$3"
  exit 0
fi
SERVER
  printf 'echo "hub-server %s"\necho "commit abc1234"\n' "$2" >> "$1"
  chmod +x "$1"
}

# agent_plist PATH PROGRAM writes an agent that runs PROGRAM.
agent_plist() {
  mkdir -p "$(dirname "$1")"
  plutil -create xml1 "$1"
  plutil -insert Label -string "$label" "$1"
  plutil -insert ProgramArguments -array "$1"
  plutil -insert ProgramArguments.0 -string "$2" "$1"
}

# setup NAME LAYOUT makes a Mac whose server runs 0.1.0-old, as
# install-native-server.sh left it (native) or a previous install-app.sh
# (app), and a new build, 0.1.0-new, to install.
setup() {
  case_dir="$work/$1"
  home="$case_dir/home"
  fake="$case_dir/fake"
  build="$case_dir/JobSearchHub.app"
  installed="$home/Applications/Job Search Hub.app"
  app_support="$home/Library/Application Support/JobSearchHub"
  plist="$home/Library/LaunchAgents/$label.plist"
  env_file="$home/.config/job-search-hub/server.env"
  mkdir -p "$fake" "$case_dir/tmp" "$home/.config/job-search-hub" "$build/Contents/MacOS"
  fake_server "$build/Contents/Helpers/bin/hub-server" 0.1.0-new
  touch "$build/Contents/MacOS/JobSearchHub"
  engine="$app_support/engines/postgres-${engine_version%%.*}"
  mkdir -p "$engine"
  echo "$engine_sha256" > "$engine/.sha256"
  case "$2" in
  native)
    old_program="$app_support/bin/run-hub-server"
    printf 'HUB_ADDR=127.0.0.1:18090\nHUB_OWNER_TOKEN=test-token\nHUB_CV_PRINT_BIN=%s\n' "$app_support/bin/hub-cvprint" > "$env_file"
    ;;
  app)
    old_program="$installed/Contents/Helpers/bin/hub-server"
    printf 'HUB_ADDR=127.0.0.1:18090\nHUB_OWNER_TOKEN=test-token\n' > "$env_file"
    ;;
  esac
  chmod 600 "$env_file"
  fake_server "$old_program" 0.1.0-old
  agent_plist "$plist" "$old_program"
  echo "$old_program" > "$fake/loaded"
  cp "$plist" "$case_dir/plist.before"
  cp "$env_file" "$case_dir/env.before"
}

# run [VAR=value...] runs the script on the build, setting status, out and err.
run() {
  status=0
  env HOME="$home" TMPDIR="$case_dir/tmp" FAKE="$fake" FAKE_ENGINE="$work/engine.tar.gz" FAKE_ENV_FILE="$env_file" PATH="$work/bin:$PATH" CODESIGN_TEAM_ID=OWNERTEAM1 "$@" \
    "$script" "$build" > "$case_dir/out" 2> "$case_dir/err" || status=$?
  out="$(cat "$case_dir/out")"
  err="$(cat "$case_dir/err")"
}

expect() {
  if [ "$2" = "$3" ]; then
    echo "ok   $1"
  else
    echo "FAIL $1: expected \"$2\", got \"$3\""
    failures=$((failures + 1))
  fi
}

contains() {
  case "$3" in
  *"$2"*) echo "ok   $1" ;;
  *)
    echo "FAIL $1: \"$2\" not in:"
    echo "$3" | sed 's/^/       /'
    failures=$((failures + 1))
    ;;
  esac
}

# health prints what the fake server answers on /v1/health now.
health() {
  FAKE="$fake" FAKE_DEAD_VERSION="${1:-}" "$work/bin/curl" -fsS http://127.0.0.1:18090/v1/health 2>/dev/null || echo down
}
# on_compose points the case's server.env at Docker's Postgres, as one from
# before the server owned its database.
on_compose() {
  printf 'HUB_DATABASE_URL=postgres://hub:0123abcd@localhost:5434/hub\nHUB_DATABASE_PASSWORD=0123abcd\n' >> "$env_file"
  cp "$env_file" "$case_dir/env.before"
}
lines() { if [ -f "$1" ]; then wc -l < "$1" | tr -d ' '; else echo 0; fi; }
loaded_version() { "$(cat "$fake/loaded")" --version | sed -n 's/^hub-server //p'; }
same() { cmp -s "$1" "$2" && echo same || echo changed; }
exists() { [ -e "$1" ] && echo yes || echo no; }

setup first-install native
run
expect "from the native layout: installs" 0 "$status"
expect "from the native layout: launchd runs the bundle's server" "$installed/Contents/Helpers/bin/hub-server" "$(cat "$fake/loaded")"
expect "from the native layout: the new version answers" 0.1.0-new "$(loaded_version)"
expect "the agent's plist runs the bundle's server" "$installed/Contents/Helpers/bin/hub-server" "$(plutil -extract ProgramArguments.0 raw "$plist")"
expect "the agent's plist names the app for Login Items" com.tonypine.JobSearchHub "$(plutil -extract AssociatedBundleIdentifiers.0 raw "$plist")"
expect "the agent restarts only after a crash" false "$(plutil -extract KeepAlive.SuccessfulExit raw "$plist")"
expect "the agent leaves the server 100 s to stop" 100 "$(plutil -extract ExitTimeOut raw "$plist")"
expect "the old bin/ goes" no "$(exists "$app_support/bin")"
contains "the HUB_CV_PRINT_BIN that pointed at bin/ goes" "HUB_OWNER_TOKEN=test-token" "$(cat "$env_file")"
expect "the HUB_CV_PRINT_BIN that pointed at bin/ goes, alone" "" "$(grep HUB_CV_PRINT_BIN "$env_file" || true)"
expect "no staged or replaced copy is left" "no no" "$(exists "$installed.installing") $(exists "$installed.replaced")"
contains "says which version is up" "hub-server 0.1.0-new is up on http://127.0.0.1:18090" "$out"

setup second-install app
run
expect "a second install: installs" 0 "$status"
expect "a second install: launchd runs the new server" 0.1.0-new "$(loaded_version)"
expect "a second install: the installed bundle is the new build" "hub-server 0.1.0-new" "$("$installed/Contents/Helpers/bin/hub-server" --version | head -n 1)"
expect "a second install: the app wasn't open, so it isn't opened" no "$(exists "$fake/open.log")"

setup refused-native native
expect "refused, from the native layout: the hub answers before" '{"status":"ok"}' "$(health)"
run FAKE_REFUSE_VERSION=0.1.0-new
expect "refused, from the native layout: fails" 1 "$status"
expect "refused, from the native layout: the old agent runs again" "$app_support/bin/run-hub-server" "$(cat "$fake/loaded")"
expect "refused, from the native layout: the hub answers after" '{"status":"ok"}' "$(health)"
expect "refused, from the native layout: the old plist is back" same "$(same "$case_dir/plist.before" "$plist")"
expect "refused, from the native layout: server.env is as it was" same "$(same "$case_dir/env.before" "$env_file")"
expect "refused, from the native layout: bin/ stays" yes "$(exists "$app_support/bin/run-hub-server")"
expect "refused, from the native layout: no app is left installed" no "$(exists "$installed")"
contains "refused: says launchd refused the agent" "launchd didn't load the server's agent" "$err"
contains "refused: says the previous server is back" "The previous server is back up on http://127.0.0.1:18090." "$err"

setup refused-app app
run FAKE_REFUSE_VERSION=0.1.0-new
expect "refused, from an installed app: fails" 1 "$status"
expect "refused, from an installed app: the old server runs again" 0.1.0-old "$(loaded_version)"
expect "refused, from an installed app: the old bundle is back" "hub-server 0.1.0-old" "$("$installed/Contents/Helpers/bin/hub-server" --version | head -n 1)"
expect "refused, from an installed app: the hub answers after" '{"status":"ok"}' "$(health)"
expect "refused, from an installed app: no staged or replaced copy is left" "no no" "$(exists "$installed.installing") $(exists "$installed.replaced")"

setup silent app
run FAKE_DEAD_VERSION=0.1.0-new
expect "a new server that never answers: fails" 1 "$status"
expect "a new server that never answers: the old server runs again" 0.1.0-old "$(loaded_version)"
expect "a new server that never answers: the hub answers after" '{"status":"ok"}' "$(health 0.1.0-new)"
contains "a new server that never answers: says so" "The server didn't answer on http://127.0.0.1:18090" "$err"

setup wrong-version app
run FAKE_ANSWER_VERSION=0.0.9
expect "another version answering: fails" 1 "$status"
expect "another version answering: the old server runs again" 0.1.0-old "$(loaded_version)"
contains "another version answering: says so" "is version \"0.0.9\", not 0.1.0-new" "$err"

setup open-app app
echo 5 > "$fake/app-open"
run
expect "an open app that takes a few seconds to quit: installs" 0 "$status"
contains "an open app is asked to quit" 'tell application id "com.tonypine.JobSearchHub" to quit' "$(cat "$fake/osascript.log")"
expect "an open app is opened again" "$installed" "$(cat "$fake/open.log")"

setup slow-app app
echo 30 > "$fake/app-open"
run
expect "an app still quitting after 10 seconds: installs" 0 "$status"
contains "an app still quitting after 10 seconds: says what it may wait on" "If it shows a dialog" "$out"

setup stuck-app app
echo 1000 > "$fake/app-open"
run
expect "an app that never quits: fails" 1 "$status"
contains "an app that never quits: says nothing changed" "didn't quit within 90 seconds, so nothing was changed" "$err"
expect "an app that never quits: the server was never stopped" "" "$(grep bootout "$fake/launchctl.log" 2>/dev/null || true)"
expect "an app that never quits: the old server still runs" 0.1.0-old "$(loaded_version)"
expect "an app that never quits: the old plist stays" same "$(same "$case_dir/plist.before" "$plist")"
expect "an app that never quits: no staged copy is left" no "$(exists "$installed.installing")"

setup new-engine app
echo stale > "$engine/.sha256"
run
expect "a new engine: installs" 0 "$status"
expect "a new engine: is downloaded from the lock's URL" https://engines.invalid/postgres-18.tar.gz "$(cat "$fake/downloads.log")"
expect "a new engine: is in place" "$engine_sha256 yes" "$(cat "$engine/.sha256") $(exists "$engine/bin/postgres")"
expect "a new engine: no download or old engine is left" "no no" "$(exists "$engine.download") $(exists "$engine.old")"

setup refused-engine app
echo stale > "$engine/.sha256"
echo old > "$engine/marker"
run FAKE_REFUSE_VERSION=0.1.0-new
expect "refused, with a new engine: fails" 1 "$status"
expect "refused, with a new engine: the old server runs again" 0.1.0-old "$(loaded_version)"
expect "refused, with a new engine: the old engine is back" "stale old no" "$(cat "$engine/.sha256") $(cat "$engine/marker") $(exists "$engine/bin/postgres")"
expect "refused, with a new engine: no download or old engine is left" "no no" "$(exists "$engine.download") $(exists "$engine.old")"

setup tampered-engine app
echo stale > "$engine/.sha256"
echo tampered > "$case_dir/tampered.tar.gz"
run FAKE_ENGINE="$case_dir/tampered.tar.gz"
expect "an engine that doesn't match the lock: fails" 1 "$status"
contains "an engine that doesn't match the lock: says so" "doesn't match the SHA-256" "$err"
expect "an engine that doesn't match the lock: the server was never stopped" "" "$(grep bootout "$fake/launchctl.log" 2>/dev/null || true)"
expect "an engine that doesn't match the lock: no download is left" no "$(exists "$engine.download")"

setup other-program app
run FAKE_PRINT_PROGRAM=/usr/local/bin/hub-server
expect "launchd running another program: fails" 1 "$status"
contains "launchd running another program: says so" "launchd runs \"/usr/local/bin/hub-server\" as $label, not $installed/Contents/Helpers/bin/hub-server." "$err"
expect "launchd running another program: the old bundle is back" "hub-server 0.1.0-old" "$("$installed/Contents/Helpers/bin/hub-server" --version | head -n 1)"
expect "launchd running another program: the old plist is back" same "$(same "$case_dir/plist.before" "$plist")"

setup stuck-old app
run FAKE_STUCK_VERSION=0.1.0-old
expect "an old server that won't stop: fails" 1 "$status"
contains "an old server that won't stop: says so" "The server didn't stop; see" "$err"
expect "an old server that won't stop: it still runs" 0.1.0-old "$(loaded_version)"
expect "an old server that won't stop: the new app isn't installed" "hub-server 0.1.0-old" "$("$installed/Contents/Helpers/bin/hub-server" --version | head -n 1)"
expect "an old server that won't stop: the plist and server.env are as they were" "same same" "$(same "$case_dir/plist.before" "$plist") $(same "$case_dir/env.before" "$env_file")"

setup stuck-new app
echo stale > "$engine/.sha256"
run FAKE_DEAD_VERSION=0.1.0-new FAKE_STUCK_VERSION=0.1.0-new
kept_plist="$(echo "$err" | sed -n 's/^  the previous agent, \(.*\), as .*/\1/p')"
kept_env="$(echo "$err" | sed -n 's/^  the previous settings, \(.*\), as .*/\1/p')"
expect "a new server that won't stop: fails" 1 "$status"
contains "a new server that won't stop: says nothing was put back" "The server didn't stop, so nothing was put back" "$err"
contains "a new server that won't stop: says where the previous app is" "the previous app, $installed.replaced, as $installed" "$err"
contains "a new server that won't stop: says where the previous engine is" "the previous engine, $engine.old, as $engine" "$err"
expect "a new server that won't stop: the previous app and engine are kept" "hub-server 0.1.0-old stale" "$("$installed.replaced/Contents/Helpers/bin/hub-server" --version | head -n 1) $(cat "$engine.old/.sha256")"
expect "a new server that won't stop: the previous agent is kept" same "$(same "$case_dir/plist.before" "$kept_plist")"
expect "a new server that won't stop: the previous server.env is kept" same "$(same "$case_dir/env.before" "$kept_env")"

setup compose app
on_compose
run
expect "on Docker's Postgres: installs" 0 "$status"
expect "on Docker's Postgres: the data is imported once" 1 "$(lines "$fake/imports.log")"
expect "on Docker's Postgres: server.env loses the database URL and password, alone" "$(printf 'HUB_ADDR=127.0.0.1:18090\nHUB_OWNER_TOKEN=test-token')" "$(cat "$env_file")"
expect "on Docker's Postgres: the new server runs" 0.1.0-new "$(loaded_version)"
contains "on Docker's Postgres: says the hub runs the database the server owns" "runs on the database the server owns, Postgres 18" "$out"
cp "$env_file" "$case_dir/env.moved"
run
expect "a second install after the move: installs" 0 "$status"
expect "a second install after the move: imports nothing" 1 "$(lines "$fake/imports.log")"
expect "a second install after the move: server.env is as the move left it" same "$(same "$case_dir/env.moved" "$env_file")"

setup compose-move-fails app
on_compose
run FAKE_MOVE_FAILS=1
expect "a move that fails: the install goes on" 0 "$status"
expect "a move that fails: server.env is as it was" same "$(same "$case_dir/env.before" "$env_file")"
expect "a move that fails: the new server runs, on Docker's Postgres" 0.1.0-new "$(loaded_version)"
contains "a move that fails: says why" "row counts differ" "$err"
contains "a move that fails: says the hub stays on Docker's Postgres" "server.env is as it was and the server runs on Docker's Postgres as before" "$err"

setup compose-moved-dead app
on_compose
run FAKE_DEAD_VERSION=0.1.0-new
expect "moved, but the new server never answers: fails" 1 "$status"
expect "moved, but the new server never answers: server.env is as it was" same "$(same "$case_dir/env.before" "$env_file")"
expect "moved, but the new server never answers: the old server runs again" 0.1.0-old "$(loaded_version)"
contains "moved, but the new server never answers: says the hub is back on Docker's Postgres" "server.env points at Docker's Postgres again" "$err"

setup compose-no-postgres app
on_compose
run FAKE_NO_POSTGRES=1
expect "moved, but /v1/health doesn't report the owned database: fails" 1 "$status"
contains "moved, but /v1/health doesn't report the owned database: says so" "doesn't report the database it owns on /v1/health" "$err"
expect "moved, but /v1/health doesn't report the owned database: server.env is as it was" same "$(same "$case_dir/env.before" "$env_file")"
expect "moved, but /v1/health doesn't report the owned database: the old server runs again" 0.1.0-old "$(loaded_version)"

setup other-database app
printf 'HUB_DATABASE_URL=postgres://hub:0123abcd@db.example:5432/hub\n' >> "$env_file"
cp "$env_file" "$case_dir/env.before"
run
expect "another Postgres: installs" 0 "$status"
expect "another Postgres: the move is asked, and imports nothing" "1 0" "$(lines "$fake/moves.log") $(lines "$fake/imports.log")"
expect "another Postgres: server.env is as it was" same "$(same "$case_dir/env.before" "$env_file")"

[ "$failures" -eq 0 ] || {
  echo "$failures failed"
  exit 1
}
