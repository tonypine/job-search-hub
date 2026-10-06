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
#   4. quits the app, stops the server, puts the bundle in place, registers the
#      server's agent from it, waits for the server to answer, and reopens the
#      app if it was open.
#
# It also moves a Mac off the layout install-native-server.sh left: its agent
# in ~/Library/LaunchAgents and its binaries in bin/ go, and so does a
# HUB_CV_PRINT_BIN in server.env that points at them.
set -euo pipefail

repo="$(cd "$(dirname "$0")/../.." && pwd)"
label=com.tonypine.jobsearchhub.server
bundle_id=com.tonypine.JobSearchHub
app_support="$HOME/Library/Application Support/JobSearchHub"
config_dir="$HOME/.config/job-search-hub"
installed="$HOME/Applications/Job Search Hub.app"
domain="gui/$(id -u)"

fail() {
  echo "$*" >&2
  exit 1
}

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
engine_new=
if [ "$(cat "$engine_dir/.sha256" 2>/dev/null || true)" != "$engine_sha256" ]; then
  echo "==> Downloading Postgres $engine_version"
  mkdir -p "$engines_dir"
  engine_new="$engines_dir/$engine_name.download"
  rm -rf "$engine_new"
  mkdir "$engine_new"
  curl -fsSL --retry 3 -o "$engine_new/engine.tar.gz" "$engine_url"
  if ! echo "$engine_sha256  $engine_new/engine.tar.gz" | shasum -a 256 -c - >/dev/null; then
    rm -rf "$engine_new"
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
# The server finds hub-cvprint beside itself now; a setting that points at
# the old bin/ would keep it on a binary this script removes.
old_cvprint='^(export )?HUB_CV_PRINT_BIN=.*JobSearchHub/bin/'
if grep -Eq "$old_cvprint" "$env_file"; then
  cleaned="$(mktemp "$env_file.XXXXXX")"
  grep -Ev "$old_cvprint" "$env_file" > "$cleaned" || true
  mv "$cleaned" "$env_file"
  echo "Dropped the HUB_CV_PRINT_BIN that pointed at the old bin/ from $env_file"
fi
chmod 600 "$env_file"
# The server listens on HUB_ADDR's port, which this Mac reaches on 127.0.0.1.
hub_port="$(sed -n 's/^HUB_ADDR=//p' "$env_file" | tail -n 1 | tr -d "\"'")"
hub_port="${hub_port##*:}"
hub_url="http://127.0.0.1:${hub_port:-8090}"

# Copied before anything stops, so the server is down only for the swap.
mkdir -p "$HOME/Applications"
staged="$installed.installing"
rm -rf "$staged" "$installed.replaced"
ditto "$source_app" "$staged"

app_was_open=false
if pgrep -x JobSearchHub >/dev/null; then
  app_was_open=true
  echo "==> Quitting the app"
  osascript -e "tell application id \"$bundle_id\" to quit" >/dev/null 2>&1 || true
  for _ in $(seq 30); do
    pgrep -x JobSearchHub >/dev/null || break
    sleep 1
  done
  if pgrep -x JobSearchHub >/dev/null; then
    fail "The app is still open; quit it and run this again."
  fi
fi

# On SIGTERM the server waits up to 30 seconds for the work still running, 5
# for open requests, then up to 30 for its Postgres to stop.
echo "==> Stopping the server"
launchctl bootout "$domain/$label" 2>/dev/null || true
for _ in $(seq 80); do
  launchctl print "$domain/$label" >/dev/null 2>&1 || break
  sleep 1
done
if launchctl print "$domain/$label" >/dev/null 2>&1; then
  fail "The server didn't stop; see ~/Library/Logs/JobSearchHub/server.log."
fi
old_plist="$HOME/Library/LaunchAgents/$label.plist"
if [ -f "$old_plist" ]; then
  rm -f "$old_plist"
  echo "Removed the old agent, $old_plist"
fi

if [ -n "$engine_new" ]; then
  # A swap that fails or is interrupted puts the old engine back: the
  # server ignores postgres-18.old, so the next start would find none.
  restore_engine() {
    if [ ! -d "$engine_dir" ] && [ -d "$engine_dir.old" ]; then mv "$engine_dir.old" "$engine_dir"; fi
  }
  trap 'restore_engine; exit 1' INT TERM HUP
  rm -rf "$engine_dir.old"
  if [ -d "$engine_dir" ]; then mv "$engine_dir" "$engine_dir.old"; fi
  if ! mv "$engine_new/$engine_name" "$engine_dir"; then
    restore_engine
    fail "Couldn't install the engine in $engine_dir."
  fi
  trap - INT TERM HUP
  rm -rf "$engine_dir.old" "$engine_new"
  echo "Installed Postgres $engine_version in $engine_dir"
fi

echo "==> Installing $installed"
if [ -d "$installed" ]; then mv "$installed" "$installed.replaced"; fi
if ! mv "$staged" "$installed"; then
  if [ -d "$installed.replaced" ]; then mv "$installed.replaced" "$installed"; fi
  fail "Couldn't put the app in $installed."
fi
rm -rf "$installed.replaced"

# The app registers the agent with SMAppService, from the plist in its
# bundle; launchd then runs the server inside it.
echo "==> Starting the server from $installed"
if ! "$installed/Contents/MacOS/JobSearchHub" --register-server; then
  fail "Couldn't register the server's agent. Open $installed and start the server in Settings › Server."
fi
# A first start creates the database and migrates it.
for _ in $(seq 120); do
  if curl -fsS "$hub_url/v1/health" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done
if ! curl -fsS "$hub_url/v1/health" >/dev/null 2>&1; then
  fail "The server didn't answer on $hub_url; see ~/Library/Logs/JobSearchHub/server.log."
fi
echo "hub-server $version is up on $hub_url, from $installed/Contents/Helpers/bin/hub-server"

if [ -d "$app_support/bin" ]; then
  rm -rf "$app_support/bin"
  echo "Removed the old binaries, $app_support/bin"
fi

if [ "$app_was_open" = true ]; then
  open "$installed"
else
  echo "Open $installed to use it."
fi
