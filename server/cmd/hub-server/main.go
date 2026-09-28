// Command hub-server serves the job-search hub over one Postgres database.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tonypine/job-search-hub/server/internal/api"
	"github.com/tonypine/job-search-hub/server/internal/store"
)

const (
	readHeaderTimeout = 10 * time.Second
	shutdownTimeout   = 5 * time.Second
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		if err := runHealthcheck(os.Getenv("HUB_ADDR")); err != nil {
			slog.Error("healthcheck failed", "error", err)
			os.Exit(1)
		}
		return
	}
	if err := run(); err != nil {
		slog.Error("hub-server stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	settings, err := parseEnvironment(os.Getenv)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	database, err := pgxpool.New(ctx, settings.databaseURL)
	if err != nil {
		return fmt.Errorf("configure the database pool: %w", err)
	}
	defer database.Close()
	if err := store.Migrate(ctx, database); err != nil {
		return err
	}

	routes := http.NewServeMux()
	routes.Handle("GET /v1/health", api.NewHealthHandler(database))

	server := &http.Server{
		Addr:              settings.address,
		Handler:           routes,
		ReadHeaderTimeout: readHeaderTimeout,
	}
	serveResult := make(chan error, 1)
	go func() { serveResult <- server.ListenAndServe() }()
	slog.Info("hub-server listening", "address", settings.address)

	select {
	case err := <-serveResult:
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("shut down: %w", err)
	}
	slog.Info("hub-server shut down")
	return nil
}
