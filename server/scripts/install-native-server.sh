#!/bin/zsh
# Installs hub-server as a macOS LaunchAgent, so it runs natively (and can use
# the GPU and local tools) while Postgres stays in Docker.
#
#   server/scripts/install-native-server.sh             build, install and (re)start
#   server/scripts/install-native-server.sh --uninstall stop it and remove the agent
#
# Settings come from ~/.config/job-search-hub/server.env, which the first run
# writes from the repo's .env with native addresses and paths. Secrets never
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
    echo "HUB_ADDR=127.0.0.1:8090"
    echo "HUB_DATABASE_URL=postgres://hub:${HUB_DATABASE_PASSWORD}@localhost:5434/hub"
    echo "HUB_OWNER_TOKEN=${HUB_OWNER_TOKEN}"
    echo "HUB_PUBLIC_URL=${HUB_PUBLIC_URL:-http://localhost:8090}"
    model_url=${HUB_JOB_FACTS_MODEL_URL-http://localhost:1234/v1}
    echo "HUB_JOB_FACTS_MODEL_URL=${model_url//host.docker.internal/localhost}"
    echo "HUB_JOB_FACTS_MODEL=${HUB_JOB_FACTS_MODEL:-qwen/qwen3.5-9b}"
    echo "HUB_GOOGLE_OAUTH_CLIENT_FILE=$config_dir/google-oauth-client.json"
    echo "HUB_AGENT_PROMPTS_DIR=$config_dir/agent-prompts"
    echo "HUB_FIREBASE_SERVICE_ACCOUNT_FILE=$config_dir/firebase-service-account.json"
    for name in HUB_BOARD_POLL_INTERVAL HUB_FEED_POLL_INTERVAL HUB_JOB_FACTS_INTERVAL HUB_GMAIL_PUBSUB_TOPIC HUB_GMAIL_PUBSUB_SUBSCRIPTION; do
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
  <key>StandardOutPath</key><string>$log_dir/server.log</string>
  <key>StandardErrorPath</key><string>$log_dir/server.log</string>
</dict>
</plist>
EOF

echo "==> Starting $label"
stop_agent
launchctl bootstrap "$domain" "$plist"
for _ in {1..30}; do
  if curl -fsS http://localhost:8090/v1/health >/dev/null 2>&1; then
    echo "hub-server is up on 127.0.0.1:8090 (logs: $log_dir/server.log)"
    exit 0
  fi
  sleep 1
done
echo "hub-server didn't answer on 8090; see $log_dir/server.log" >&2
exit 1
