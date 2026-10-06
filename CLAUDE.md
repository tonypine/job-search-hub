# Working in this repository

## The repo is meant to be public

Nothing personal goes in a commit: no tokens, no `.env`, no company or person data from a real search, no recruiter names. Real data lives in the Postgres volume, and notes about the search live outside the repo (see `CLAUDE.local.md` if it exists).

## Commits

`type: message`, e.g. `feat: store job boards for companies`. Types: `feat`, `fix`, `refactor`, `chore`, `test`, `docs`, `style`, `perf`. One small delivery per commit.

## Verification

Checks are split by cost. The cheap ones run locally before a push; the full suite runs in CI.

### Before a push

Run only the checks for what the change touches:

```bash
cd server
gofmt -l .                          # must print nothing
go vet ./...
go test ./internal/<package> ...    # only the packages the change touches
```

Store and tool tests (the packages whose tests use `internal/testdatabase`) each get a fresh database created through `HUB_TEST_DATABASE_URL` and dropped afterwards. Without the variable they fail rather than skip. Agent runs don't get `.env`, so they leave those packages to CI's `server-test` job. In an agent's sandbox, point the build cache at `/tmp` first: `export GOCACHE=/private/tmp/job-search-hub-go-build`.

For `macos/`, agent runs don't run `swift build` or `swift test`: the sandbox blocks SwiftPM. CI's `macos` job builds and tests it.

For `android/`, run `cd android && ./gradlew :core:test` when `android/core` changed, and leave the rest to CI's `android` job.

Don't run `go test ./...` or `swift test` as a routine pre-push step.

Turn on the versioned git hooks once per clone:

```bash
git config core.hooksPath .githooks
```

`.githooks/pre-push` then runs gofmt on the changed `.go` files and `go vet ./...` in `server/` before every push that changes Go, in a few seconds and without tests. Never push with `git push --no-verify`. When the hook fails, fix what it reports, commit, and push again.

### The full gate

CI (`.github/workflows/ci.yml`) runs on every pull request and reports one check, `ci`, which `main` requires. Its jobs are `server-static` (gofmt, go vet, staticcheck, go mod tidy), `server-test` (the whole Go suite against Postgres 18), `macos` (`swift build` and `swift test`) and `android` (`:core:test :data:testDebugUnitTest :app:lintDebug :app:assembleDebug`). Jobs for parts of the repo a PR doesn't touch skip.

Running the full suite locally is optional, for a change to shared infrastructure (migrations, `internal/testdatabase`, the store, the build) where waiting on CI is slow. `server/scripts/test-with-postgres.sh` runs `go test` against a throwaway Postgres it creates from the installed hub's engine (`~/Library/Application Support/JobSearchHub/engines/postgres-18`, or `HUB_POSTGRES_ENGINES`) in a temporary folder, and deletes it on exit, Ctrl-C included. No Docker, no `.env`:

```bash
server/scripts/test-with-postgres.sh ./...   # any go test arguments, as in server/
cd server
go run honnef.co/go/tools/cmd/staticcheck@v0.8.1 ./...   # the version CI pins in .github/workflows/ci.yml
cd ../macos && swift build && swift test
```

## Running the apps

These are for a person at the Mac. Agent runs never launch the Mac app, `screenshot-page.sh` or an Android emulator on the host: a GUI app an agent starts lands on the operator's desktop, and a crash shows a "quit unexpectedly" dialog there. Symphony's QA pass runs the apps in its own VM and emulator. An agent that needs a screenshot renders the view offscreen with SwiftUI's `ImageRenderer`, or leaves it to the QA pass.

```bash
cd macos
./Scripts/make-app.sh                            # builds and signs build/JobSearchHub.app, server included
./Scripts/install-app.sh                         # builds it and installs it in ~/Applications, then restarts the server from it
./Scripts/screenshot-page.sh companies out.png   # opens build/JobSearchHub.app on a page and captures its window
```
