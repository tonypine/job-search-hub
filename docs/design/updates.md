# How the hub updates itself

A design for getting each merge to `main` onto the running hub without anyone pulling the repository
and running scripts. It's written from the owner's side: how they learn a new version exists, what
they do to install it, what happens to the work running at that moment, and how they get back to the
last good version. Nothing in the hub changes with this document. The work it proposes is split into
tickets at the end.

## Words

In the hub, an **update** is already an item in the Updates feed: a follow-up that fell due, a fresh
match, a reply. To keep the two apart, this design calls a release a **new version**, and the
apps say it that way too: "New version", "Install 0.1.520", "Settings › Version". A version never
appears in the Updates feed, except for one case: an install that failed and was rolled back
(see [Failures](#failures)).

## Today

After each merge, the supervisor session runs these steps by hand:

1. `git pull` on `main` in `~/repos/job-search-hub`.
2. `server/scripts/install-native-server.sh`, which builds `hub-server` into
   `~/Library/Application Support/JobSearchHub/bin/`, builds `hub-cvprint` beside it, and boots the
   `com.tonypine.jobsearchhub.server` LaunchAgent out and back in.
3. `macos/Scripts/make-app.sh`, then quit and reopen the app, which runs from
   `macos/build/JobSearchHub.app` in the checkout.

The Android app already updates on its own channel: each merge that changes `android/` becomes a
signed APK on GitHub Releases (`docs/decisions/0001-android-release-distribution.md`), which the
owner downloads on the phone.

What the code does at a restart today, which the design builds on:

- **The server's background work is driven by the database.** Every poller and writer (boards,
  feeds, facts, briefs, CV drafts, screens, interview packs, mail triage) runs as a pass over rows
  that still need work, so a pass a restart cuts short is picked up by the next one. Comparisons
  resume at start (`comparisons.Runner.RunUnfinished`), agent runs whose process died are closed
  (`abandonedruns`), Gmail catches up from its history ID, and the Mac app's event stream resumes
  from the last event it heard (`HubEventStream`).
- **What a restart does lose** is the call in flight: a local model call in `modelqueue`, which is
  only in memory, or a `claude -p` run for a full brief or a CV draft (`claudeprint`, run with
  `exec.CommandContext`, so it's killed with the server). The next pass runs it again from the start.
  A local model call costs only time. A `claude -p` run uses the Claude plan again. The server
  stops 5 seconds after `SIGTERM` (`shutdownTimeout`), and nothing waits for background work.
- **MCP sessions live in the server's memory.** `mcptools.NewHandler` serves streamable HTTP with
  the SDK's default, stateful sessions. After a restart, the session ID that an open Claude
  session holds is unknown to the server.
- **Embedded Claude sessions are the Mac app's child processes** (`ClaudeSessionHost`). Quitting
  the app ends them. The conversation survives in Claude Code's transcript, and *Resume session*
  picks it up again, but a turn in progress is cut off. The app already knows what each session is
  doing: its hooks write `working`, `blocked` (a permission prompt or a question) or `idle`.
- **The Mac app's other children**: company research and job fixes run through the bundled `hub`
  command (`CompanyResearch`, `RemoteTaskRunner`), including the tasks a phone queues with *Ask the
  Mac…*.
- **Migrations** are goose files embedded in `hub-server`, applied at start (`store.Migrate`).
  Every file has a `-- +goose Down` section, but no test runs them, and some of them drop data.
- **Backups**: from 03:00 the server writes a nightly `pg_dump` and keeps the newest 14
  (`databasebackup`).
- **Signing**: `make-app.sh` signs with the first `Apple Development:` identity in the keychain.
  That keeps the app's designated requirement stable, so its Keychain item (the owner token)
  survives rebuilds. `hub-server` and `hub-cvprint` aren't signed beyond the linker's ad-hoc
  signature, and neither touches the Keychain.

## Principles

1. **Install only what `main` passed.** A new version is a commit on `main` whose `ci` check
   passed. Nothing else gets installed automatically: not a branch, not the owner's working
   checkout.
2. **Prepare in the background, interrupt briefly.** Fetching and building take minutes and change
   nothing that's running. Only the swap interrupts anything, for seconds.
3. **Don't cut work off, and don't wait forever.** Before the swap, the hub stops starting new
   work and waits for the work already running. If that takes too long, the owner chooses what
   happens. Nobody is there to choose at night, so the install waits for the next night.
4. **A failed install rolls itself back.** If the new version doesn't come up, the hub goes back to
   the old one, database included, and says so. The owner never has to repair a half-installed
   hub.
5. **Going back is honest about data.** When going back means restoring the database, the owner
   sees what would be lost before confirming.
6. **Nothing secret leaves the Mac.** Builds are signed with the owner's personal identity, from
   their own keychain.

## The flow, as the owner lives it

### 1. Learning a new version exists

Once an hour, the updater checks `main` (see [The updater](#the-updater)). When it finds a newer
commit whose `ci` passed, it prepares that version in the background: fetch, build, sign, verify.
The owner hears about it only when the version is **ready to install**, never "available, please
wait for the build".

Where it shows:

- **The foot of the main window's sidebar**, below the sessions, shows `New version 0.1.520`.
  It's a small label that doesn't badge the Dock or interrupt. Clicking it opens Settings ›
  Version.
- **Job Search Hub › Check for New Version…** in the app menu runs a check right away and opens
  Settings › Version.
- **⌘K** finds *Install new version* and *Check for new version*.
- **No notification** for a ready version. Merges land several times a day. A banner for each
  would be noise, and the night install (below) handles most of them.

### 2. Seeing what's in it

**Settings › Version** is a new Settings tab, beside Connection, Server, Accounts, Phones and
Models:

```
┌ Version ─────────────────────────────────────────────────────────────┐
│ Job Search Hub 0.1.512 · server 0.1.512 · installed today, 03:31     │
│                                                                      │
│ ● 0.1.520 is ready to install                    [Install now…]      │
│   8 changes since 0.1.512                                            │
│                                                                      │
│   New                                                                │
│     Mac   Suggest a second route for applications unanswered…  #67   │
│     Phone Snooze a follow-up from its notification             #70   │
│   Fixed                                                              │
│     Mac   Keep the inspector from looping the window's layout  #66   │
│     Server Retry board polls that time out                     #71   │
│   Show all 8 changes                                                 │
│                                                                      │
│ Install new versions  (•) At night, when nothing is running          │
│                       ( ) Only when I choose                         │
│                                                                      │
│ Previous version: 0.1.509             [Go back to 0.1.509…]          │
│ [Check now]  [Show install log]                                      │
└──────────────────────────────────────────────────────────────────────┘
```

- The list comes from the squash-merge subjects on `main` between the running commit and the new
  one, as `scripts/release/changelog.sh` already does for Android. Each subject's `feat:`, `fix:` or
  `perf:` prefix sorts it under **New**, **Fixed** or **Faster**. `chore`, `test`, `docs`,
  `refactor` and `style` stay hidden behind *Show all*, since they don't change what the owner
  sees. A revert shows under **Fixed** as "Took out: …".
- Each line is tagged **Mac**, **Server** or **Phone**, from the paths the commit touched (`macos/`,
  `server/`, `android/`), and links to its pull request.
- Phone changes are listed here but install on the phone (see [Android](#android)).
- When the running version is a local build (see [Local builds](#local-builds)), the header says
  "Local build of `branch` at `abc1234`".

### 3. Choosing when it's applied

There are two ways to install, and the owner picks between them with **Install new versions**:

- **At night, when nothing is running** (the default). From 03:30, half an hour after the nightly
  backup, the updater installs the ready version if the hub is idle: no Claude session working
  or waiting on the owner, no agent run, no remote task, no unsaved edit in the app, and the
  server's drain finishes within its limit (see [Draining](#draining-work-in-flight)). If any of
  that fails, nothing changes, and the next night tries again. Merges during the day then arrive
  as one install the next morning.
- **Only when I choose**: the version waits in Settings › Version and the sidebar until
  the owner installs it.

Either way, **Install now…** is always there.

### 4. Installing

*Install now…* opens a sheet that says what's about to happen, using what's running at that
moment:

```
┌ Install 0.1.520 ─────────────────────────────────────────────────────┐
│ The hub's server restarts, which takes about 10 seconds, and Job     │
│ Search Hub reopens. Your phone shows the hub as offline meanwhile.   │
│                                                                      │
│ Running now                                                          │
│   ◐ Claude session · Acme, Staff Engineer         working            │
│   ◐ Researching Initech                           agent run, 2 min   │
│   ✎ Profile has unsaved edits                                        │
│   ○ 2 Claude sessions idle: they reopen where they left off          │
│                                                                      │
│ This version changes the database. A copy is saved first.            │
│                                                                      │
│        [Cancel]   [Install anyway]   [Install when these finish]     │
└──────────────────────────────────────────────────────────────────────┘
```

- **Install when these finish** is the default. The hub stops starting new background work right
  away, and the sheet turns into a progress view that ticks off each item as it ends. The owner can
  keep working: a session that's working can finish its turn. A session the owner messages again
  goes back to `working`, and the install keeps waiting.
- **Install anyway** cuts the work off. The sheet first says what that costs, for example: "The
  Acme session's turn stops mid-way. Its conversation is kept, and it reopens with the new version.
  Researching Initech fails; run it again afterwards."
- Unsaved edits block both buttons until they're saved or discarded. The sheet's line for each one
  opens it.

Then the progress view shows the steps, as the updater reports them:

```
✓ Prepared 0.1.520
✓ Work finished
✓ Database saved (hub-pre-0.1.520.dump)
◐ Restarting the server…
○ Reopening Job Search Hub
```

The app quits by itself on the last step, and the new one opens with the same windows. The Claude
sessions that were open reopen in their tabs, resumed from their transcripts. A one-line banner
says **Now on 0.1.520 · What's new**, and opens the same list as Settings › Version. It sits at the
top of the page, where the connection banner (`ConnectionBanner`) goes, and gives way to it while
the hub can't be reached. It goes away when dismissed, or after a day.

After a night install, the banner is there in the morning, with the same text.

### 5. If the app isn't running

The updater doesn't need the app open. It installs the server, puts the new app in place, and the
next time the owner opens the app, it's the new one, with the banner.

## What updates, and in what order

One version is one commit on `main`, so the server, `hub-cvprint`, the bundled `hub` command and
the Mac app always come from the same commit. The Android app has its own channel.

| Order | Part | How it's swapped | Downtime |
|---|---|---|---|
| 1 | `hub-server` and its migrations | `current` points at the new version's folder, then launchd restarts the agent | ~10 s, longer for a slow migration |
| 1 | `hub-cvprint` | Same folder as the server, so it switches at the same moment. The server runs it per CV, so nothing holds it open | none of its own |
| 2 | Mac app, with the bundled `hub` | The app quits, the bundle is replaced, and the app reopens | the app's relaunch |
| — | Android | GitHub Releases, as today. The phone says a new version exists | none on the Mac |

**The server goes first** because the new app may call new endpoints, while the old app keeps
working against the new server. That's the compatibility rule that makes the order safe:

- **The server keeps its API working for the previous app and the paired phones.** It can add
  endpoints and fields. Renaming or removing one waits until the clients that use it have
  shipped without it. The phone matters most here, since it updates by hand and may lag by days.
- **Clients send their version** with every request (`X-Hub-Client: macos/0.1.520`,
  `android/0.1.498`). The server keeps the version each paired phone last used. When a change has
  to drop an old client after all, the server answers that client with `426 Upgrade Required`
  and a message, which the client shows instead of a decoding error: "Install the newest phone app
  to keep using the hub".
- **`hub-cvprint` is the server's own tool**, so it switches with the server. A CV printed during
  the swap is retried by the next CV pass, as any failed print is today.

The `hub` CLI that the owner runs in a terminal (`go install ./cmd/hub`) isn't part of the
install. The new layout gives it a stable path (`current/bin/hub`), and the setup docs say to put
that on the `PATH` instead of `go install`.

## Draining work in flight

The swap needs the server and the app to stop doing things for a moment. Draining is how they
reach that moment without losing work. What happens to each kind of work:

| Work | Where it runs | During a drain | If cut off anyway |
|---|---|---|---|
| Background passes (boards, feeds, facts, briefs, screens, packs, mail and conversation triage, follow-ups, fresh matches, backups) | server | No new pass starts. A pass that's running finishes its current item | The next pass redoes the item. Nothing lost |
| Local model calls (`modelqueue`) | server, `llama-server` | Calls already running finish. Waiting calls stay queued, since they're rebuilt from the database after the restart | Redone by the next pass; costs only time |
| `claude -p` runs: full briefs, CV drafts, comparisons | server | Runs already going finish, up to the drain limit. No new run starts | Redone from the start, using the Claude plan again (comparisons resume where they stopped). That's why the drain waits for them |
| Agent runs (*Add company*, *Find people*, triage) | Mac app, through the bundled `hub` | Wait for them. New ones aren't started | The run fails and `abandonedruns` closes it. The owner runs it again |
| Remote tasks (*Ask the Mac…*, job fixes) | Mac app | Wait. New tasks stay queued in the database | A task the app had claimed goes back to the queue when the app reopens |
| Embedded Claude sessions | Mac app | Wait while any is `working` or `blocked`. `idle` ones don't hold anything up | The turn stops. The conversation is kept and reopens. Text typed but not sent is lost, which the app can't see, so the sheet says so |
| Unsaved edits in the app (profile, prompts, notes) | Mac app | Must be saved or discarded first | Not allowed |
| CV and outreach drafts | database | Nothing to do: they're stored as they're written | — |
| Event stream, pushes, Gmail, Pub/Sub | server and clients | Nothing to do | Clients reconnect and resume. Pushes go out from the database. Gmail catches up from its history ID |
| The phone | phone | Nothing to do | Shows the hub offline for the seconds of the restart |

**A server-only install keeps the app's sessions running.** An install never swaps only the server
on purpose, since the app and the server come from the same commit. But if the new app turns out
broken and only the app is rolled back (see [Failures](#failures)), sessions run on across a server
restart, and their MCP connection has to survive it. Today it doesn't: the session ID the client
holds dies with the server. One of two fixes, chosen by testing what Claude Code does with a
`404` on a stale session (the MCP spec says it should open a new session):

- if Claude Code reconnects by itself, nothing to change but a test that proves it;
- otherwise, serve the hub's MCP endpoint **stateless** (`StreamableHTTPOptions{Stateless: true}`).
  The hub's tools keep no per-session state, and each request already carries its bearer token.

The same fix covers today's manual restarts, including *Restart* in Settings › Server.

**How the server drains.** A new `POST /v1/drain` (owner only) puts the server in draining mode:

- every `Run` loop checks a shared flag before starting a pass;
- the model queue grants no turn to a new ticket;
- `claudeprint` refuses new runs;
- `POST /v1/agent-runs` answers `503` with `Retry-After`.

`GET /v1/drain` lists what's still running: model calls, `claude -p` runs and agent runs, each with
its subject and how long it has been going. `DELETE /v1/drain` cancels draining. Reads and the
owner's writes keep working the whole time, so the app stays usable while it waits. A draining
server that hears nothing from the updater for 10 minutes cancels draining by itself, so a crashed
updater can't leave the hub idle.

On `SIGTERM`, the server stops accepting requests, then waits up to 30 seconds (instead of today's
5) for the running work that the drain didn't already finish.

**How the app drains.** The app writes what it's running to the updater's folder: sessions and
their activity, agent runs, remote tasks, unsaved edits. When the updater asks it to quit, it
records which sessions were open and in which tabs or windows, and quits. On the next launch, it
reopens those sessions.

**Limits.** The night install waits up to 10 minutes for the drain, then cancels it and tries
again the next night. A manual install waits as long as the owner leaves the sheet open, and
*Install anyway* is always one click away.

## The updater

An install restarts the server and the app, so neither of them can run it. A third program does:
**`hub-update`**, a Go command in `server/cmd/hub-update`, run by a one-shot LaunchAgent,
`com.tonypine.jobsearchhub.updater`. It isn't kept alive. It runs on demand, when the app asks
with `launchctl kickstart`, and on a schedule (`StartCalendarInterval`: hourly checks, plus the
03:30 install).

The updater runs in the owner's GUI session, so it can:

- reopen the app;
- reach the login keychain to sign;
- use the toolchains (Go, Swift) the hub already needs.

It doesn't run as root. It reaches the network only to `git fetch` from the configured repository,
and to read GitHub's API for `ci` status and the newest Android release.

Its files live in `~/Library/Application Support/JobSearchHub/`:

```
source/                  the updater's own clone, fetched from origin, main only
versions/
  0.1.512-1de6185/       one folder per installed version
    bin/hub-server  bin/hub-cvprint  bin/hub  bin/hub-update
    Job Search Hub.app
  0.1.520-9f3c2aa/
current  -> versions/0.1.520-9f3c2aa
previous -> versions/0.1.512-1de6185
update/
  state.json             what the updater is doing, written atomically; the app watches it
  request.json           what the app asked for: check, install, go back
  app.json               what the app is running, for the drain
  log                    the updater's log, behind *Show install log*
backups/                 nightly dumps, as today, plus hub-pre-<version>.dump
```

- **The LaunchAgent runs `current/bin/run-hub-server`**, so restarting it picks up the switch.
- **The app is installed in `~/Applications/Job Search Hub.app`**, copied from the version's
  folder, instead of running from the checkout's `macos/build/`. The Dock and Spotlight keep one
  stable path. Keychain access follows the code signature, not the path, so the move doesn't ask
  again.
- **The updater keeps three versions** and removes older ones, along with the pre-update dumps of
  versions it has removed. The nightly dumps keep their own rotation.
- **Versions are numbered like the phone's**: `0.1.<commit count on main>`, plus the short commit.
  The build stamps the number into `hub-server` and the `hub` command (`-ldflags`), into
  `hub-cvprint`, and into the app's `CFBundleShortVersionString` and `CFBundleVersion`. Today
  those are fixed at `0.1.0`. A new `GET /v1/version` reports the server's version, commit and
  schema version.

### Checking

`hub-update check`, run every hour:

1. `git fetch origin main` in `source/`.
2. Take the newest commit on `origin/main` whose `ci` check passed. The updater reads this from
   GitHub's API (`commits/<sha>/check-runs?check_name=ci`). The repository is public, so it needs
   no token, and two requests an hour stay far under the unauthenticated limit. A commit whose `ci`
   is still running is left for the next check.
3. Make sure it descends from the running commit (`git merge-base --is-ancestor`). Since `main`
   refuses force pushes, it always should, so a version that doesn't is refused as an error, never
   installed.
4. If it's newer than both the running version and the one already prepared, prepare it.
5. Read the newest `android-v*` release too, and tell the server, which tells the phones (see
   [Android](#android)).

### Preparing

`hub-update prepare <commit>`:

1. Check the disk space and the toolchains: the Go version in `server/go.mod`, a Swift toolchain
   with the macOS 26 SDK, and `pg_dump`. Anything missing stops the prepare with a message that
   names it ("Go 1.26 isn't installed. Install it with `brew install go`").
2. Check out the commit in `source/` and build everything into `versions/<version>.partial/`,
   with the same commands as `install-native-server.sh` and `make-app.sh`. Those scripts become
   thin wrappers around the same steps, so a manual install and an update can't drift apart.
3. Sign the app and the bundled `hub` (see [Builds and signing](#builds-and-signing)).
4. Verify:
   - `codesign --verify --strict` on the app;
   - the app's designated requirement matches the running app's, so the Keychain keeps trusting it;
   - `hub-server --version` and `hub-update --version` print the expected version;
   - `hub-server migrate --check` lists the migrations the database lacks, without applying them.
5. Rename the folder to `versions/<version>/` and record the version as ready in `state.json`.

A prepare that fails changes nothing that's running. Settings › Version shows "Couldn't prepare
0.1.520" with *Show install log*, and the next check tries again when there's a newer commit. A
version that fails twice isn't retried until a newer one exists.

### Installing

`hub-update install <version>`, a state machine whose state is in `state.json`. If the Mac
sleeps, loses power or the updater crashes, the next run picks up from the recorded step:

1. **Drain.** `POST /v1/drain` to the server. Ask the app, through `state.json`, to drain. Wait
   until both report idle, or until the limit or the owner's *Install anyway*.
2. **Save the database**, only when `migrate --check` listed migrations: `pg_dump` to
   `backups/hub-pre-<version>.dump`. If the dump fails, cancel the drain and stop. Nothing has
   changed yet.
3. **Switch the server.** Point `previous` at the running version and `current` at the new one,
   then `launchctl kickstart -k`. The new server applies its migrations at start, as today.
4. **Check the server.** Within 60 seconds, `GET /v1/version` has to answer with the new version,
   and `GET /v1/health` with `ok`. Otherwise, roll back (see [Failures](#failures)).
5. **Switch the app.** Ask the app to quit, and wait for its process to end. Replace
   `~/Applications/Job Search Hub.app` with a rename, keeping the old one until the end, and open
   the new one. If the app wasn't running, replace it without opening it.
6. **Check the app.** The new app writes `launched` to `app.json` once its window is up and it's
   connected to the hub. If that doesn't happen within 60 seconds, or the app exits, put the old
   app back and open it (see [Failures](#failures)).
7. **Done.** Record the install, with the old and new versions and the dump's name, so *Go back*
   knows what it would cost.

The updater installs a new version of itself like any other part. The next run uses the new one,
so it never replaces the binary that's running.

## Migrations

- **Migrations stay as they are**: goose files, applied by the new server at start, each in its own
  transaction.
- **An old server refuses a newer database.** `hub-server` compares the database's goose version
  with its own newest migration, and won't start when the database is ahead. Its log says "The
  database is at migration 91; this server knows up to 88. Install 0.1.520 or later, or restore a
  backup from before it." Without that check, a server put back without its database would run
  against a schema it doesn't know.
- **No down migrations at runtime.** The `Down` sections stay for development, but rollback
  restores the pre-update dump instead. Down migrations are untested, some drop data, and running
  them needs the new binary's files, while rollback runs the old binary. A dump restores the
  database exactly as it was, and it's the same restore the nightly backups already rely on.
- **A slow migration shows.** If the server hasn't answered after 10 seconds, the progress view
  says "Updating the database…" instead of looking stuck. The 60-second check waits as long as
  the log shows migrations still running.

## Failures

Every failure leaves the hub on a version that works, and says what happened in Settings ›
Version, with *Show install log*.

| What fails | What happens | What the owner sees |
|---|---|---|
| The check can't reach GitHub | Nothing. It retries next hour | Nothing, unless checks fail for a day: "Couldn't check for new versions since yesterday" |
| `ci` didn't pass on `main`'s newest commit | That commit isn't offered. An older green commit still can be | Nothing |
| A toolchain is missing, or the disk is full | The prepare stops | "Couldn't prepare 0.1.520: Go 1.26 isn't installed" |
| The build or verification fails | The prepare stops; `.partial` is removed | "Couldn't prepare 0.1.520", with the log |
| The drain runs past its limit at night | Draining is cancelled; next night tries again | The version stays ready |
| The pre-update dump fails | Draining is cancelled; nothing switches | "Couldn't save the database before installing; nothing changed" |
| The new server doesn't come up, or a migration fails | **Automatic rollback**: stop it, restore the pre-update dump if any migration ran, point `current` back, restart the old server, and check it. The new version is marked bad and isn't offered again; the next newer one is | "0.1.520 couldn't start, so the hub went back to 0.1.512. Nothing was lost." Also an update in the feed and on the phones, on the Hub channel, since this can happen at night |
| The new app doesn't open, or crashes within a minute | The old app is put back and opened. The server stays on the new version, which the old app works with, by the compatibility rule | "Job Search Hub 0.1.520 didn't open, so the previous app is back. The server is on 0.1.520." |
| The Mac sleeps or loses power mid-install | The next run reads `state.json` and either finishes or rolls back, never leaving half a switch | The install finishes or rolls back, and says which |
| Rollback itself fails | The updater stops and leaves everything as it is. The app (or the phone, if the server is up) shows the exact state, with the manual commands to finish | "The hub couldn't go back by itself", with the steps |

Restoring the pre-update dump after a failed start loses nothing. The new server never served a
request, and the old one was drained and stopped before the dump.

### Going back later

A version can install fine and still break something the owner only notices later. **Settings ›
Version › Go back to 0.1.512…** handles that:

- **If no migration ran since 0.1.512**, going back only switches `current` and the app, like an
  install in reverse, and nothing is lost. The sheet says so.
- **If migrations ran**, going back also restores `hub-pre-0.1.520.dump`, and anything written
  since is lost. The hub can say exactly what that is, since every write is in the change log. The
  sheet counts the `changes` rows since the dump:

  ```
  Going back to 0.1.512 also takes the database back to today, 14:02,
  before 0.1.520 changed it. Since then: 3 jobs found, 1 card moved,
  2 people added, 4 updates. These would be lost.

  [Cancel]                                    [Go back and lose these]
  ```

  The sheet suggests the other way out too: "Or stay on 0.1.520 and wait for a fix." With merges
  landing daily, that's often the better choice.
- A version the owner went back from is marked bad, like one that failed to start, and isn't
  offered again.
- Going back works one version at a time, to `previous`. A version older than that means
  restoring a backup by hand, as the README describes today.

## Builds and signing

**Builds are made on the Mac, from the updater's own clone, of a commit on `main` that passed
`ci`.** They aren't downloaded artifacts. The reasons:

- **The owner's signing key stays in their keychain.** A CI build would need the personal Apple
  Development certificate and its private key in the repository's GitHub secrets. A Mac app also
  can't be notarized with a Development certificate, so a downloaded one would still be built and
  signed outside Apple's distribution path.
- **The Mac already has the toolchains.** The hub needs Go for the bundled `hub` command, and the
  Swift toolchain for `hub-cvprint`, and today's flow builds both here already.
- **One Mac, one owner.** Artifacts pay off when many machines install the same build. Here the
  cost is a few minutes of background building per version, before the owner is told about it.
- **`ci` is still the gate.** A commit only gets built here after CI built and tested it, and only
  from `main`, which refuses force pushes and changes outside a pull request.

**Signing**:

- The app and its bundled `hub` are signed with the owner's personal Apple Development identity,
  never a work identity. `make-app.sh` picks the first `Apple Development:` identity in the
  keychain today, so a work identity in the same keychain could be picked by accident. The
  updater instead **pins the identity's team ID** in `~/.config/job-search-hub/update.json`,
  chosen once at setup. It refuses to sign with another team, and refuses to install an app
  whose designated requirement differs from the running app's. Both checks keep the Keychain's
  trust in the app across updates.
- The first signature from the updater may make the keychain ask whether `codesign` can use the
  key. Answering *Always Allow* once covers later updates. The setup ticket says so.
- An Apple Development certificate lasts a year. The updater warns in Settings › Version 30 days
  before it expires. A renewed certificate keeps the same name and team, so the designated
  requirement, and the Keychain's trust, stay the same.
- `hub-server`, `hub-cvprint` and `hub-update` keep the linker's ad-hoc signature. None of them
  uses the Keychain, and launchd runs them by path.

**Revisit this** if the hub gets a second Mac or a second owner, or if a Developer ID identity
becomes available. Then CI-built, notarized releases on GitHub, the way Android already works,
start paying for themselves.

### Android

The phone keeps its channel: a signed APK on GitHub Releases for each merge that changes
`android/`. What changes is that the owner hears about it:

- The updater's hourly check reads the newest `android-v*` release and tells the server.
- The phone sends its version with every request, and the server keeps the one each phone last
  used.
- The phone's Settings shows "Version 0.1.498 · 0.1.503 is out" with *Download*, which opens the
  release's APK. Installing still goes through Android's own installer, as today. The phone shows
  one notification per release, on the Hub channel, which the owner can mute there.
- The Mac's Settings › Phones shows each phone's version beside its name.

Installing from inside the app (`PackageInstaller`, with the install-packages permission) is left
out. One phone and one tap through the browser don't justify the permission.

### Local builds

Development keeps working as today. `install-native-server.sh` and `make-app.sh`, run from any
checkout, install a **local build** into the same layout, as `versions/0.1.<N>-local-<sha>/`, and
switch to it. The updater never replaces a local build at night. Settings › Version shows
"Local build of `branch` at `abc1234`" and offers main's newest version, which installs only when
the owner chooses it.

Once the updater ships, the supervisor session no longer has to pull and install after each
merge. The night install, or the owner's *Install now*, does it.

## Out of scope

- Postgres's own upgrades (the `postgres:18` image in `compose.yaml`). They're rare, and need a
  dump and restore that's worth doing by hand.
- Moving the hub to another machine; see the README's *Where it runs*.
- Updating Claude Code, `llama-server` or the local models. They have their own updaters.
- Delta or binary-patch updates. The build is local, so there's nothing to download.

## Decisions for review

These are the choices the tickets build on. Each is cheaper to change now than after they ship.

1. **Builds come from the Mac, not from CI**, signed with the personal identity pinned by team ID.
2. **Night installs are the default.** With *Only when I choose* as the default instead, every
   merge would wait for a click.
3. **Rollback restores a dump instead of running down migrations**, and says what it would lose.
4. **The app moves to `~/Applications/Job Search Hub.app`**, out of the checkout's `macos/build/`.
5. **A separate updater**, run by its own LaunchAgent, rather than the app or the server.
6. **The server keeps its API working for the previous app and the phones**, as a rule for every
   change from now on.

## Tickets

In the order they should land. Each can ship on its own; the later ones need the earlier ones as
marked.

| Ticket | What it delivers | Blocked by |
|---|---|---|
| TP-585 | **Versions everywhere**: `0.1.<N>` stamped into `hub-server`, `hub`, `hub-cvprint` and the app; `GET /v1/version`; `hub-server --version` and `migrate --check`; clients send `X-Hub-Client`, and the server keeps each phone's version; `hub-server` refuses a database newer than its migrations; *About* and Settings show the version | — |
| TP-586 | **Server restarts without breaking work in flight**: drain mode (`/v1/drain`), a longer graceful stop, and MCP sessions that survive a restart | — |
| TP-587 | **Versioned install layout**: `versions/`, `current`, `previous`; the LaunchAgent runs `current`; the app installs to `~/Applications`; the scripts install local builds into it; the one-time move from today's layout | TP-585 |
| TP-588 | **`hub-update check` and `prepare`**: the updater's clone, the `ci` check, the changelog, building and signing with the pinned identity, verification, `state.json`, and the LaunchAgent with the hourly check | TP-585, TP-587 |
| TP-589 | **`hub-update install` for the server**: drain, pre-update dump, switch, health check, automatic rollback with restore, recovery after a crash, bad versions | TP-586, TP-588 |
| TP-590 | **The app's part in an install**: report what's running to `app.json`, quit when asked, reopen the open sessions after relaunch, put remote tasks back in the queue, launch check and rollback to the old app | TP-589 |
| TP-591 | **Settings › Version and the install sheet**: the sidebar's *New version* label, *Check for New Version…*, the change list, the install sheet with the drain list, progress, the *Now on* banner, and failure notices | TP-590 |
| TP-592 | **Night installs**: the 03:30 run, the idle rules, the *Install new versions* setting, and the failed-install update on the phones | TP-591 |
| TP-593 | **Going back**: *Go back to …*, the cost from the change log, restoring the dump, marking the version bad | TP-591 |
| TP-594 | **The phone hears about new versions**: the server learns the newest APK release, the phone shows it in Settings and notifies once, and the Mac lists each phone's version | TP-585, TP-588 |
