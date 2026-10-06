#!/bin/sh
# Builds the Postgres engine the hub runs on the Mac, from the official source
# tarball, and packs it as postgres-<version>-macos-arm64.tar.gz in <out-dir>:
#
#   postgres-18/
#     bin/      postgres initdb pg_ctl pg_dump pg_restore psql pg_isready
#     lib/      libpq, and postgresql/ with the server's own modules
#     share/    postgresql/: what initdb copies, the time zones, text search
#     COPYRIGHT
#
# No ICU, readline or Homebrew library: a binary links only the system's and,
# through @loader_path, its own, so the folder runs wherever it is unpacked.
# Postgres finds share/ and lib/ relative to its binary by itself.
#
#   build.sh <out-dir> [<work-dir>]   build; the work dir defaults to a temp one
#   build.sh --version                print the Postgres version it builds
#
# Runs on an Apple Silicon Mac with Xcode or its command line tools. The
# postgres-engine workflow runs it, then check.sh on the result.

set -eu

POSTGRES_VERSION=18.6
POSTGRES_SHA256=555610c24d53e4316da5b7d3fc25c279d96856d5e0e23ee308c328c5fa881d9f
POSTGRES_URL=https://ftp.postgresql.org/pub/source/v$POSTGRES_VERSION/postgresql-$POSTGRES_VERSION.tar.bz2

# The Mac app's floor (macos/Package.swift).
DEPLOYMENT_TARGET=26.0

PROGRAMS="postgres initdb pg_ctl pg_dump pg_restore psql pg_isready"

fail() {
	echo "::error::$*" >&2
	exit 1
}

if [ "${1:-}" = --version ]; then
	echo "$POSTGRES_VERSION"
	exit 0
fi
if [ $# -lt 1 ] || [ $# -gt 2 ]; then
	echo "usage: $0 <out-dir> [<work-dir>] | --version" >&2
	exit 2
fi

[ "$(uname -s)" = Darwin ] || fail "Builds on macOS only."
[ "$(uname -m)" = arm64 ] || fail "Builds on Apple Silicon (arm64) only."

mkdir -p "$1"
out=$(cd "$1" && pwd)
if [ $# -eq 2 ]; then
	mkdir -p "$2"
	work=$(cd "$2" && pwd)
else
	work=$(mktemp -d "${TMPDIR:-/tmp}/pg-engine.XXXXXX")
fi
case $out$work in
*[[:space:]]*) fail "The out and work dirs can't have spaces in their paths." ;;
esac
# Postgres installs lib/ and share/ without their postgresql/ folders when the
# prefix's path already says postgres or pgsql (src/Makefile.global.in), and the
# layout above, compiled into the binaries, needs those folders.
case $work in
*postgres* | *pgsql*) fail "The work dir's path can't contain 'postgres' or 'pgsql': Postgres would then install lib/ and share/ without their postgresql/ folders." ;;
esac
echo "Building Postgres $POSTGRES_VERSION in $work"

# Only the system's tools and headers: nothing from Homebrew or the caller's
# environment may end up in a binary. pkg-config would find Homebrew's ICU,
# lz4 or zstd, so it gets an empty search path.
PATH=/usr/bin:/bin:/usr/sbin:/sbin
unset CPPFLAGS CFLAGS LDFLAGS LIBS CPATH C_INCLUDE_PATH LIBRARY_PATH PKG_CONFIG_PATH \
	DYLD_LIBRARY_PATH DYLD_FALLBACK_LIBRARY_PATH
mkdir -p "$work/no-pkg-config"
PKG_CONFIG_LIBDIR=$work/no-pkg-config
MACOSX_DEPLOYMENT_TARGET=$DEPLOYMENT_TARGET
export PATH PKG_CONFIG_LIBDIR MACOSX_DEPLOYMENT_TARGET

tarball=$work/postgresql-$POSTGRES_VERSION.tar.bz2
if [ ! -f "$tarball" ]; then
	curl -fsSL --retry 3 -o "$tarball.part" "$POSTGRES_URL"
	mv "$tarball.part" "$tarball"
fi
echo "$POSTGRES_SHA256  $tarball" | shasum -a 256 -c - >/dev/null ||
	fail "$tarball doesn't match the pinned SHA-256 $POSTGRES_SHA256."

src=$work/postgresql-$POSTGRES_VERSION
prefix=$work/install
rm -rf "$src" "$prefix"
tar -xjf "$tarball" -C "$work"

# ICU and readline are on by default; the rest that needs a library is off.
# zlib, for pg_dump's compression, is the system's.
(
	cd "$src"
	./configure --prefix="$prefix" --without-icu --without-readline --with-zlib >"$work/configure.log" 2>&1 ||
		{ tail -n 40 "$work/configure.log"; fail "configure failed; the log is $work/configure.log."; }
	if grep -q 'unrecognized options' "$work/configure.log"; then
		grep 'unrecognized options' "$work/configure.log"
		fail "configure didn't recognise an option."
	fi
	make -s -j "$(getconf _NPROCESSORS_ONLN)"
	make -s install
)

engine_name=postgres-${POSTGRES_VERSION%%.*}
stage=$work/stage
engine=$stage/$engine_name
rm -rf "$stage"
mkdir -p "$engine/bin" "$engine/lib" "$engine/share"

for program in $PROGRAMS; do
	cp "$prefix/bin/$program" "$engine/bin/"
done
# The server's modules (plpgsql, text search, encodings, replication), without
# pgxs, which only builds extensions.
cp -R "$prefix/lib/postgresql" "$engine/lib/"
rm -rf "$engine/lib/postgresql/pgxs"
cp -R "$prefix/share/postgresql" "$engine/share/"
cp "$src/COPYRIGHT" "$engine/"

macho_files() {
	find "$engine" -type f | while IFS= read -r file; do
		if file -b "$file" | grep -q Mach-O; then
			echo "$file"
		fi
	done
}

# The libraries the binaries load from the install, copied in by the name
# they're loaded by (libpq.5.dylib, not the libpq.5.18.dylib it points at).
for file in $(macho_files); do
	otool -L "$file" | tail -n +2 | awk '{print $1}' | while IFS= read -r dependency; do
		case $dependency in
		"$prefix/lib/"*)
			name=${dependency#"$prefix/lib/"}
			[ -f "$engine/lib/$name" ] || cp -L "$dependency" "$engine/lib/$name"
			;;
		/usr/lib/* | /System/Library/*) ;;
		*) fail "$file loads $dependency, which is neither the system's nor Postgres's." ;;
		esac
	done
done

# Every file loses its local symbols, and every load of the install's
# libraries becomes relative to the loading file. Then the file is signed
# again, since the edits break the linker's signature. strip -x keeps the
# postgres binary's global symbols, which its modules link against.
for file in $(macho_files); do
	relative=${file#"$engine/"}
	up=
	dir=$(dirname "$relative")
	while [ "$dir" != . ]; do
		up=../$up
		dir=$(dirname "$dir")
	done
	strip -x "$file"
	changes=
	case $relative in
	lib/*/*) ;;
	lib/*.dylib)
		name=${relative#lib/}
		changes="-id @loader_path/${up}lib/$name"
		;;
	esac
	for dependency in $(otool -L "$file" | tail -n +2 | awk '{print $1}'); do
		case $dependency in
		"$prefix/lib/"*)
			changes="$changes -change $dependency @loader_path/${up}lib/${dependency#"$prefix/lib/"}"
			;;
		esac
	done
	if [ -n "$changes" ]; then
		# shellcheck disable=SC2086 # $changes is a list of options.
		install_name_tool $changes "$file" 2>/dev/null
	fi
	codesign --force --sign - "$file" 2>/dev/null
done

asset=postgres-$POSTGRES_VERSION-macos-arm64.tar.gz
COPYFILE_DISABLE=1 tar --no-mac-metadata -czf "$out/$asset" -C "$stage" "$engine_name"
(cd "$out" && shasum -a 256 "$asset" >"$asset.sha256")

echo "Built $out/$asset"
cat "$out/$asset.sha256"
