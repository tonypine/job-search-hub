// Command hub-server serves the job-search hub over one Postgres database.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/modelcontextprotocol/go-sdk/auth"

	"github.com/tonypine/job-search-hub/server/internal/api"
	"github.com/tonypine/job-search-hub/server/internal/boardpoller"
	"github.com/tonypine/job-search-hub/server/internal/chatcompletions"
	"github.com/tonypine/job-search-hub/server/internal/conversationtriage"
	"github.com/tonypine/job-search-hub/server/internal/exchangerates"
	"github.com/tonypine/job-search-hub/server/internal/feedpoller"
	"github.com/tonypine/job-search-hub/server/internal/gmailwatch"
	"github.com/tonypine/job-search-hub/server/internal/google"
	"github.com/tonypine/job-search-hub/server/internal/hubevents"
	"github.com/tonypine/job-search-hub/server/internal/jobalerts"
	"github.com/tonypine/job-search-hub/server/internal/jobboards"
	"github.com/tonypine/job-search-hub/server/internal/jobfacts"
	"github.com/tonypine/job-search-hub/server/internal/mailactions"
	"github.com/tonypine/job-search-hub/server/internal/mailtriage"
	"github.com/tonypine/job-search-hub/server/internal/mcptools"
	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/tokens"
)

const (
	readHeaderTimeout = 10 * time.Second
	shutdownTimeout   = 5 * time.Second
	// mailTriageInterval retries mail the model couldn't read; new mail
	// nudges a pass at once.
	mailTriageInterval = 5 * time.Minute
	// conversationTriageInterval picks up LinkedIn conversations imported
	// since the last pass, or that the model couldn't read.
	conversationTriageInterval = 10 * time.Minute
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

	hub := store.New(database)
	if err := seedAgentPrompts(ctx, hub, settings.agentPromptsDir); err != nil {
		return err
	}
	verifier := tokens.NewVerifier(settings.ownerToken, hub)
	requireOwner := auth.RequireBearerToken(verifier, &auth.RequireBearerTokenOptions{
		Scopes: []string{tokens.ScopeOwner}, AllowMissingExpiration: true,
	})

	rates := exchangerates.NewCache(exchangerates.DefaultAPIBase)
	routes := http.NewServeMux()
	routes.Handle("GET /v1/health", api.NewHealthHandler(database))
	api.RegisterAgentRunRoutes(routes, hub, requireOwner)
	api.RegisterCompanyRoutes(routes, hub, requireOwner)
	api.RegisterProfileRoutes(routes, hub, requireOwner)
	api.RegisterPipelineRoutes(routes, hub, requireOwner)
	api.RegisterJobCriteriaRoutes(routes, hub, requireOwner)
	api.RegisterConnectionRoutes(routes, hub, requireOwner)
	api.RegisterRecruiterRoutes(routes, hub, rates, requireOwner)
	api.RegisterCompanySuggestionRoutes(routes, hub, rates, requireOwner)
	api.RegisterLinkedInProfileRoutes(routes, hub, rates, requireOwner)
	api.RegisterArtifactRoutes(routes, hub, requireOwner)
	broadcaster := hubevents.NewBroadcaster()
	updateRecorder := hubevents.NewRecorder(hub, broadcaster)
	api.RegisterUpdateRoutes(routes, hub, updateRecorder, requireOwner)
	api.RegisterEventRoutes(routes, hub, broadcaster, requireOwner)
	api.RegisterClaudeSessionRoutes(routes, hub, rates, requireOwner)
	boards := jobboards.NewVerifier()
	api.RegisterJobRoutes(routes, hub, boards, rates, requireOwner)
	googleClient := makeGoogleClient(settings, hub)
	api.RegisterGoogleRoutes(routes, hub, googleClient, requireOwner)
	boardPoller := boardpoller.New(hub, boards)
	routes.Handle("/mcp", mcptools.NewHandler(mcptools.NewServer(hub, boards, boardPoller), verifier))

	if settings.boardPollInterval > 0 {
		go boardPoller.Run(ctx, settings.boardPollInterval)
	}
	if settings.feedPollInterval > 0 {
		go feedpoller.New(hub, boards).Run(ctx, settings.feedPollInterval)
	}
	var modelClient *chatcompletions.Client
	if settings.jobFactsModelURL != "" {
		modelClient = chatcompletions.NewClient(settings.jobFactsModelURL)
	}
	if modelClient != nil {
		go conversationtriage.NewClassifier(hub, modelClient, settings.jobFactsModel).Run(ctx, conversationTriageInterval)
	}
	if modelClient != nil && settings.jobFactsInterval > 0 {
		extractor := jobfacts.NewExtractor(hub, modelClient, settings.jobFactsModel)
		go extractor.Run(ctx, settings.jobFactsInterval)
		slog.Info("job facts reading on", "model", settings.jobFactsModel, "every", settings.jobFactsInterval.String())
	}

	var mailBackfiller api.MailBackfiller
	if googleClient != nil && settings.gmailSubscription != "" {
		watcher := gmailwatch.New(hub, googleClient, settings.gmailTopic, settings.gmailSubscription)
		mailBackfiller = watcher
		if modelClient != nil {
			classifier := mailtriage.NewClassifier(hub, googleClient, modelClient, settings.jobFactsModel)
			mailHandler := mailactions.NewHandler(hub, updateRecorder)
			alertReader := jobalerts.NewReader(hub, googleClient)
			watcher.OnMailRecorded = func() {
				classifier.Nudge()
				mailHandler.Nudge()
			}
			classifier.OnClassified = func() {
				mailHandler.Nudge()
				alertReader.Nudge()
			}
			go classifier.Run(ctx, mailTriageInterval)
			go mailHandler.Run(ctx, mailTriageInterval)
			go alertReader.Run(ctx, mailTriageInterval)
		}
		go watcher.Run(ctx)
		slog.Info("gmail changes on", "topic", settings.gmailTopic)
	}
	api.RegisterMailRoutes(routes, hub, mailBackfiller, requireOwner)

	server := &http.Server{
		Addr:              settings.address,
		Handler:           routes,
		ReadHeaderTimeout: readHeaderTimeout,
		// Requests end with the server, so open event streams don't hold up
		// a shutdown.
		BaseContext: func(net.Listener) context.Context { return ctx },
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

// makeGoogleClient reads the hub's Google OAuth client, or returns nil,
// leaving Google off, when there is none or it can't be read.
func makeGoogleClient(settings config, hub *store.Store) *google.Client {
	if settings.googleClientFile == "" {
		return nil
	}
	clientJSON, err := os.ReadFile(settings.googleClientFile)
	if err != nil {
		slog.Warn("Google stays off: the OAuth client file can't be read", "file", settings.googleClientFile, "error", err)
		return nil
	}
	client, err := google.NewClient(clientJSON, settings.publicURL+"/v1/google/callback", hub)
	if err != nil {
		slog.Warn("Google stays off", "error", err)
		return nil
	}
	slog.Info("Google sign-in on", "redirect", settings.publicURL+"/v1/google/callback")
	return client
}

// seedAgentPrompts gives each prompt kind that has no version yet its first
// one, from the private seed folder.
func seedAgentPrompts(ctx context.Context, hub *store.Store, folder string) error {
	if folder == "" {
		return nil
	}
	if _, err := os.Stat(folder); err != nil {
		slog.Warn("no agent prompt seed folder; kinds without a prompt stay without one", "folder", folder, "error", err)
		return nil
	}
	seeded, err := hub.SeedAgentPrompts(ctx, os.DirFS(folder))
	if err != nil {
		return fmt.Errorf("seed the agent prompts from %s: %w", folder, err)
	}
	if len(seeded) > 0 {
		slog.Info("seeded agent prompts", "kinds", seeded, "folder", folder)
	}
	return nil
}
