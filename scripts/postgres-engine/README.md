# The Mac's Postgres engine

The hub will run its own Postgres 18 on the Mac (`docs/design/owned-database.md`), from a folder the
install script and app updates put in place, with nothing from Homebrew. This folder builds that
engine from the official source; `.github/workflows/postgres-engine.yml` publishes it as a GitHub
Release asset, and `server/postgres-engine.lock` pins the asset by its SHA-256.

## What's in it

`postgres-<version>-macos-arm64.tar.gz` holds one folder, `postgres-18/`:

```text
postgres-18/
  bin/        postgres initdb pg_ctl pg_dump pg_restore psql pg_isready
  lib/        libpq.5.dylib, and postgresql/ with the server's own modules
  share/      postgresql/: what initdb copies, time zones, text search
  COPYRIGHT   Postgres's licence, the PostgreSQL License
```

It's built without ICU, readline, OpenSSL or any other library outside macOS; pg_dump's compression
uses the system's zlib. Every binary loads only the system's libraries and its own through
`@loader_path`, and Postgres finds `lib/` and `share/` relative to its own binary, so the folder runs
wherever it's unpacked. The binaries are signed ad hoc, as the linker signs them; a download by
`curl` gets no quarantine flag, so Gatekeeper doesn't stop them.

## The scripts

- `build.sh <out-dir> [<work-dir>]` downloads the source tarball, refuses it unless it matches the
  pinned SHA-256, builds it for macOS 26 on Apple Silicon, and writes the tarball and its
  `.sha256` to `<out-dir>`. `build.sh --version` prints the version it builds. The work dir's
  path can't contain `postgres` or `pgsql`: Postgres would then install `lib/` and `share/` without
  their `postgresql/` folders.
- `check.sh <tarball>` unpacks it into a new folder, checks each binary and library with `lipo`,
  `codesign` and `otool -L`, then creates a cluster there with the hub's `initdb` options, starts
  it on a Unix socket alone, dumps a database with the types the hub uses and restores it into
  another. It runs with a bare environment, so nothing outside the folder helps it pass.

Both run on any Apple Silicon Mac. An agent's sandbox can build and run the binary checks, but
not start the cluster: it blocks the shared memory Postgres needs.

## Publishing a new version

1. Change `POSTGRES_VERSION` and `POSTGRES_SHA256` in `build.sh`, from the `.sha256` file next to
   the tarball on https://ftp.postgresql.org/pub/source/.
2. Open a pull request. The workflow builds and checks the engine, publishes it as
   `postgres-v<version>-<n>` (`n` counts the builds of that version), then fails its `lock` job,
   printing the three lines the lock should hold.
3. Put those lines in `server/postgres-engine.lock` and push. The workflow finds the release built
   from this `build.sh`, skips the build, checks that the lock pins it, and runs `check.sh` on the
   published asset.

The release's notes carry the SHA-256 of the `build.sh` it was built from, which is how a run finds
it. A run whose `build.sh` already has a release builds nothing, so the merge to `main` and later
pull requests cost a short check, not a build. To build again without changing the recipe, for a
newer Xcode say, run the workflow from the Actions tab with `publish`, then update the lock the
same way.

Releases are tagged `postgres-v…` and never marked latest, so they don't touch the Android app's
releases.
