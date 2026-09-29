# job-search-hub

A local hub for running a job search. Target companies go on a watch list, and agents fill in each company's dossier: its job board, and the people worth contacting. The hub tracks whether anyone at the company writes back, since contact is the first barrier to an offer.

It runs on one Mac: a Go server and Postgres in Docker Compose, agent sessions through Claude Code, and native clients to follow.

## What works today

- **The hub server** (`hub-server`) keeps companies, the watch list, job boards, people, agent runs, agent prompts and the owner's profile in Postgres. Every write is recorded in a change log with who made it: the owner, or one agent run.
- **MCP tools** at `/mcp` are the hub's interface for Claude Code and agents. Agents can only write through them, and some tools are the owner's alone: the watch list, prompts and the profile.
- **The `hub` CLI** lists the watch list, prints a dossier, and runs the company triage agent:

  ```
  hub company add wealthsimple.com
  ```

  The agent runs as an isolated `claude -p` session. It loads none of your user settings, plugins, skills or other MCP servers, and has only web search, web fetch and the hub's tools. The run is refused if its startup report shows anything else. It never fetches LinkedIn, and every person it stores needs a source URL.

## Setup

You need Docker, Go 1.26 and Claude Code, logged in with a Claude plan (agent runs use your subscription).

1. **Configure and start the stack.**

   ```bash
   cp .env.example .env    # then fill in both values; each line says how to generate it
   docker compose up -d --build
   curl -fsS localhost:8090/v1/health
   ```

   Both services listen on 127.0.0.1 only: the server on 8090, Postgres on 5434.

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

In Settings, enter the hub URL and the owner token. The token is kept in the Keychain. To set it without typing:

```bash
set -a && . ./.env && set +a
open macos/build/JobSearchHub.app --env HUB_OWNER_TOKEN="$HUB_OWNER_TOKEN" --args --import-owner-token
```

The build signs with an Apple Development certificate (`CODESIGN_IDENTITY` overrides which), so the Keychain keeps trusting the app across rebuilds.

## Prompts

Agent prompts are versioned records, not code. Read one with `get_agent_prompt` and save a new version with `update_agent_prompt`. Each run records the version it used. At the start of a run the server fills three placeholders: `{{company}}`, `{{owner_profile}}` and `{{company_dossier}}` (whatever the hub already stores about the company, fenced as data).

The prompts live only in the database; none are in this repo. A fresh database takes each prompt's first version from a private seed folder, `~/.config/job-search-hub/agent-prompts/` by default: `<kind>.md`, plus `<kind>.schema.json` when the prompt has a result schema. The server reads it at start for any kind that has no version yet. Tests use made-up prompts from `server/internal/testdatabase/agentprompts/`.

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
