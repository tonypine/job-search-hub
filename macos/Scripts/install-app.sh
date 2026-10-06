#!/bin/bash
# Installs a build of the app's bundle as ~/Applications/Job Search Hub.app and
# runs the server from it.
#
#   macos/Scripts/install-app.sh                    build this checkout with make-app.sh, then install it
#   macos/Scripts/install-app.sh <JobSearchHub.app> install a bundle built already
#
# Until hub-update installs versions itself, this script does each step:
#
#   1. checks the bundle carries the server and is signed by the pinned team
#      (CODESIGN_TEAM_ID, or ~/.config/job-search-hub/codesign-team-id), whose
#      signature the Keychain trusts with the owner token;
#   2. installs the Postgres engine pinned in server/postgres-engine.lock in
#      ~/Library/Application Support/JobSearchHub/engines, where the server
#      finds it while the bundle doesn't carry one;
#   3. writes ~/.config/job-search-hub/server.env from the repository's .env on
#      a first install;
#   4. quits the app, stops the server, puts the bundle in place, loads the
#      server's agent from ~/Library/LaunchAgents, which runs the bundle's
#      hub-server, waits for that version to answer, and reopens the app if it
#      was open.
#
# The hub is never left down: until the new server answers, the previous app,
# engine, server.env and agent are kept, and any failure from the moment the
# server stops puts them back, starts the previous server, and exits non-zero.
#
# It also moves a Mac off the layout install-native-server.sh left: its
# binaries in bin/ go, and so does a HUB_CV_PRINT_BIN in server.env that points
# at them.
set -euo pipefail

repo="$(cd "$(dirname "$0")/../.." && pwd)"
label=com.tonypine.jobsearchhub.server
bundle_id=com.tonypine.JobSearchHub
app_support="$HOME/Library/Application Support/JobSearchHub"
config_dir="$HOME/.config/job-search-hub"
installed="$HOME/Applications/Job Search Hub.app"
staged="$installed.installing"
plist="$HOME/Library/LaunchAgents/$label.plist"
log_file="$HOME/Library/Logs/JobSearchHub/server.log"
domain="gui/$(id -u)"
# The app quits in seconds, unless it shows a dialog or macOS asks whether
# this terminal may control it.
quit_timeout=90

fail() {
  echo "$*" >&2
  exit 1
}

# Set once the server stops, until the new one answers: an exit in between
# puts the previous one back.
swapping=false
committed=false
app_swapped=false
engine_swapped=false
engine_new=
previous_plist=
work="$(mktemp -d "${TMPDIR:-/tmp}/install-app.XXXXXX")"

# On SIGTERM the server waits up to 30 seconds for the work still running, 5
# for open requests, then up to 30 for its Postgres to stop.
stop_agent() {
  launchctl bootout "$domain/$label" 2>/dev/null || true
  for _ in $(seq 80); do
    launchctl print "$domain/$label" >/dev/null 2>&1 || return 0
    sleep 1
  done
  return 1
}

# start_agent PLIST loads the agent, which starts the server. launchd now and
# then refuses a label it has only just let go, so it tries again.
start_agent() {
  for _ in 1 2 3; do
    launchctl bootstrap "$domain" "$1" && return 0
    sleep 2
  done
  return 1
}

# A first start creates the database and migrates it.
wait_for_health() {
  for _ in $(seq 120); do
    curl -fsS "$hub_url/v1/health" >/dev/null 2>&1 && return 0
    sleep 1
  done
  return 1
}

# put_back undoes a swap that failed: the previous app, engine, server.env and
# agent go back, and the previous server starts.
put_back() {
  set +e
  echo "==> Putting the previous server back" >&2
  if ! stop_agent; then
    echo "The server didn't stop, so it stays as it is; see $log_file." >&2
    return
  fi
  if [ "$app_swapped" = true ]; then rm -rf "$installed"; fi
  if [ -d "$installed.replaced" ]; then mv "$installed.replaced" "$installed"; fi
  if [ "$engine_swapped" = true ]; then
    rm -rf "$engine_dir"
    if [ -d "$engine_dir.old" ]; then mv "$engine_dir.old" "$engine_dir"; fi
  fi
  if [ -f "$work/previous.env" ]; then cp -p "$work/previous.env" "$env_file"; fi
  if [ -z "$previous_plist" ]; then
    rm -f "$plist"
    echo "There was no agent before this install to put back." >&2
    return
  fi
  cp -p "$previous_plist" "$plist"
  if start_agent "$plist" && wait_for_health; then
    echo "The previous server is back up on $hub_url." >&2
  else
    echo "The previous server didn't come back either; see $log_file, and start it with: launchctl bootstrap $domain $plist" >&2
  fi
}

on_exit() {
  local status=$?
  trap - EXIT INT TERM HUP
  if [ "$swapping" = true ] && [ "$committed" != true ]; then
    put_back
    [ "$status" -ne 0 ] || status=1
  fi
  rm -rf "$work" "$staged" ${engine_new:+"$engine_new"}
  exit "$status"
}
trap on_exit EXIT
trap 'exit 1' INT TERM HUP

case $# in
0)
  "$repo/macos/Scripts/make-app.sh"
  source_app="$repo/macos/build/JobSearchHub.app"
  ;;
1) source_app="${1%/}" ;;
*) fail "usage: $0 [<JobSearchHub.app>]" ;;
esac
[ -d "$source_app" ] || fail "No app bundle at $source_app."

echo "==> Checking $source_app"
server="$source_app/Contents/Helpers/bin/hub-server"
[ -x "$server" ] || fail "$source_app doesn't carry the server; make-app.sh builds it when Go is installed."
codesign --verify --strict --deep "$source_app"
team="$(codesign --display --verbose=2 "$source_app" 2>&1 | sed -n 's/^TeamIdentifier=//p')"
pinned="${CODESIGN_TEAM_ID:-}"
if [ -z "$pinned" ] && [ -f "$config_dir/codesign-team-id" ]; then
  pinned="$(tr -d '[:space:]' < "$config_dir/codesign-team-id")"
fi
if [ -z "$pinned" ] || [ "$team" != "$pinned" ]; then
  echo "$source_app is signed by team \"${team:-none}\", not the pinned team \"${pinned:-none pinned}\"." >&2
  echo "Build it with make-app.sh, which pins the team of the keychain's Apple Development identity, or pin yours with" >&2
  fail "CODESIGN_TEAM_ID: the Keychain trusts the app with the owner token by that team's signature."
fi
version="$("$server" --version | sed -n 's/^hub-server //p')"
[ -n "$version" ] || fail "$server --version doesn't name a version."
echo "hub-server $version, signed by team $team"

# The engine is unpacked beside the installed one and swapped in once the
# server is stopped. Only a folder named postgres-<major> is an engine to the
# server, so neither a download nor the folder it replaces is one.
lock="$repo/server/postgres-engine.lock"
engine_version="$(sed -n 's/^version=//p' "$lock")"
engine_url="$(sed -n 's/^url=//p' "$lock")"
engine_sha256="$(sed -n 's/^sha256=//p' "$lock")"
[ -n "$engine_version" ] && [ -n "$engine_url" ] && [ -n "$engine_sha256" ] || fail "$lock doesn't pin an engine."
engine_name="postgres-${engine_version%%.*}"
engines_dir="$app_support/engines"
engine_dir="$engines_dir/$engine_name"
if [ "$(cat "$engine_dir/.sha256" 2>/dev/null || true)" != "$engine_sha256" ]; then
  echo "==> Downloading Postgres $engine_version"
  mkdir -p "$engines_dir"
  engine_new="$engines_dir/$engine_name.download"
  rm -rf "$engine_new"
  mkdir "$engine_new"
  curl -fsSL --retry 3 -o "$engine_new/engine.tar.gz" "$engine_url"
  if ! echo "$engine_sha256  $engine_new/engine.tar.gz" | shasum -a 256 -c - >/dev/null; then
    fail "The engine from $engine_url doesn't match the SHA-256 in $lock; not installing it."
  fi
  tar -xzf "$engine_new/engine.tar.gz" -C "$engine_new"
  [ -x "$engine_new/$engine_name/bin/postgres" ] || fail "The engine has no $engine_name/bin/postgres."
  echo "$engine_sha256" > "$engine_new/$engine_name/.sha256"
  echo "Postgres $engine_version verified"
fi

# The server reads its settings from server.env, which every version shares.
mkdir -p "$config_dir"
env_file="$config_dir/server.env"
if [ ! -f "$env_file" ]; then
  echo "==> Writing $env_file from $repo/.env"
  [ -f "$repo/.env" ] || fail "Missing $repo/.env; copy .env.example and fill it in first."
  (
    set -a
    # shellcheck source=/dev/null
    source "$repo/.env"
    set +a
    [ -n "${HUB_OWNER_TOKEN:-}" ] || fail "Set HUB_OWNER_TOKEN in $repo/.env."
    umask 077
    # Written beside it and moved into place, so a run that fails partway
    # leaves no server.env for the next run to keep.
    written="$(mktemp "$env_file.XXXXXX")"
    trap 'rm -f "$written"' EXIT
    {
      echo "# hub-server's settings, which every installed version reads. Written by install-app.sh; edit freely."
      echo "# Without HUB_DATABASE_URL the server runs its own Postgres, in $app_support/postgres."
      echo "HUB_ADDR=127.0.0.1:8090"
      echo "HUB_OWNER_TOKEN=$HUB_OWNER_TOKEN"
      echo "HUB_PUBLIC_URL=${HUB_PUBLIC_URL:-http://localhost:8090}"
      model_url="${HUB_JOB_FACTS_MODEL_URL-http://localhost:1234/v1}"
      echo "HUB_JOB_FACTS_MODEL_URL=${model_url//host.docker.internal/localhost}"
      echo "HUB_JOB_FACTS_MODEL=${HUB_JOB_FACTS_MODEL:-qwen/qwen3.5-9b}"
      echo "HUB_GOOGLE_OAUTH_CLIENT_FILE=$config_dir/google-oauth-client.json"
      echo "HUB_AGENT_PROMPTS_DIR=$config_dir/agent-prompts"
      echo "HUB_FIREBASE_SERVICE_ACCOUNT_FILE=$config_dir/firebase-service-account.json"
      for name in HUB_BOARD_POLL_INTERVAL HUB_FEED_POLL_INTERVAL HUB_JOB_FACTS_INTERVAL HUB_GMAIL_PUBSUB_TOPIC HUB_GMAIL_PUBSUB_SUBSCRIPTION \
          HUB_JSEARCH_API_KEY HUB_JSEARCH_URL HUB_JSEARCH_MONTHLY_REQUESTS; do
        if [ -n "${!name:-}" ]; then echo "$name=${!name}"; fi
      done
    } > "$written"
    mv "$written" "$env_file"
  )
fi
chmod 600 "$env_file"
# The server listens on HUB_ADDR's port, which this Mac reaches on 127.0.0.1.
hub_port="$(sed -n 's/^HUB_ADDR=//p' "$env_file" | tail -n 1 | tr -d "\"'")"
hub_port="${hub_port##*:}"
hub_url="http://127.0.0.1:${hub_port:-8090}"

# The agent: a plist in ~/Library/LaunchAgents that runs the installed
# bundle's hub-server, so replacing the bundle replaces the server the next
# start runs. SMAppService refused to register the bundle's own plist on the
# owner's Mac, with EPERM (TP-718); launchctl bootstrap is what has run the
# server there all along. AssociatedBundleIdentifiers lists it under the app
# in System Settings › General › Login Items. It restarts after a crash, not
# after a clean exit, as when an update stops it to swap the bundle, and
# ExitTimeOut leaves room for the server's shutdown.
program="$installed/Contents/Helpers/bin/hub-server"
agent="$work/agent.plist"
plutil -create xml1 "$agent"
plutil -insert Label -string "$label" "$agent"
plutil -insert ProgramArguments -array "$agent"
plutil -insert ProgramArguments.0 -string "$program" "$agent"
plutil -insert AssociatedBundleIdentifiers -array "$agent"
plutil -insert AssociatedBundleIdentifiers.0 -string "$bundle_id" "$agent"
plutil -insert StandardOutPath -string "$log_file" "$agent"
plutil -insert StandardErrorPath -string "$log_file" "$agent"
plutil -insert RunAtLoad -bool true "$agent"
plutil -insert KeepAlive -dictionary "$agent"
plutil -insert KeepAlive.SuccessfulExit -bool false "$agent"
plutil -insert ExitTimeOut -integer 70 "$agent"
plutil -lint "$agent" >/dev/null

# Copied before anything stops, so the server is down only for the swap.
mkdir -p "$HOME/Applications"
rm -rf "$staged" "$installed.replaced"
ditto "$source_app" "$staged"

# Nothing has stopped yet, so an app that won't quit leaves the hub as it was.
# The quit is sent without waiting for the app's reply, which comes only once
# it has quit; the loop below does the waiting.
app_was_open=false
if pgrep -x JobSearchHub >/dev/null; then
  app_was_open=true
  echo "==> Quitting the app"
  if ! quit_error="$(osascript -e "ignoring application responses" -e "tell application id \"$bundle_id\" to quit" -e "end ignoring" 2>&1 >/dev/null)"; then
    echo "Couldn't ask the app to quit: $quit_error" >&2
  fi
  for second in $(seq "$quit_timeout"); do
    pgrep -x JobSearchHub >/dev/null || break
    if [ "$second" -eq 10 ]; then
      echo "Still waiting for the app to quit, for up to $quit_timeout seconds. If it shows a dialog, or macOS asks whether this terminal may control Job Search Hub, answer it."
    fi
    sleep 1
  done
  if pgrep -x JobSearchHub >/dev/null; then
    fail "The app didn't quit within $quit_timeout seconds, so nothing was changed; quit it and run this again."
  fi
fi

swapping=true
if [ -f "$plist" ]; then
  previous_plist="$work/previous.plist"
  cp -p "$plist" "$previous_plist"
fi
cp -p "$env_file" "$work/previous.env"

echo "==> Stopping the server"
stop_agent || fail "The server didn't stop; see $log_file."

if [ -n "$engine_new" ]; then
  rm -rf "$engine_dir.old"
  if [ -d "$engine_dir" ]; then mv "$engine_dir" "$engine_dir.old"; fi
  engine_swapped=true
  mv "$engine_new/$engine_name" "$engine_dir" || fail "Couldn't install the engine in $engine_dir."
  echo "Installed Postgres $engine_version in $engine_dir"
fi

# The server finds hub-cvprint beside itself now; a setting that points at
# the old bin/ would keep it on a binary this script removes.
old_cvprint='^(export )?HUB_CV_PRINT_BIN=.*JobSearchHub/bin/'
if grep -Eq "$old_cvprint" "$env_file"; then
  cleaned="$(mktemp "$env_file.XXXXXX")"
  grep -Ev "$old_cvprint" "$env_file" > "$cleaned" || true
  chmod 600 "$cleaned"
  mv "$cleaned" "$env_file"
  echo "Dropped the HUB_CV_PRINT_BIN that pointed at the old bin/ from $env_file"
fi

echo "==> Installing $installed"
if [ -d "$installed" ]; then mv "$installed" "$installed.replaced"; fi
mv "$staged" "$installed" || fail "Couldn't put the app in $installed."
app_swapped=true

echo "==> Starting the server from $installed"
mkdir -p "$(dirname "$plist")" "$(dirname "$log_file")"
cp "$agent" "$plist"
start_agent "$plist" || fail "launchd didn't load the server's agent from $plist."
wait_for_health || fail "The server didn't answer on $hub_url; see $log_file."
# Another server on the port, or launchd still running the old program,
# would answer the health check too.
answered="$(curl -fsS "$hub_url/v1/version" 2>/dev/null | sed -n 's/.*"version": *"\([^"]*\)".*/\1/p' || true)"
[ "$answered" = "$version" ] || fail "The server on $hub_url is version \"${answered:-unknown}\", not $version."
loaded="$(launchctl print "$domain/$label" 2>/dev/null | sed -n 's/^[[:space:]]*program = //p' | head -n 1 || true)"
[ "$loaded" = "$program" ] || fail "launchd runs \"${loaded:-nothing}\" as $label, not $program."
committed=true
echo "hub-server $version is up on $hub_url, from $program"

rm -rf "$installed.replaced"
if [ "$engine_swapped" = true ]; then rm -rf "$engine_dir.old"; fi
if [ -d "$app_support/bin" ]; then
  rm -rf "$app_support/bin"
  echo "Removed the old binaries, $app_support/bin"
fi

if [ "$app_was_open" = true ]; then
  open "$installed"
else
  echo "Open $installed to use it."
fi
