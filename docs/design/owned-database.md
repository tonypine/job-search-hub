# The hub owns its database

A proposal for how the installed hub runs its database itself, so nothing else has to run on the
owner's Mac: no Docker Desktop, no Homebrew Postgres. Nothing changes with this document. The work
it proposes is split into tickets at the end, in Backlog until the proposal is approved.

## Summary

**Recommendation: bundle Postgres 18 with the server and run it as the server's child process.**
`hub-server` creates the cluster on first start, starts and stops it with itself, migrates it as
today, dumps it nightly with the bundled `pg_dump`, and moves it to a new Postgres major when an app
update brings one. It talks to it over a Unix socket in a private folder, so the hub opens no
database port and keeps no database password.

The SQL, the 84 migrations, `pgx`, the change log, the tests and the backups stay as they are. The
new work sits around the store, not in it: building a relocatable Postgres for the Mac, a small
package that runs it, and the upgrade path between majors.

SQLite would remove the server process and the engine upgrades, but it means rewriting the schema
and reviewing all 328 queries for behaviour that SQLite doesn't share. That is a large, risky
rewrite of code that works, for a 28 MB database that Postgres handles without effort.

## Today

- Postgres 18 runs in Docker Compose (`compose.yaml`, service `db`, volume `hub-db`, which Compose
  names `job-search-hub_hub-db`), on `127.0.0.1:5434` with a password from `.env`.
- `hub-server` runs natively under launchd (`server/scripts/install-native-server.sh`), reads
  `HUB_DATABASE_URL` from `~/.config/job-search-hub/server.env`, and waits up to three minutes for
  the database at start (`cmd/hub-server/wait_for_database.go`), because at login launchd starts it
  before Docker Desktop has Postgres up.
- It applies pending goose migrations at every start (`store.Migrate`).
- From 03:00 it dumps the database with `pg_dump` from Homebrew's `libpq` into
  `~/Library/Application Support/JobSearchHub/backups/`, keeping the newest 14
  (`internal/databasebackup`).
- Tests that touch the database call `testdatabase.New`, which creates and migrates a fresh database
  per test through `HUB_TEST_DATABASE_URL`. 30 test packages use it. Locally that URL points at the
  Compose Postgres; CI's `server-test` job runs a `postgres:18` service container and installs the
  Postgres 18 client for the backup test.
- The Mac app's Settings › Server starts, stops and restarts the server's LaunchAgent
  (`ServerLaunchAgent`). It knows nothing about the database.

How much of the code is Postgres's, measured on `main`:

| What | Count |
|---|---|
| Migrations | 84 files, about 54 tables |
| Columns of `timestamptz` / `uuid` / `jsonb` / arrays | 100 / 91 / 21 / 9 |
| `gen_random_uuid()` defaults | 35 |
| Query call sites (`Exec`, `Query`, `QueryRow`) | 328, in `internal/store` (55 files) |
| Lines using `now()` | 60, in 37 files |
| Casts (`::jsonb`, `::date`, …) and date functions (`extract`, `age`, `date_trunc`) | 17 and 12 |
| `SELECT … FOR UPDATE` row locks | 21, in 11 files |
| `ANY($n)` array parameters, `unnest`, `LATERAL`, `DISTINCT ON`, `CopyFrom`, `make_interval` | 13, 4, 3, 1, 2, 1 |
| `ON CONFLICT` upserts, `RETURNING` | 39, 76 |

Outside tests, `pgx` is confined to `internal/store`, `internal/testdatabase` and `cmd/hub-server`.

## Options

### A. A bundled Postgres the server runs as a child process (recommended)

The app ships a relocatable Postgres 18 (the server, `initdb`, `pg_ctl`, `pg_dump`, `pg_restore`,
their libraries and `share/`). `hub-server` runs `postgres` itself.

- **Keeps** every query, migration and test as they are, `jsonb` and arrays included, and the
  nightly `pg_dump`. Moving the current data over is a dump and a restore on the same major.
- **Costs**
  - Building Postgres for the Mac so it runs from any folder, with no Homebrew libraries, and
    publishing it pinned by checksum. Postgres is under the permissive PostgreSQL License.
  - A package that starts, watches and stops the cluster, including one left behind by a server that
    crashed. `internal/modelruntime` already starts and stops `llama-server` as a child.
  - Moving to a new major once every few years, by dump and restore with both engines in the app for
    a while.
  - Tens of MB more in the app, and a second process using up to about 150 MB of memory with the
    default `shared_buffers`.
- **Risk:** low for the data, moderate for the build. The SQL the hub has run for months doesn't
  change.

### B. SQLite

One file, opened in-process with `modernc.org/sqlite` (pure Go, so builds stay `CGO_ENABLED=0`).

- **Gains:** no server process, no socket, no orphan to clean up, no engine majors to upgrade (the
  file format has been stable since 2004). Tests open a file in `t.TempDir()`, and CI needs no
  service. Backups are `VACUUM INTO`.
- **Costs**
  - The 84 migrations collapse into a new SQLite schema: 100 `timestamptz` columns become text or
    integer timestamps, 91 `uuid` columns text, 35 `gen_random_uuid()` defaults move into Go, 21
    `jsonb` columns become JSON text with different operators, and 9 array columns become JSON or
    join tables.
  - The store moves from `pgx` to `database/sql`, and every one of the 328 queries is re-read. About
    140 lines use what SQLite lacks: `now()`, Postgres casts and date functions, `ANY($n)` with Go
    slices, `unnest`, `LATERAL`, `DISTINCT ON`, `make_interval` and `CopyFrom`. The checks on
    Postgres error codes (`23503`, `23505`) change to SQLite's.
  - The 21 `FOR UPDATE` locks have no equivalent. SQLite allows one writer at a time, so the
    background workers' writes queue behind each other and need a busy timeout. The places that
    rely on row locks need rethinking one by one.
  - Timestamps, time zones and ordering behave differently in ways the tests may not catch: they
    were written against Postgres.
  - A one-off converter from the Docker database to the SQLite file, table by table.
- **Risk:** high. It touches nearly every store function, all at once, to replace something that
  works. It would also be the biggest change to the server since it started, for no feature.

### C. Others considered

- **Postgres as a prerequisite** (Postgres.app or `brew install postgresql@18`, run as a service).
  Cheapest to build, but it's another thing the owner installs and keeps running, which is what
  the ticket asks to stop. Homebrew also upgrades it on its own schedule.
- **The `fergusstrange/embedded-postgres` Go library.** A variant of A that downloads prebuilt
  binaries from Maven Central on first start and runs `initdb` and `pg_ctl`. It would save the
  lifecycle package but not the hard parts: the download is a runtime network fetch from a third
  party, the binaries are whatever that project builds, and orphans and major upgrades are still
  ours to handle. A's own package is a few hundred lines, modelled on `modelruntime`.
- **PGlite** (Postgres compiled to WebAssembly). It would need a WebAssembly runtime in Go, serves
  one connection, and isn't proven outside JavaScript. Not mature enough for the hub's data.
- **Another container runtime** (Colima, Apple's `container`). Still a VM and a runtime on the
  owner's Mac, which is the thing to remove.
- **An embedded key-value store** (bbolt, Badger) or DuckDB. A rewrite of the data layer bigger
  than B, for no gain over it.

### Comparison

| | A. Bundled Postgres | B. SQLite |
|---|---|---|
| Store and migrations | unchanged | new schema; 328 queries re-read, about 140 lines rewritten |
| Moving today's data | `pg_dump` and `pg_restore` | a converter, table by table |
| Tests | unchanged; new tests for the lifecycle | simpler; every store test re-run against new semantics |
| CI | keeps its Postgres service; installs Postgres 17 and 18 for the lifecycle tests | no service |
| Backups | unchanged, with the bundled `pg_dump` | `VACUUM INTO` |
| Engine upgrades | a new major every few years, by dump and restore | none |
| On the Mac | tens of MB, one more process | nothing |
| Risk | the Postgres build and the lifecycle code | behaviour changes across the whole store |
| Tickets | 8, mostly small | about as many, but two of them are the rewrite and the converter |

A keeps the risk in new code that can be tested on its own. B spreads it across code that works.

## The design

### Where things live

Everything stays under `~/Library/Application Support/JobSearchHub/`, where the binaries, models
and backups already are:

```text
JobSearchHub/
  bin/                  hub-server, hub-cvprint, run-hub-server (today)
  engines/
    postgres-18/        the bundled engine: bin/, lib/, share/; replaced by app updates
  postgres/             0700, the server's alone
    hub-server.lock     held by the server, or by a database command, while it runs
    .s.PGSQL.5432       the Unix socket, 0700
    18/                 the cluster (PGDATA), named by its major
  backups/              nightly dumps, and the ones taken before migrations, upgrades and imports
  models/               (today)
```

- The data directory is named by its Postgres major, so a new major's cluster is built beside the
  old one and the old one stays until the new one has run. Only a folder named by a bare major is a
  cluster the server opens. A cluster being built (by an upgrade, a restore or an import) is
  `<major>.partial` until it's checked; one left by a build that was killed mid-way is removed by
  the next build, which holds the lock, before its `initdb`. `18.replaced-<date>` is left for the
  owner to delete.
- The server finds the engines next to its own binary (`../engines` from `hub-server`), so the
  layout holds wherever TP-381 puts the app. `HUB_POSTGRES_ENGINES` and `HUB_POSTGRES_DIR` override
  both, for development.
- The socket's path is 71 characters plus the user name, within macOS's 103 for user names up to
  32 characters. The server checks the length and says so if a long user name breaks it, rather
  than failing inside Postgres.
- The data directory is excluded from Time Machine (`tmutil addexclusion`). A copy of a live data
  directory is the wrong thing to restore, and only into the same major. The dumps in `backups/`,
  which Time Machine does copy, are the backup.

### Owned or external

`HUB_DATABASE_URL` becomes optional:

- **Unset:** the server owns the database, as below. This is the installed hub.
- **Set:** the server uses that Postgres as today, and waits for it as today. CI, the tests, a
  server on Linux, and the cutover from Docker use this.

The `postgres` settings that matter are passed on its command line when the server starts it, not
written into the data directory, so an app update can change them:

```text
listen_addresses=''              no TCP port at all
unix_socket_directories=<postgres/>
unix_socket_permissions=0700
max_connections=30               the pool, a dump, and a psql session
shared_buffers=128MB
```

The cluster is created with
`initdb --username=hub --auth-local=trust --encoding=UTF8 --locale-provider=builtin --builtin-locale=C.UTF-8`.
Access is by file permissions: only the owner's user can reach the socket. That's the same boundary
as today, where any of the owner's processes can read the password in `server.env`.
`HUB_DATABASE_PASSWORD` leaves `.env`.

The builtin `C.UTF-8` locale has no collation version, so neither macOS updates nor engine updates
can invalidate an index. It sorts text by code point, so `Zeta` comes before `acme`. Today's
Docker Postgres (`en_US.utf8`) sorts them the other way. About ten queries order by a name or a
title, most only to break ties. The ones a list's order depends on, such as the companies list
(`company_summaries.go`) and the company search (`companies.go`), move to `ORDER BY lower(…)` before
the cutover, so nothing changes on screen.

### Starting and stopping

At start, with no `HUB_DATABASE_URL`:

1. **Lock.** The server takes an exclusive `flock` on `postgres/hub-server.lock` for its whole life;
   the kernel drops it when the process ends, even on `SIGKILL`. If it's held, another server or a
   database command owns the database, and the server exits saying which.
2. **Pick the engine and the cluster.** The engine is the newest major in `engines/`; its cluster is
   `postgres/<major>/`. A cluster of a newer major than any engine means the app was rolled back
   past an upgrade: the server refuses to start and says so, rather than open a stale older cluster.
   An older major's cluster alone means an upgrade is due (see below). No cluster means a fresh
   install: `initdb` into `postgres/<major>.partial`, removing any an earlier `initdb` left when it
   was killed, then a rename to `postgres/<major>/`, so a cluster folder is always a finished one.
   The exception is a `<major>.replaced-<date>` folder there without its replacement,
   which means a restore was killed between its two renames: the server refuses to start and names
   the folder to rename back, rather than open an empty cluster.
3. **Clean up an orphan.** If `postmaster.pid` names a live process, a previous server may have died
   and left its Postgres behind. Holding the lock proves no server owns the cluster, but not that
   the process is a Postgres: after a reboot the PID can belong to anything of the owner's. So the
   server signals it only if it is this cluster's postmaster: its executable is a `postgres` binary
   under `engines/` (`proc_pidpath` on the Mac, `/proc/<pid>/exe` on Linux), and its start time
   matches the one Postgres wrote on the file's third line, to within a few seconds. Then
   `pg_ctl stop -m fast` stops it. Otherwise the file is stale and the server deletes it, since
   Postgres refuses to start while the file names a live process. A dead PID needs nothing:
   Postgres replaces the stale file itself.
4. **Start** `postgres -D postgres/<major> -c …` as a child in the server's process group. Its
   stderr goes into the server's log, tagged `postgres`.
5. **Wait** until it accepts a connection on the socket, for up to 30 seconds. If it exits first,
   the error carries its last log lines. The three-minute wait for Docker Desktop is gone.
6. **Migrate**, after a dump if migrations are pending (see Backups), then serve as today.

At stop (`SIGTERM` from launchd or Settings › Server › Stop): the HTTP server and the workers stop
as today, the pool closes, then the server sends Postgres `SIGINT` (fast shutdown) and waits up to
20 seconds, then `SIGQUIT`, which is safe: Postgres recovers from its WAL at the next start. The
LaunchAgent's `ExitTimeOut` goes to 30 seconds so launchd doesn't kill the server mid-way.

If Postgres exits on its own while the server runs, the server logs it and exits non-zero, and
launchd's `KeepAlive` restarts both. One supervisor, launchd, rather than two. If the server is
killed, launchd ends the rest of its process group (it does unless `AbandonProcessGroup` is set),
and step 3 catches anything that escapes, such as a server started from a terminal.

The Mac app needs no change: Settings › Server's Start, Stop and Restart already drive the server,
and the database now comes and goes with it.

### Engine upgrades with app updates

How updates arrive is TP-381's. What this design asks of it: an update replaces `engines/` while
the server is stopped, then starts it again.

- **A minor version** (18.0 to 18.1) has the same data format. The new binaries take over at the
  next start, and there's nothing else to do.
- **A new major** (18 to 19) needs the data moved. Postgres ships one a year and supports each for
  five; 18 is supported until November 2030. The hub moves deliberately, not every year: when the
  bundled major nears its last year, or a new one brings something the hub needs.

The release that moves to a new major ships both engines, `postgres-18` and `postgres-19`, and keeps
shipping the old one for a year, so an owner who skips releases still upgrades. At start, the server
sees `postgres/18/` and no `postgres/19/`, and:

1. starts the 18 cluster with the 18 engine;
2. dumps it with 19's `pg_dump` (a newer `pg_dump` reads older servers) to
   `backups/hub-pre-upgrade-18-to-19-<date>.dump`, and stops it;
3. removes any `postgres/19.partial` an earlier upgrade left when it was killed, runs `initdb` into
   `postgres/19.partial`, restores the dump with 19's `pg_restore`, and checks each table's row
   count against the source;
4. renames `postgres/19.partial` to `postgres/19`, and starts normally.

At 28 MB this takes seconds. `pg_upgrade` would be faster for a large database, but needs the same
two engines plus checks of its own, and dump and restore is the path the backups already prove
every night.

If any step fails, the server removes `19.partial`, logs why, and runs on the 18 cluster with the
18 engine it still ships, so the hub keeps working; the next start tries again. That holds only if
the release's migrations run on both majors, so **a release that moves to a new major ships no
migration that needs it.** `postgres/18/` stays until a later release removes it, after the new
cluster has run for a while. An owner whose release no longer carries the old engine is told to
restore the newest dump, which any newer `pg_restore` reads.

The upgrade path is tested in CI from 17 to 18, so it works before the hub ever needs it.

### Backups

- **Nightly dumps stay as they are**, with the bundled `pg_dump` instead of Homebrew's. Its version
  always matches the server's, which removes the reason CI pins the client today, and the
  `brew install libpq` step.
- **Before pending migrations**, the server dumps to `backups/hub-pre-migration-<version>.dump`, so
  a migration that goes wrong can be undone. The newest 5 stay. At today's size it adds about a
  second to a start that migrates.
- **Before a major upgrade or an import**, a dump as above, kept until removed by hand.
- **Restore** becomes a command: `hub-server database restore <dump>`. It takes the lock, so it
  refuses while the server runs ("stop the hub in Settings › Server first"). It restores into a
  fresh cluster beside the current one, `postgres/18.partial`, after removing any a killed restore
  left (`--no-owner --no-privileges --exit-on-error`), checks it, then swaps it in and keeps the old
  one as `postgres/18.replaced-<date>/` for the owner to delete.
  A restore that fails leaves the current cluster as it was.

### Moving the current data out of `hub-db`

Once the owned database ships, with the server still pointed at Docker:

1. Stop the hub in Settings › Server.
2. Run `hub-server database import "postgres://hub:<password>@localhost:5434/hub"`. It dumps the
   Docker database with the bundled `pg_dump` to `backups/hub-import-<date>.dump`, restores it into
   a new owned cluster as `restore` does, and prints each table's row count in both, failing on any
   difference. It refuses if the owned cluster already holds data, unless given `--replace`.
3. Remove `HUB_DATABASE_URL` from `~/.config/job-search-hub/server.env`.
4. Start the hub. Check Today, the Pipeline and a company's dossier in the Mac app, and the phone.
5. `docker compose stop db`, keeping the volume.
6. After two weeks of nightly dumps from the owned database: `docker compose down`,
   `docker volume rm job-search-hub_hub-db`, and Docker Desktop can go.

The server is stopped from step 1 to step 4, so no write is lost between the dump and the switch.
To go back before step 6: stop the hub, put `HUB_DATABASE_URL` back, `docker compose start db`, and
start it. Writes made on the owned database since step 4 stay only there; dump it and restore into
Docker to keep them.

`install-native-server.sh` changes with it: it installs the engine, and a fresh install writes
`server.env` without `HUB_DATABASE_URL`. An existing `server.env` is kept as today, so nothing
switches until the owner does step 3.

### Tests

`testdatabase.New` stays as it is: it needs a Postgres it can create databases in, through
`HUB_TEST_DATABASE_URL`, and an owned cluster is one. What changes is where that Postgres comes from
on the Mac, now that Docker goes:

- `server/scripts/test-with-postgres.sh` starts a throwaway cluster from the installed engine (or
  `HUB_POSTGRES_ENGINES`) in a temporary folder, points `HUB_TEST_DATABASE_URL` at its socket, runs
  `go test` with the arguments it's given, and stops and removes the cluster on exit. It replaces
  `docker compose up -d db` in `CLAUDE.md`'s optional full run.
- The new lifecycle package (`internal/postgresprocess`) is tested against real Postgres binaries:
  `initdb`, start, stop, an orphan left by a killed parent, a `postmaster.pid` naming a live process
  that isn't Postgres (left untouched, and the cluster still starts), and a crash; later a restore,
  an import and an upgrade from 17 to 18, each also after a leftover `.partial` from a killed run.
  The engines come from `HUB_TEST_POSTGRES_ENGINE` and `HUB_TEST_POSTGRES_OLD_ENGINE`; like
  `testdatabase`, the tests fail rather than skip without them.
- Agent sessions keep leaving the database packages to CI.

### CI

- **The `postgres:18` service stays** for the store and tool suite. An owned database runs the same
  Postgres 18 SQL, and these tests are about the SQL, so a service container shared by every package
  is still the right way to run them.
- `server-test` installs the `postgresql-17` and `postgresql-18` server packages from the PGDG
  repository it already adds, without their default clusters (one would take port 5432 from the
  service), instead of the client alone. It sets `HUB_TEST_POSTGRES_ENGINE=/usr/lib/postgresql/18`
  and `HUB_TEST_POSTGRES_OLD_ENGINE=/usr/lib/postgresql/17` for the lifecycle tests. The backup test
  keeps running, not skipping.
- A new workflow, `postgres-engine.yml`, builds the Mac engine on a macOS runner when its recipe
  changes or on demand, checks it, and publishes it as a release asset (`postgres-v18.<minor>-<n>`).
  Its check: `otool -L` lists only system libraries and `@loader_path`, and a cluster from it starts,
  dumps and restores from a folder outside the build. It runs only when the recipe changes, since
  macOS minutes cost ten times Linux ones.

### What goes away

- `compose.yaml`, the `docker-server` profile and `server/Dockerfile`. A server on another host
  still runs against any Postgres through `HUB_DATABASE_URL`; it just isn't packaged for Docker
  any more, as TP-394 already decided for CI.
- `HUB_DATABASE_PASSWORD` in `.env.example`, the `brew install libpq` step, and the README's
  "Postgres restarts with Docker Desktop".
- Port 5434.

## Rollout

Each step ships on its own and keeps the hub working. Until the cutover (6), the installed hub keeps
using Docker; the steps before it can be tested against a throwaway cluster. Each row is a sub-issue
of TP-411, in Backlog until the proposal is approved.

| # | Ticket | Blocked by |
|---|---|---|
| 1 | TP-599 Build a relocatable Postgres 18 for the Mac and publish it pinned by checksum | none |
| 2 | TP-600 Run a Postgres cluster as a child process: `internal/postgresprocess`, tested in CI | none |
| 3 | TP-601 The server owns its database when `HUB_DATABASE_URL` is unset, and the install script installs the engine | TP-599, TP-600 |
| 4 | TP-602 Backups and restore with the bundled tools | TP-601 |
| 5 | TP-603 Run the full server suite on the Mac without Docker | TP-599 |
| 6 | TP-604 Move the owner's data out of the Docker volume `hub-db` | TP-602 |
| 7 | TP-605 Move the owned database to a new Postgres major when an app update brings one | TP-602 |
| 8 | TP-606 Retire Docker: Compose, the Dockerfile and the database password | TP-603, TP-604 |

## Open questions for review

- **Bundled Postgres over SQLite.** The recommendation above. SQLite stays possible later; nothing
  here makes it harder.
- **Building Postgres ourselves.** Ticket 1 builds it from the official source tarball, pinned by
  checksum, on a macOS runner. The alternative is to repackage a prebuilt one (zonky's, which
  `embedded-postgres` uses): faster to start, but a third party's build of the binary that holds the
  data.
- **Sorting by code point**, with the queries that sort names moved to `lower(…)`. The alternative,
  ICU, adds about 30 MB to the app and a collation version that can change under the indexes.
- **No Time Machine copy of the live data directory**, with the dumps as the backup.
- **Deleting `compose.yaml` and the Dockerfile** at the end, rather than keeping them for other
  hosts.
