#!/bin/zsh
# Installs hub-server as a macOS LaunchAgent, so it runs natively (and can use
# the GPU and local tools), with the Postgres engine pinned in
# server/postgres-engine.lock, which the server runs its database on.
#
#   server/scripts/install-native-server.sh             build, install and (re)start
#   server/scripts/install-native-server.sh --uninstall stop it and remove the agent
#
# Settings come from ~/.config/job-search-hub/server.env, which the first run
# writes from the repo's .env with native addresses and paths, and without
# HUB_DATABASE_URL, so the server owns its database. A server.env that sets
# HUB_DATABASE_URL keeps using that Postgres (Docker's, say). Secrets never
# enter the repo; the file is kept chmod 600.
set -euo pipefail

label=com.tonypine.jobsearchhub.server
repo=${0:A:h:h:h}
app_dir="$HOME/Library/Application Support/JobSearchHub"
config_dir="$HOME/.config/job-search-hub"
log_dir="$HOME/Library/Logs/JobSearchHub"
plist="$HOME/Library/LaunchAgents/$label.plist"
domain="gui/$(id -u)"

# stop_agent unloads the agent and waits until launchd has let it go, since
# bootstrapping it again before that fails.
stop_agent() {
  launchctl bootout "$domain/$label" 2>/dev/null || true
  for _ in {1..30}; do
    launchctl print "$domain/$label" >/dev/null 2>&1 || return 0
    sleep 1
  done
}

if [[ "${1:-}" == "--uninstall" ]]; then
  stop_agent
  rm -f "$plist"
  echo "Removed the $label LaunchAgent. The binary stays in $app_dir/bin."
  exit 0
fi

mkdir -p "$app_dir/bin" "$config_dir" "$log_dir"

# The engine is unpacked beside the installed one and swapped in once the
# server is stopped. Only a folder named postgres-<major> is an engine to the
# server, so neither a download nor the folder it replaces is one.
lock="$repo/server/postgres-engine.lock"
engine_version=$(sed -n 's/^version=//p' "$lock")
engine_url=$(sed -n 's/^url=//p' "$lock")
engine_sha256=$(sed -n 's/^sha256=//p' "$lock")
[[ -n "$engine_version" && -n "$engine_url" && -n "$engine_sha256" ]] || { echo "$lock doesn't pin an engine." >&2; exit 1; }
engine_name="postgres-${engine_version%%.*}"
engines_dir="$app_dir/engines"
engine_dir="$engines_dir/$engine_name"
engine_new=
if [[ "$(cat "$engine_dir/.sha256" 2>/dev/null)" != "$engine_sha256" ]]; then
  echo "==> Downloading Postgres $engine_version"
  mkdir -p "$engines_dir"
  engine_new="$engines_dir/$engine_name.download"
  rm -rf "$engine_new"
  mkdir "$engine_new"
  curl -fsSL --retry 3 -o "$engine_new/engine.tar.gz" "$engine_url"
  echo "$engine_sha256  $engine_new/engine.tar.gz" | shasum -a 256 -c - >/dev/null || {
    echo "The engine from $engine_url doesn't match the SHA-256 in $lock; not installing it." >&2
    rm -rf "$engine_new"
    exit 1
  }
  tar -xzf "$engine_new/engine.tar.gz" -C "$engine_new"
  [[ -x "$engine_new/$engine_name/bin/postgres" ]] || { echo "The engine has no $engine_name/bin/postgres." >&2; exit 1; }
  echo "$engine_sha256" > "$engine_new/$engine_name/.sha256"
  echo "Postgres $engine_version verified"
fi

echo "==> Building hub-server"
(cd "$repo/server" && go build -trimpath -o "$app_dir/bin/hub-server" ./cmd/hub-server)

echo "==> Building hub-cvprint, which the server prints CVs with"
(cd "$repo/macos" && swift build -c release --product hub-cvprint >/dev/null)
cp "$(cd "$repo/macos" && swift build -c release --product hub-cvprint --show-bin-path)/hub-cvprint" "$app_dir/bin/hub-cvprint"

env_file="$config_dir/server.env"
if [[ ! -f "$env_file" ]]; then
  echo "==> Writing $env_file from $repo/.env"
  [[ -f "$repo/.env" ]] || { echo "Missing $repo/.env; copy .env.example and fill it in first." >&2; exit 1; }
  set -a; source "$repo/.env"; set +a
  umask 077
  {
    echo "# hub-server's settings when it runs natively. Written by install-native-server.sh; edit freely."
    echo "# Without HUB_DATABASE_URL the server runs its own Postgres, in $app_dir/postgres."
    echo "HUB_ADDR=127.0.0.1:8090"
    echo "HUB_OWNER_TOKEN=${HUB_OWNER_TOKEN}"
    echo "HUB_PUBLIC_URL=${HUB_PUBLIC_URL:-http://localhost:8090}"
    model_url=${HUB_JOB_FACTS_MODEL_URL-http://localhost:1234/v1}
    echo "HUB_JOB_FACTS_MODEL_URL=${model_url//host.docker.internal/localhost}"
    echo "HUB_JOB_FACTS_MODEL=${HUB_JOB_FACTS_MODEL:-qwen/qwen3.5-9b}"
    echo "HUB_GOOGLE_OAUTH_CLIENT_FILE=$config_dir/google-oauth-client.json"
    echo "HUB_AGENT_PROMPTS_DIR=$config_dir/agent-prompts"
    echo "HUB_FIREBASE_SERVICE_ACCOUNT_FILE=$config_dir/firebase-service-account.json"
    for name in HUB_BOARD_POLL_INTERVAL HUB_FEED_POLL_INTERVAL HUB_JOB_FACTS_INTERVAL HUB_GMAIL_PUBSUB_TOPIC HUB_GMAIL_PUBSUB_SUBSCRIPTION \
        HUB_JSEARCH_API_KEY HUB_JSEARCH_URL HUB_JSEARCH_MONTHLY_REQUESTS; do
      if [[ -n "${(P)name:-}" ]]; then echo "$name=${(P)name}"; fi
    done
  } > "$env_file"
fi
chmod 600 "$env_file"

cat > "$app_dir/bin/run-hub-server" <<EOF
#!/bin/zsh
# Started by launchd ($label): loads the settings, then runs the server.
export PATH="/opt/homebrew/bin:/usr/local/bin:\$HOME/.local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
set -a; source "$env_file"; set +a
exec "$app_dir/bin/hub-server"
EOF
chmod 755 "$app_dir/bin/run-hub-server"

cat > "$plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>$label</string>
  <key>ProgramArguments</key><array><string>$app_dir/bin/run-hub-server</string></array>
  <key>WorkingDirectory</key><string>$app_dir</string>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>ExitTimeOut</key><integer>30</integer>
  <key>StandardOutPath</key><string>$log_dir/server.log</string>
  <key>StandardErrorPath</key><string>$log_dir/server.log</string>
</dict>
</plist>
EOF

echo "==> Starting $label"
stop_agent
if [[ -n "$engine_new" ]]; then
  rm -rf "$engine_dir.old"
  if [[ -d "$engine_dir" ]]; then mv "$engine_dir" "$engine_dir.old"; fi
  mv "$engine_new/$engine_name" "$engine_dir"
  rm -rf "$engine_dir.old" "$engine_new"
  echo "Installed Postgres $engine_version in $engine_dir"
fi
launchctl bootstrap "$domain" "$plist"
# A first start creates the database and migrates it.
for _ in {1..60}; do
  if curl -fsS http://localhost:8090/v1/health >/dev/null 2>&1; then
    echo "hub-server is up on 127.0.0.1:8090 (logs: $log_dir/server.log)"
    exit 0
  fi
  sleep 1
done
echo "hub-server didn't answer on 8090; see $log_dir/server.log" >&2
exit 1
