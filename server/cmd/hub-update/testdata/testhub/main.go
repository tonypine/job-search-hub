// Command testhub stands in for hub-server in the bundles hub-update's
// end-to-end test installs. Built with its version and mode, it serves
// /v1/version and /v1/health on HUB_ADDR, logs what the real server logs
// around its migrations, and keeps its "database" in a file in HOME:
//
//	go build -ldflags "-X main.version=0.1.252 -X main.mode=migrate"
//
// Modes: "" serves; "migrate" dumps the database and migrates it first;
// "crash" migrates, then stops with an error, as a failed migration does.
//
//	hub-server
//	hub-server database count-writes <migration>
//	hub-server database restore <dump>
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/serverenv"
)

var (
	version = "0.0.0"
	mode    = ""
)

// fromMigration is the newest migration of the old version's server, whose
// dump the new one takes before migrating.
const fromMigration = 88

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		fail(err)
	}
	support := filepath.Join(home, "Library", "Application Support", "JobSearchHub")
	data := filepath.Join(support, "data.txt")
	dump := filepath.Join(support, "backups", fmt.Sprintf("hub-pre-migration-%d.dump", fromMigration))
	args := os.Args[1:]
	switch {
	case len(args) == 0:
		serve(home, data, dump)
	case len(args) == 3 && args[0] == "database" && args[1] == "count-writes":
		countWrites(dump)
	case len(args) == 3 && args[0] == "database" && args[1] == "restore":
		if err := copyFile(args[2], data); err != nil {
			fail(err)
		}
	default:
		fail(fmt.Errorf("unknown command %v", args))
	}
}

func serve(home, data, dump string) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	settings, err := serverenv.Read(serverenv.Path(home))
	if err != nil {
		fail(err)
	}
	if mode == "migrate" || mode == "crash" {
		logger.Info("migrating the database", "from", fromMigration)
		if err := os.MkdirAll(filepath.Dir(dump), 0o700); err != nil {
			fail(err)
		}
		if err := copyFile(data, dump); err != nil {
			fail(err)
		}
		logger.Info("database dumped before migrating", "dump", dump)
		if err := appendLine(data, "migrated by "+version); err != nil {
			fail(err)
		}
		logger.Info("migration applied", "version", 91)
	}
	if mode == "crash" {
		logger.Error("hub-server stopped", "error", "apply migrations: ERROR: column \"x\" does not exist")
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/version", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"version":%q,"newest_migration":91}`, version)
	})
	mux.HandleFunc("GET /v1/health", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"database":"ok"}`)
	})
	// The rollback's update in the feed, kept in a file the test reads.
	mux.HandleFunc("POST /v1/updates", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+settings["HUB_OWNER_TOKEN"] {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if err := appendLine(filepath.Join(home, "updates.jsonl"), string(body)); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
	})
	server := &http.Server{Addr: settings["HUB_ADDR"], Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	logger.Info("hub-server listening", "version", version, "addr", server.Addr)
	if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		logger.Error("hub-server stopped", "error", err.Error())
		os.Exit(1)
	}
}

// countWrites prints what hub-server database count-writes prints: the
// dump, when it was taken, and that nothing was written since the mark.
func countWrites(dump string) {
	result := map[string]any{"marked": true, "lost": "Nothing was lost."}
	if info, err := os.Stat(dump); err == nil {
		result["dump"] = dump
		result["dumped_at"] = info.ModTime().UTC()
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		fail(err)
	}
}

func copyFile(from, to string) error {
	data, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	return os.WriteFile(to, data, 0o600)
}

func appendLine(path, line string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = fmt.Fprintln(file, line)
	return err
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "hub-server:", err)
	os.Exit(1)
}
