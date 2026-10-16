package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// The variables a server.env from before the server owned its database set
// for compose.yaml's Postgres, which the repository no longer has.
var composeVariables = []string{"HUB_DATABASE_URL", "HUB_DATABASE_PASSWORD"}

// isComposeDatabase says whether databaseURL names the Postgres compose.yaml
// ran: the hub role and database on this Mac's port 5434. Any other
// Postgres, on another host or port, is one the owner chose and keeps.
func isComposeDatabase(databaseURL string) bool {
	address, err := url.Parse(databaseURL)
	if err != nil || (address.Scheme != "postgres" && address.Scheme != "postgresql") || address.User == nil {
		return false
	}
	switch address.Hostname() {
	case "localhost", "127.0.0.1", "::1":
	default:
		return false
	}
	return address.Port() == "5434" && address.User.Username() == "hub" && strings.TrimSuffix(address.Path, "/") == "/hub"
}

// databaseMove moves a hub still on compose.yaml's Postgres into the
// database the server owns, in engines' newest engine, dir and backups.
type databaseMove struct {
	engines, dir, backups string
	// isOld says whether a HUB_DATABASE_URL is the one to move:
	// isComposeDatabase, or a test's own Postgres.
	isOld func(databaseURL string) bool
	check importCheck
}

// run moves the data of the database envFile's HUB_DATABASE_URL names, when
// isOld says it's compose.yaml's: it imports it as hub-server database import
// --replace does, and only once the import succeeded writes envFile again
// without HUB_DATABASE_URL and HUB_DATABASE_PASSWORD, every other line as it
// was. A failure leaves envFile untouched, so the server starts on the old
// database again. Any other HUB_DATABASE_URL, or none, moves nothing.
//
// It replaces because a server.env that still names compose.yaml's Postgres
// never ran on the database the server owns: whatever that holds is the copy
// of a move an install rolled back, kept beside it once replaced. After a
// move, envFile names it no more, so no second import replaces the hub's data.
func (move databaseMove) run(ctx context.Context, envFile string, out io.Writer) (moved bool, err error) {
	settings, err := os.ReadFile(envFile)
	if err != nil {
		return false, fmt.Errorf("read the server's settings: %w", err)
	}
	variables, err := parseEnvFile(bytes.NewReader(settings))
	if err != nil {
		return false, fmt.Errorf("read %s: %w", envFile, err)
	}
	// The server keeps the first value of a variable set twice.
	var source string
	for _, variable := range variables {
		if variable[0] == "HUB_DATABASE_URL" {
			source = variable[1]
			break
		}
	}
	if source == "" || !move.isOld(source) {
		fmt.Fprintln(out, "No database to move: server.env doesn't point at compose.yaml's Postgres.")
		return false, nil
	}

	fmt.Fprintln(out, "Moving the hub's data from compose.yaml's Postgres into the database the server owns")
	if err := importDatabase(ctx, move.engines, move.dir, move.backups, source, true, move.check, out); err != nil {
		return false, err
	}
	if err := writeEnvFileWithout(envFile, settings, composeVariables); err != nil {
		return false, fmt.Errorf("the data was imported, but %s still points at compose.yaml's Postgres: %w", envFile, err)
	}
	fmt.Fprintf(out, "Removed %s from %s: the server now runs on the database it owns.\n", strings.Join(composeVariables, " and "), envFile)
	return true, nil
}

// writeEnvFileWithout replaces path, whose content is settings, with the
// same lines but those that set one of names. It writes beside path and
// renames, so path is either as it was or the new file, chmod 600.
func writeEnvFileWithout(path string, settings []byte, names []string) error {
	var kept bytes.Buffer
	for line := range strings.Lines(string(settings)) {
		name, _, _ := strings.Cut(strings.TrimPrefix(strings.TrimSpace(line), "export "), "=")
		if !strings.Contains(line, "=") || !slices.Contains(names, strings.TrimSpace(name)) {
			kept.WriteString(line)
		}
	}
	written, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(written.Name())
	if _, err := written.Write(kept.Bytes()); err != nil {
		written.Close()
		return err
	}
	if err := written.Chmod(0o600); err != nil {
		written.Close()
		return err
	}
	if err := written.Sync(); err != nil {
		written.Close()
		return err
	}
	if err := written.Close(); err != nil {
		return err
	}
	return os.Rename(written.Name(), path)
}
