# job-search-hub

A local hub for running a job search. Target companies go on a watch list, and agents fill in each company's dossier: its job board, and the people worth contacting. The hub tracks whether anyone at the company writes back, since contact is the first barrier to an offer.

It runs on one Mac: a Go server and Postgres in Docker Compose, agent sessions through Claude Code, and native clients to follow.

## What works today

- **The hub server** (`hub-server`) keeps companies, the watch list, job boards, people, agent runs, agent prompts and the owner's profile in Postgres. Every write is recorded in a change log with who made it: the owner, or one agent run.
- **MCP tools** at `/mcp` are the hub's interface for Claude Code and agents. Agents can only write through them, and some tools are the owner's alone: the watch list, prompts and the profile.
- **Follow-ups** fall due by pipeline phase, a week after applying by default. A cold message to a company, recorded with `record_outreach` or *Messaged someone…* on the company in the Mac app, puts its outreach card in Applied, so it falls due the same way. From 08:00 each follow-up due that day becomes an update on the Mac and the paired phones, once per due date.
- **Fresh matches** reach you while a posting is new. Boards are read every 15 minutes, and a job posted in the last three days whose brief judges it a strong match becomes an update on the Mac and the paired phones, once per job, saying how long ago it was posted. Matches found between 22:00 and 08:00 wait for the morning.
- **Company suggestions** on the Companies page list the companies you follow on LinkedIn, and the ones on [startups.gallery](https://startups.gallery)'s remote list whose board has a fitting job open. The server reads that list once a week, robots.txt first and with spaced requests that name the hub, and keeps only each company's name, site and careers link. *Research* starts Add company for a suggestion.
- **The `hub` CLI** lists the watch list, prints a dossier, and runs the company triage agent:

  ```
  hub company add wealthsimple.com
  ```

  The agent runs as an isolated `claude -p` session. It loads none of your user settings, plugins, skills or other MCP servers, and has only web search, web fetch and the hub's tools. The run is refused if its startup report shows anything else. It never fetches LinkedIn, and every person it stores needs a source URL.

## Setup

You need Docker, Go 1.26 and Claude Code, logged in with a Claude plan (agent runs use your subscription).

1. **Configure and start the stack.** Postgres runs in Docker; the server runs natively on the Mac as a LaunchAgent, so it can run local models on the GPU.

   ```bash
   cp .env.example .env    # then fill in both values; each line says how to generate it
   docker compose up -d                     # Postgres only
   server/scripts/install-native-server.sh  # builds hub-server, installs and starts the LaunchAgent
   curl -fsS localhost:8090/v1/health
   ```

   Both listen on 127.0.0.1 only: the server on 8090, Postgres on 5434. The first install writes the server's settings to `~/.config/job-search-hub/server.env` (chmod 600) from `.env`; later runs keep it. Logs go to `~/Library/Logs/JobSearchHub/server.log`. Each night from 03:00 the server dumps the database with `pg_dump` (`brew install libpq`) to `~/Library/Application Support/JobSearchHub/backups/hub-YYYY-MM-DD.dump` and keeps the newest 14; restore one with `pg_restore --clean --dbname=<url> <file>`. Run the script again after pulling changes, and `--uninstall` to remove the agent. On a host other than a Mac, run the server in Docker instead: `docker compose --profile docker-server up -d --build`.

2. **Install the CLI.**

   ```bash
   cd server && go install ./cmd/hub
   ```

   It lands in `$(go env GOPATH)/bin`, which needs to be on your `PATH`. The CLI reads `HUB_OWNER_TOKEN` (and optionally `HUB_URL`) from the environment, or from `~/.config/job-search-hub/config.json`:

   ```json
   { "url": "http://localhost:8090", "owner_token": "…" }
   ```

   Keep that file `chmod 600`; the CLI refuses it otherwise.

3. **Give Claude Code the hub's tools.**

   ```bash
   set -a && . ./.env && set +a
   claude mcp add --scope user --transport http job-search-hub http://localhost:8090/mcp \
     --header "Authorization: Bearer $HUB_OWNER_TOKEN"
   ```

   Any Claude Code session can then read the watch list and dossiers, and edit prompts and your profile, as the owner.

4. **Write your profile.** Agents use it as context for every run. Ask Claude Code to draft it from your resume and save it with `update_owner_profile`.

## The macOS app

```bash
cd macos && ./Scripts/make-app.sh      # builds and signs build/JobSearchHub.app
open build/JobSearchHub.app
```

In Settings (⌘,) › Connection, enter the hub URL and the owner token. The token is kept in the Keychain. To set it without typing:

```bash
set -a && . ./.env && set +a
open macos/build/JobSearchHub.app --env HUB_OWNER_TOKEN="$HUB_OWNER_TOKEN" --args --import-owner-token
```

The build signs with an Apple Development certificate (`CODESIGN_IDENTITY` overrides which), so the Keychain keeps trusting the app across rebuilds. Without one in the keychain, or with `CODESIGN_IDENTITY=-`, it signs ad hoc. An ad-hoc or self-signed build gets the Keychain's access prompt at launch: the window opens and says it's waiting for Keychain access until you answer, and denying leaves the app without a token.

The app works from the keyboard. **⌘K** (Go › Jump to…) finds a job, company, person or page, and runs the rare actions kept out of the toolbars: add a company or a job by URL, generate missing CVs, pause or resume the local models. A job, company or person opens in the inspector over the page you're on. In Decide, and in Today's Decide card, ↑↓ move through the jobs, Return opens one, and **P**, **L** and **S** pursue it, leave it for later or skip it, then bring up the next. ⌘N is the page's Add, and ⌘[ and ⌘] go back and forward in the inspector.

## Prompts

Agent prompts are versioned records, not code. Read one with `get_agent_prompt` and save a new version with `update_agent_prompt`. Each run records the version it used. At the start of a run the server fills three placeholders: `{{company}}`, `{{owner_profile}}` and `{{company_dossier}}` (whatever the hub already stores about the company, fenced as data).

The prompts live only in the database; none are in this repo. A fresh database takes each prompt's first version from a private seed folder, `~/.config/job-search-hub/agent-prompts/` by default: `<kind>.md`, plus `<kind>.schema.json` when the prompt has a result schema. The server reads it at start for any kind that has no version yet. Tests use made-up prompts from `server/internal/testdatabase/agentprompts/`.

## Android

`android/` holds the phone companion: updates and good-fit jobs, paired with the hub through a QR code from the Mac app's Settings › Phones. See `android/README.md`.

## Where it runs

The hub stays on the Mac rather than moving to an always-on PC (decided October 2026). The reasons:

- **It already runs all day.** With system sleep off, the server keeps running under launchd (`KeepAlive`) and Postgres restarts with Docker Desktop, so a locked screen doesn't take the hub down.
- **Downtime loses nothing.** Gmail catches up from the stored history ID, Pub/Sub holds notifications for 7 days, alert emails of the last 30 days are read on the next pass, and board and feed polls pick up where they stopped. Moving would only add gathering while the Mac is shut or away.
- **Moving costs more than that.** The server runs natively so it can use the Mac's GPU for local models. The phones reach it over HTTPS through `tailscale serve`. The Google sign-in and the Pub/Sub listener live here too. The nightly `pg_dump` (see Setup) runs only with the native server, because the Docker image has no `pg_dump`. All of that would have to be set up again on a Windows Docker host.

Revisit this if the PC turns out to run the local models well. If it does, move the model worker there first and leave the hub where it is.

## CI

`.github/workflows/ci.yml` runs on every pull request and push to `main`. Jobs that only matter for one part of the repo skip when a change doesn't touch it, and one job, `ci`, passes when nothing it needs failed. `ci` is the only check `main` requires, so a skipped job never leaves a PR waiting.

`main` is protected by `.github/rulesets/main.json`: changes land through a pull request with no required approvals, since Symphony opens PRs under the owner's account and the approval is moving the Linear ticket to `Merging`. `ci` from GitHub Actions has to pass, but the branch doesn't have to be up to date with `main`; the push run on `main` catches the rare conflict. Force pushes and deleting `main` are blocked, and nobody can bypass the rules, admins included: in an emergency, turn the ruleset off in Settings › Rules first.

The ruleset and these settings need a repository admin, once:

1. **Settings › Rules › Rulesets › New ruleset › Import a ruleset**, pick `.github/rulesets/main.json`, and create it.
2. **Settings › General › Pull Requests**: allow squash merging only, since Symphony squash-merges; turn on **Allow auto-merge**, so Symphony can land an approved PR when `ci` goes green; turn on **Automatically delete head branches**.

## Layout

```
compose.yaml              Postgres and hub-server
server/cmd/hub-server     the server
server/cmd/hub            the CLI
server/internal/…         store, MCP tools, REST API, prompts, stream parsing
server/agents/…           each agent's result contract
server/migrations         the schema, applied at server start
```

See `CLAUDE.md` for how to run the tests.
