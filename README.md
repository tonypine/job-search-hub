# job-search-hub

A local hub for running a job search. Target companies go on a watch list, and agents fill in each company's dossier: its job board, and the people worth contacting. The hub tracks whether anyone at the company writes back, since contact is the first barrier to an offer.

It runs on one Mac: a Go server that runs its own Postgres, agent sessions through Claude Code, and native clients for the Mac and Android.

## What works today

- **The hub server** (`hub-server`) keeps companies, the watch list, job boards, people, agent runs, agent prompts and the owner's profile in Postgres. Every write is recorded in a change log with who made it: the owner, or one agent run.
- **MCP tools** at `/mcp` are the hub's interface for Claude Code and agents. Agents can only write through them, and some tools are the owner's alone: the watch list, prompts and the profile.
- **Follow-ups** fall due by pipeline phase, a week after applying by default. A cold message to a company, recorded with `record_outreach` or *Messaged someone…* on the company in the Mac app, puts its outreach card in Applied, so it falls due the same way. A mail you send that starts a thread with someone at a company in the hub, which has no open card, counts as one by itself, and an update says so. From 08:00 each follow-up due that day becomes an update on the Mac and the paired phones, once per due date. On the Mac's Pipeline page, a card that went out and nobody answered by its follow-up offers a second route: *Write to …* someone the company's dossier lists or who can introduce you, which drafts the message with the outreach prompt in the card's session for you to send, or *Find people*, which researches the company again when nobody is on file. *Followed up…* then restarts the count.
- **Fresh matches** reach you while a posting is new. Boards are read every 15 minutes, and a job posted in the last three days whose brief judges it a strong match becomes an update on the Mac and the paired phones, once per job, saying how long ago it was posted. Matches found between 22:00 and 08:00 wait for the morning.
- **Company suggestions** on the Companies page list the companies you follow on LinkedIn, and the ones on [startups.gallery](https://startups.gallery)'s remote list whose board has a fitting job open. The server reads that list once a week, robots.txt first and with spaced requests that name the hub, and keeps only each company's name, site and careers link. *Research* starts Add company for a suggestion.
- **The `hub` CLI** lists the watch list, prints a dossier, and runs the company triage agent:

  ```
  hub company add wealthsimple.com
  ```

  The agent runs as an isolated `claude -p` session. It loads none of your user settings, plugins, skills or other MCP servers, and has only web search, web fetch and the hub's tools. The run is refused if its startup report shows anything else. It never fetches LinkedIn, and every person it stores needs a source URL.

## Setup

You need Go 1.26 and Claude Code, logged in with a Claude plan (agent runs use your subscription).

1. **Configure and install the hub.** The server ships inside the Mac app's bundle and runs natively as a LaunchAgent, so it can run local models on the GPU, and runs its own Postgres 18 beside it: nothing else to install or keep running.

   ```bash
   cp .env.example .env    # then fill in HUB_OWNER_TOKEN; the line says how to generate it
   macos/Scripts/install-app.sh   # builds the app with the server, installs it in ~/Applications and starts the server
   curl -fsS localhost:8090/v1/health
   ```

   The script builds the bundle with `macos/Scripts/make-app.sh` (see [The macOS app](#the-macos-app)) and installs it as `~/Applications/Job Search Hub.app`. It writes the server's LaunchAgent to `~/Library/LaunchAgents/com.tonypine.jobsearchhub.server.plist` and loads it with `launchctl bootstrap`, so launchd runs `Contents/Helpers/bin/hub-server` from the installed app, starts it at login, and restarts it if it crashes; System Settings › General › Login Items lists it under Job Search Hub. The script checks that the new version answers on `/v1/health` and `/v1/version`. Until it does, it keeps the previous app, engine, `server.env` and agent, and on any failure it puts them back, starts the previous server and exits non-zero, so a failed install never leaves the hub down. It waits up to 90 seconds for an open app to quit, and changes nothing if it doesn't. Settings › Server in the app starts, stops and restarts the server.

   The script also downloads the Postgres engine pinned in `server/postgres-engine.lock`, refuses it unless its SHA-256 matches, and installs it in `~/Library/Application Support/JobSearchHub/engines/postgres-18/`. The first install writes the server's settings to `~/.config/job-search-hub/server.env` (chmod 600) from `.env`; later runs keep it. The server reads that file itself at start (a variable already in its environment wins), puts Homebrew's and `~/.local/bin` on its `PATH`, and prints CVs with the `hub-cvprint` beside it in the bundle (`HUB_CV_PRINT_BIN` overrides). Logs go to `~/Library/Logs/JobSearchHub/server.log`. Run the script again after pulling changes. To remove the hub, stop the server in Settings › Server, delete `~/Library/LaunchAgents/com.tonypine.jobsearchhub.server.plist`, and delete the app.

   On a Mac set up with the old `server/scripts/install-native-server.sh`, the first `install-app.sh` moves it over: it points that script's agent in `~/Library/LaunchAgents` at the installed app, and removes its binaries in `~/Library/Application Support/JobSearchHub/bin/`, and a `HUB_CV_PRINT_BIN` in `server.env` that points at them.

   The server listens on 127.0.0.1:8090 only. Which database it uses depends on `HUB_DATABASE_URL` in `server.env`:

   - **Unset (a fresh install): the server owns its database.** At start it runs Postgres from the newest engine in the bundle's `Contents/Helpers/engines/` once the bundle carries one, and in `~/Library/Application Support/JobSearchHub/engines/` until then (`HUB_POSTGRES_ENGINES` overrides the folder), on a cluster in `~/Library/Application Support/JobSearchHub/postgres/18/` (`HUB_POSTGRES_DIR` overrides it), creating it on the first start, then migrates it. Postgres listens on no port, only on a Unix socket in that folder, which only your user can reach, and it starts and stops with the server: Settings › Server's Stop in the Mac app stops both. The cluster is left out of Time Machine; the nightly dumps are the backup. If Postgres stops by itself, the server exits, and launchd restarts both. When an update brings a new Postgres major, the server moves the database to it at start (see [Moving to a new Postgres major](#moving-to-a-new-postgres-major)).
   - **Set: the server uses that Postgres**, and waits up to three minutes for it at start. This is for a server on a host other than a Mac, which runs against any Postgres 18, a managed one included.

   Each night from 03:00 the server dumps the database with `pg_dump` to `~/Library/Application Support/JobSearchHub/backups/hub-YYYY-MM-DD.dump` and keeps the newest 14: the engine's own `pg_dump` when the server owns the database, `pg_dump` from the `PATH` otherwise, or the one `HUB_PG_DUMP` names. When the server owns the database, it also dumps it before applying new migrations, to `backups/hub-pre-migration-<version>.dump`, where `<version>` is the migration the dump holds, and keeps the newest 5. A start with no migration to apply takes no dump.

   To restore a dump into the database the server owns, stop the hub in Settings › Server, then run:

   ```bash
   ~/Applications/Job\ Search\ Hub.app/Contents/Helpers/bin/hub-server database restore ~/Library/Application\ Support/JobSearchHub/backups/hub-YYYY-MM-DD.dump
   ```

   It refuses while the hub runs. It restores the dump into a new cluster beside the current one (`postgres/18.partial`), migrates it, and only then swaps it in. The old cluster is kept as `postgres/18.replaced-<date>/`, and the command prints where; delete that folder once the hub runs well on the restored data. A restore that fails leaves the current database as it was. If a restore is killed between moving the old cluster aside and moving the new one in, the server refuses to start and names the `18.replaced-<date>` folder to rename back to `18`. For a database the server doesn't own, restore with `pg_restore --clean --dbname=<url> <file>`.

   **A hub still on the old Docker database moves itself.** A `server.env` from before the server owned its database points `HUB_DATABASE_URL` at the Postgres the old Compose file ran on `localhost:5434`. The next `install-app.sh` moves it, with nothing for you to do: while the server is stopped for the update, the new server's `hub-server database move-from-compose` imports that database as `hub-server database import` does (below), and only once each table holds as many rows as in the old one removes `HUB_DATABASE_URL` and `HUB_DATABASE_PASSWORD` from `server.env`. The script then checks that `/v1/health` reports the database the server owns (`"postgres":{"major":18}`). If any step fails, `server.env` is put back as it was, the hub keeps running on the old database, and the script says why. The old container and its `hub-db` volume are left as they are. A `HUB_DATABASE_URL` that names any other Postgres is never moved.

   To import a hub database from another Postgres by hand, stop the hub in Settings › Server, then run:

   ```bash
   ~/Applications/Job\ Search\ Hub.app/Contents/Helpers/bin/hub-server database import "postgres://<user>:<password>@<host>:<port>/<database>"
   ```

   It dumps it with the engine's `pg_dump` to `backups/hub-import-YYYY-MM-DD.dump`, which it keeps, restores the dump into a new cluster as a restore does, and prints each table's row count in both databases. It fails, leaving the database the server owns as it was, if any count differs. It refuses when the server owns a database that already holds data, from an earlier import or a start without `HUB_DATABASE_URL`; `--replace` replaces it, keeping it as `postgres/18.replaced-<date>/`. Then remove `HUB_DATABASE_URL` from `server.env`, if it's there, and start the hub.

2. **Install the CLI.** In the Mac app's Settings › Server, click *Install* on the *hub command* row. It links `~/.local/bin/hub` to the copy in the installed app's bundle, so `hub` in a terminal is always the installed version (`hub --version` prints it); `~/.local/bin` needs to be on your `PATH`. Without the app, `cd server && go install ./cmd/hub` puts one in `$(go env GOPATH)/bin`. The CLI reads `HUB_OWNER_TOKEN` (and optionally `HUB_URL`) from the environment, or from `~/.config/job-search-hub/config.json`:

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
cd macos && ./Scripts/make-app.sh      # builds and signs build/JobSearchHub.app, the whole bundle
./Scripts/install-app.sh               # builds it and installs it in ~/Applications, as in Setup
open ~/Applications/Job\ Search\ Hub.app
```

`make-app.sh` builds one bundle per version: the app, and in `Contents/Helpers/bin/` the server (`hub-server`), the CV printer (`hub-cvprint`), the `hub` command and the installer (`hub-update`, which only reports its version for now), for Apple silicon, with the server's LaunchAgent in `Contents/Library/LaunchAgents/`. Without Go, as in Symphony's QA VM, it builds the app alone, and Settings › Server says the build has no server. A build in `macos/build/` runs too, for trying a change: it leaves the installed app's server alone, and `Scripts/screenshot-page.sh` captures its pages without touching the installed app.

In Settings (⌘,) › Connection, enter the hub URL and the owner token. The token is kept in the Keychain. When the Keychain refuses it, as in a VM with no usable login keychain, the app still uses it until it quits, and Connection says why the Keychain didn't keep it. To set it without typing:

```bash
set -a && . ./.env && set +a
open ~/Applications/Job\ Search\ Hub.app --env HUB_OWNER_TOKEN="$HUB_OWNER_TOKEN" --args --import-owner-token
```

On a QA machine whose Keychain won't keep the token, launch the build in QA mode instead. It then takes the token from `HUB_OWNER_TOKEN` and leaves the Keychain alone; without `--qa-mode` the app ignores the variable:

```bash
open macos/build/JobSearchHub.app --env HUB_OWNER_TOKEN="$HUB_OWNER_TOKEN" --args --qa-mode
```

The build signs the bundle and every command in it with the Apple Development identity of a pinned team: `CODESIGN_TEAM_ID`, or the team ID in `~/.config/job-search-hub/codesign-team-id` (the certificate's Organizational Unit in Keychain Access). With nothing pinned and one Apple Development identity in the keychain, the build signs with it and writes its team to that file, so a work certificate added later is never picked. With two or more and nothing pinned, it lists them with their teams and stops, rather than guess. It refuses any other team's identity; `CODESIGN_IDENTITY` only narrows the choice among the team's. The Keychain then keeps trusting the app across rebuilds. With no Apple Development identity at all, or with `CODESIGN_IDENTITY=-`, it signs ad hoc, as CI does, and `install-app.sh` refuses to install that build. An ad-hoc or self-signed build gets the Keychain's access prompt at launch: the window opens and says it's waiting for Keychain access until you answer, and denying leaves the app without a token.

The app works from the keyboard. **⌘K** (Go › Jump to…) finds a job, company, person or page, and runs the rare actions kept out of the toolbars: add a company or a job by URL, generate missing CVs, pause or resume the local models. A job, company or person opens in the inspector over the page you're on. In Decide, and in Today's Decide card, ↑↓ move through the jobs, Return opens one, and **P**, **L** and **S** pursue it, leave it for later or skip it, then bring up the next. ⌘N is the page's Add, and ⌘[ and ⌘] go back and forward in the inspector.

## Prompts

Agent prompts are versioned records, not code. Read one with `get_agent_prompt` and save a new version with `update_agent_prompt`. Each run records the version it used. At the start of a run the server fills three placeholders: `{{company}}`, `{{owner_profile}}` and `{{company_dossier}}` (whatever the hub already stores about the company, fenced as data).

The prompts live only in the database; none are in this repo. A fresh database takes each prompt's first version from a private seed folder, `~/.config/job-search-hub/agent-prompts/` by default: `<kind>.md`, plus `<kind>.schema.json` when the prompt has a result schema. The server reads it at start for any kind that has no version yet. Tests use made-up prompts from `server/internal/testdatabase/agentprompts/`.

## Android

`android/` holds the phone companion: updates and good-fit jobs, paired with the hub through a QR code from the Mac app's Settings › Phones. See `android/README.md`.

## Releases

Once `ci` passes on a merge to `main`, `.github/workflows/release.yml` releases each app the merge changed, as a GitHub Release, both numbered `0.1.<N>`, where `N` is the commit count on `main`:

| The merge changed | Release |
| --- | --- |
| `server/` or `macos/` | `mac-v0.1.<N>`, "Job Search Hub 0.1.<N> for Mac": the signed app, server inside, as `Job-Search-Hub-0.1.<N>.zip`, and its `Job-Search-Hub-0.1.<N>.zip.sha256` |
| `android/` | `android-v0.1.<N>`: the signed `job-search-hub-0.1.<N>.apk`, and its `job-search-hub-0.1.<N>.apk.sha256` |
| both | both, with the same `N` |
| only docs, CI or scripts | nothing |

A release tags the commit it built, and GitHub lets the workflow's token tag a commit only while its `.github/workflows/` matches `main`'s tip: anything else counts as creating workflows, which takes a permission `GITHUB_TOKEN` can't have, and the release fails with `HTTP 403: Resource not accessible by integration`. So when a later merge changed `.github/workflows/` before a commit's release ran, that commit isn't released, the run says why, and the next release from `main` carries its changes. To release `main`'s tip by hand, use Actions › release › Run workflow with the tags left empty; an app that has never been released gets its first release that way.

Each release's notes list the commits since that app's previous release that touch its paths, under New, Fixed and Other changes, each line tagged with the parts of the hub its commit touched (`Mac`, `Server`, `Phone`) and ending with its pull request. The Mac app reads that format for *What's new*, so `scripts/release/changelog_test.sh` pins it. `docs/design/updates.md` › Releases from CI and `docs/decisions/0001-android-release-distribution.md` say why it works this way.

Before it publishes the Mac app, `scripts/release/verify-app.sh` refuses a bundle that fails `codesign --verify --strict --deep`, holds an executable signed ad hoc, by another team than `MAC_SIGNING_TEAM_ID` or without the hardened runtime, carries another version in its `Info.plist` or `hub-server --version`, or links a library outside the system and the bundle. The copy unzipped from the zip is checked again. The release isn't notarized: Apple doesn't notarize with a development certificate, so the first install from a browser asks to allow it once in System Settings › Privacy & Security.

To install a phone release the first time, open its page on the phone, download the APK and open it; Android asks to allow installs from the browser. From then on the app finds its own new versions and installs them (see `android/README.md` › New versions). Each release installs over the previous one and keeps the pairing.

The Mac app looks for its own releases: at launch and every hour it asks GitHub's API for the `mac-v*` releases, without a token and without the server, skipping drafts and pre-releases. It downloads the newest one above its own version into `~/Library/Application Support/JobSearchHub/Updates/<version>/` and checks it, in order: the zip's SHA-256 against its `.sha256`, `codesign --verify --strict --deep`, the running app's team ID and designated requirement, the version in its `Info.plist` and `hub-server --version`, and whether its newest migration is ahead of the running server's. Only a version that passes shows, as *New version 0.1.<N>* at the foot of the sidebar; one that fails is deleted and named in Settings › Version, along with what's new across every release since the running one. *Job Search Hub › Check for New Version…* checks right away. Marking a release as a pre-release on GitHub withdraws it. Installing from the app comes with a later version; until then:

To install a Mac release, download its zip and `.sha256` into one folder, check them with `shasum -a 256 -c Job-Search-Hub-0.1.<N>.zip.sha256`, unzip with `ditto -x -k Job-Search-Hub-0.1.<N>.zip .`, and run `macos/Scripts/install-app.sh JobSearchHub.app`, which checks the bundle's team, quits the app, stops the server, puts the bundle in `~/Applications` and starts the server from it.

The repository is public, so anyone can download the APK. It holds no tokens, since a phone pairs at runtime. It does carry the Firebase client config from `google-services.json`. That's how Firebase client config works: it names the Firebase project but doesn't let anyone send pushes, which takes the service account key that stays on the Mac.

### One-time setup

Until an app's signing secrets exist, its release job fails at its first step, naming the missing ones, and publishes nothing.

**The Android app:**

1. **Make the release key** on the Mac, outside the repository:

   ```bash
   keytool -genkeypair -v -storetype PKCS12 -keystore ~/job-search-hub-release.keystore \
     -alias job-search-hub -keyalg RSA -keysize 4096 -validity 10000 \
     -dname "CN=Job Search Hub"
   ```

   It asks for a password; with PKCS12 the key's password is the same one. **Store the keystore file and its password in the password manager before anything else.** Every release has to be signed with this key: without it, a new APK can't install over the old one, and the phone has to uninstall the app and pair again.

2. **Add the repository secrets** in Settings › Secrets and variables › Actions › New repository secret:

   | Secret | Value |
   | --- | --- |
   | `RELEASE_KEYSTORE_BASE64` | `base64 -i ~/job-search-hub-release.keystore \| pbcopy` |
   | `RELEASE_KEYSTORE_PASSWORD` | the keystore's password |
   | `RELEASE_KEY_PASSWORD` | the same password |
   | `GOOGLE_SERVICES_JSON_BASE64` | optional: `base64 -i android/app/google-services.json \| pbcopy`. Without it, releases have pushes off and the run warns. |
   | `LINEAR_RELEASE_API_KEY` | optional: a Linear personal API key made only for releases (Linear › Settings › Security & access › Personal API keys), not Symphony's. With it, each release run is announced in one update on the initiative the Job Search Hub project belongs to: the version, a link per platform it released, and the changelog. A project in no initiative gets a project update instead, and the run warns. |

   Then delete `~/job-search-hub-release.keystore`; the password manager keeps it.

   The key's alias isn't a secret: the job uses `job-search-hub`, or the repository variable `RELEASE_KEY_ALIAS` (Settings › Secrets and variables › Actions › Variables) for a key made with another alias. Don't make it a secret: Actions hides a secret's value in every log, and this one is the repository's name. A `RELEASE_KEY_ALIAS` secret left from an older setup is unused; delete it.

3. **Ship the first release:** Actions › release › Run workflow, with the tags left empty, or merge the next app change.

4. **Swap the debug build for the release**, once: a debug build is signed with another key, so the release can't install over it. Uninstall the app, install the release and pair again. Later releases install over it.

**The Mac app**, signed with the owner's personal Apple Development certificate, the one `make-app.sh` signs local builds with, never a work one:

1. **Export the certificate** with its private key: in Keychain Access › login › My Certificates, find "Apple Development: <your Apple ID> (…)", the one whose Organizational Unit (in Get Info) is your personal team's ID, and check that it holds a private key. Pick it, then File › Export Items…, as Personal Information Exchange (`.p12`), to `~/job-search-hub-signing.p12` outside the repository, with a new password. **Store the file and its password in the password manager.**

2. **Add the repository secrets**, as above:

   | Secret | Value |
   | --- | --- |
   | `MAC_SIGNING_CERTIFICATE_BASE64` | `base64 -i ~/job-search-hub-signing.p12 \| pbcopy` |
   | `MAC_SIGNING_CERTIFICATE_PASSWORD` | the `.p12`'s password |
   | `MAC_SIGNING_TEAM_ID` | the team ID, ten capital letters and digits: the certificate's Organizational Unit, or `cat ~/.config/job-search-hub/codesign-team-id` once a local build has pinned it |

   Then delete `~/job-search-hub-signing.p12`; the password manager keeps it.

3. **Ship the first release:** Actions › release › Run workflow, with the tags left empty, or merge the next server or Mac change.

The job imports the certificate into a keychain of its own, which it deletes once the app is signed, or after a failed step. It signs only with the identity of `MAC_SIGNING_TEAM_ID`'s team, and fails if the `.p12` holds none.

A Linear update that failed can be posted again from Actions › release › Run workflow, with the run's release tags, e.g. `mac-v0.1.252 android-v0.1.252`.

To build a signed release locally, set the four `RELEASE_*` variables Gradle reads (`RELEASE_KEYSTORE_PATH` is the keystore file's path) and run `./gradlew :app:assembleRelease` in `android/`. With none of them set the release APK is unsigned; with only some, the build fails and names the missing ones.

### Renewing the Mac certificate

An Apple Development certificate lasts a year. The Mac release job writes its expiry date in the run's summary, warns there and in an annotation from 30 days before it, and fails, naming the certificate, once it has expired. To renew it:

1. In Xcode › Settings › Accounts, pick the personal team, Manage Certificates…, and add an Apple Development certificate. The new one keeps the common name and the team, so the app's designated requirement, and the Keychain's trust in it, stay the same.
2. Export it as in the one-time setup, step 1, and replace `MAC_SIGNING_CERTIFICATE_BASE64` and `MAC_SIGNING_CERTIFICATE_PASSWORD` with the new file and password. `MAC_SIGNING_TEAM_ID` stays.
3. Re-run the failed `release` run, or merge the next server or Mac change.

### Versions

Every part of the hub carries one version, `0.1.<N>`, where `N` is the commit count on `main`, as the APK's `versionCode` is: the Mac app (`CFBundleShortVersionString` and `CFBundleVersion`), `hub-server`, `hub`, `hub-update` and `hub-cvprint`. `scripts/release/version.sh` prints it. A release build passes `HUB_VERSION_CODE=<N>`, as `release.yml` passes `-PversionCode` to Gradle; any other build is `0.1.0-dev.<short commit>`. `macos/Scripts/make-app.sh` stamps them all, `hub-cvprint` through `macos/Scripts/build-cvprint.sh`, the Go commands with `version.sh --go-ldflags`.

- **About Job Search Hub** shows the app's version, and Settings › Phones each phone's, once it has called the hub.
- `hub-server --version` prints the version, the commit and the newest migration it knows; `hub --version` and `hub-cvprint --version` print theirs.
- `GET /v1/version`, which needs no token, answers `{"version":"0.1.<N>","commit":"…","newest_migration":84}`.

The apps send `X-Hub-Client: macos/<version>` or `android/<version>` with every request, and the server keeps each paired phone's. It can answer an app it no longer serves with `426 Upgrade Required` and a message, which the app shows in place of the page; `clientMinimums` in `server/cmd/hub-server/main.go` sets the oldest release per platform, and none is turned away yet.

The server refuses to start on a database migrated past its newest migration, by a newer release, and logs `The database is at migration 91; this server knows up to 88. Install 0.1.<M> or later, or restore a backup from before it.` Each release records itself in the database as the first to know its newest migration, which is how an older server names `0.1.<M>`.

### Moving to a new Postgres major

The server runs its database on the newest engine in `engines/`. When an update adds a newer major beside the one the database is on, say `postgres-19` beside `postgres-18`, the next start moves the data: it dumps the 18 cluster with 19's `pg_dump` to `backups/hub-pre-upgrade-18-to-19-<date>.dump`, restores the dump into a new cluster, `postgres/19.partial`, checks that every table has the rows it had, renames it to `postgres/19/`, and starts on it. The old `postgres/18/` stays; a later release removes it.

If any step fails, the server removes `19.partial`, logs why, and runs on the 18 cluster with the 18 engine; the next start tries again. `/v1/health` then answers `{"database":"ok","postgres":{"major":18,"upgrade_failed_to":19}}`. If the update no longer carries the 18 engine, the server refuses to start and names the newest dump to restore with `hub-server database restore`.

**Release rule:** a release that moves to a new Postgres major ships the old major's engine too, and every release keeps shipping it for a year after. None of them ships a migration that needs the new major, since a failed move runs the hub on the old one. See `docs/design/owned-database.md` › Engine upgrades with app updates.

## Where it runs

The hub stays on the Mac rather than moving to an always-on PC (decided October 2026). The reasons:

- **It already runs all day.** With system sleep off, the server keeps running under launchd (`KeepAlive`) and owns its database, a Postgres it starts and stops itself, so a locked screen doesn't take the hub down.
- **Downtime loses nothing.** Gmail catches up from the stored history ID, Pub/Sub holds notifications for 7 days, alert emails of the last 30 days are read on the next pass, and board and feed polls pick up where they stopped. Moving would only add gathering while the Mac is shut or away.
- **Moving costs more than that.** The server runs natively so it can use the Mac's GPU for local models. The phones reach it over HTTPS through `tailscale serve`. The Google sign-in and the Pub/Sub listener live here too. All of that would have to be set up again on the PC, where the server would run against a Postgres of its own through `HUB_DATABASE_URL`.

Revisit this if the PC turns out to run the local models well. If it does, move the model worker there first and leave the hub where it is.

## CI

`.github/workflows/ci.yml` runs on every pull request and push to `main`. Jobs that only matter for one part of the repo skip when a change doesn't touch it, and one job, `ci`, passes when nothing it needs failed. `ci` is the only check `main` requires, so a skipped job never leaves a PR waiting.

`main` is protected by `.github/rulesets/main.json`: changes land through a pull request with no required approvals, since Symphony opens PRs under the owner's account and the approval is moving the Linear ticket to `Merging`. `ci` from GitHub Actions has to pass, but the branch doesn't have to be up to date with `main`; the push run on `main` catches the rare conflict. Force pushes and deleting `main` are blocked, and nobody can bypass the rules, admins included: in an emergency, turn the ruleset off in Settings › Rules first.

`.github/workflows/postgres-engine.yml` builds the Postgres engine the hub will run on the Mac, on a macOS runner, and publishes it as a `postgres-v…` release that `server/postgres-engine.lock` pins. It runs only for changes to `scripts/postgres-engine/`, the lock or itself, and builds only when `build.sh` changes. See `scripts/postgres-engine/README.md`.

The ruleset and these settings need a repository admin, once:

1. **Settings › Rules › Rulesets › New ruleset › Import a ruleset**, pick `.github/rulesets/main.json`, and create it.
2. **Settings › General › Pull Requests**: allow squash merging only, since Symphony squash-merges; turn on **Allow auto-merge**, so Symphony can land an approved PR when `ci` goes green; turn on **Automatically delete head branches**.

## Layout

```
server/cmd/hub-server     the server
server/cmd/hub            the CLI
server/internal/…         store, MCP tools, REST API, prompts, stream parsing
server/agents/…           each agent's result contract
server/migrations         the schema, applied at server start
```

See `CLAUDE.md` for how to run the tests.
