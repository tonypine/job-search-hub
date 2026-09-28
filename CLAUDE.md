# Working in this repository

## The repo is meant to be public

Nothing personal goes in a commit: no tokens, no `.env`, no company or person data from a real search, no recruiter names. Real data lives in the Postgres volume, and notes about the search live outside the repo (see `CLAUDE.local.md` if it exists).

## Commits

`type: message`, e.g. `feat: store job boards for companies`. Types: `feat`, `fix`, `refactor`, `chore`, `test`, `docs`, `style`, `perf`. One small delivery per commit.

## Verification

Run these before calling a change done:

```bash
docker compose up -d db
set -a && . ./.env && set +a
export HUB_TEST_DATABASE_URL="postgres://hub:${HUB_DATABASE_PASSWORD}@localhost:5434/postgres"
cd server && gofmt -l . && go vet ./... && go test ./...
```

Store and tool tests each get a fresh database created through `HUB_TEST_DATABASE_URL` and dropped afterwards. Without the variable they fail rather than skip.
