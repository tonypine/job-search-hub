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
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"

	"github.com/tonypine/job-search-hub/server/internal/abandonedruns"
	"github.com/tonypine/job-search-hub/server/internal/api"
	"github.com/tonypine/job-search-hub/server/internal/boarddiscovery"
	"github.com/tonypine/job-search-hub/server/internal/boardfinder"
	"github.com/tonypine/job-search-hub/server/internal/boardpoller"
	"github.com/tonypine/job-search-hub/server/internal/chatcompletions"
	"github.com/tonypine/job-search-hub/server/internal/claudeprint"
	"github.com/tonypine/job-search-hub/server/internal/comparisons"
	"github.com/tonypine/job-search-hub/server/internal/conversationtriage"
	"github.com/tonypine/job-search-hub/server/internal/cvdrafts"
	"github.com/tonypine/job-search-hub/server/internal/cvpdfs"
	"github.com/tonypine/job-search-hub/server/internal/cvscreens"
	"github.com/tonypine/job-search-hub/server/internal/databasebackup"
	"github.com/tonypine/job-search-hub/server/internal/exchangerates"
	"github.com/tonypine/job-search-hub/server/internal/feedpoller"
	"github.com/tonypine/job-search-hub/server/internal/followupreminders"
	"github.com/tonypine/job-search-hub/server/internal/freshmatches"
	"github.com/tonypine/job-search-hub/server/internal/gmailwatch"
	"github.com/tonypine/job-search-hub/server/internal/google"
	"github.com/tonypine/job-search-hub/server/internal/hiringthread"
	"github.com/tonypine/job-search-hub/server/internal/hubevents"
	"github.com/tonypine/job-search-hub/server/internal/interviewpacks"
	"github.com/tonypine/job-search-hub/server/internal/jobalerts"
	"github.com/tonypine/job-search-hub/server/internal/jobboards"
	"github.com/tonypine/job-search-hub/server/internal/jobbriefs"
	"github.com/tonypine/job-search-hub/server/internal/jobfacts"
	"github.com/tonypine/job-search-hub/server/internal/mailactions"
	"github.com/tonypine/job-search-hub/server/internal/mailtriage"
	"github.com/tonypine/job-search-hub/server/internal/marketgaps"
	"github.com/tonypine/job-search-hub/server/internal/mcptools"
	"github.com/tonypine/job-search-hub/server/internal/modelqueue"
	"github.com/tonypine/job-search-hub/server/internal/modelrouter"
	"github.com/tonypine/job-search-hub/server/internal/modelruntime"
	"github.com/tonypine/job-search-hub/server/internal/modelwork"
	"github.com/tonypine/job-search-hub/server/internal/postingtexts"
	"github.com/tonypine/job-search-hub/server/internal/push"
	"github.com/tonypine/job-search-hub/server/internal/startupsgallery"
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
	// databaseBackupCheckInterval is how often the server checks whether
	// the night's database dump is due.
	databaseBackupCheckInterval = 15 * time.Minute
	// jobBriefInterval picks up jobs whose facts were read, or whose brief went
	// stale, since the last pass.
	jobBriefInterval = 5 * time.Minute
	// fullBriefCheckInterval is how often the server checks whether the
	// night's full briefs are due.
	fullBriefCheckInterval = 15 * time.Minute
	// cvDraftInterval picks up jobs pursued since the last pass, to draft
	// their CVs.
	cvDraftInterval = 5 * time.Minute
	// cvScreenInterval picks up tailored CVs drafted or edited since the last
	// pass, to screen them as a recruiter would.
	cvScreenInterval = 5 * time.Minute
	// marketGapsCheckInterval is how often the server checks whether the
	// day's market gaps are due.
	marketGapsCheckInterval = time.Hour
	// interviewPackInterval picks up jobs pursued, or whose knowledge base
	// changed, since the last pass, to prepare their interview packs.
	interviewPackInterval = 10 * time.Minute
	// hiringThreadInterval paces the reading of Hacker News' monthly "Who is
	// hiring?" thread, whose comments keep arriving for days.
	hiringThreadInterval = 6 * time.Hour
	// galleryCheckInterval is how often the server checks whether a week
	// has passed since startups.gallery's remote list was last read.
	galleryCheckInterval = 6 * time.Hour
	// postingTextInterval picks up the jobs alerts listed since the last
	// pass, and the ones whose wait for their company's board is over.
	postingTextInterval = 30 * time.Minute
	// followUpReminderInterval is how often the server checks for follow-ups
	// fallen due since the last pass.
	followUpReminderInterval = 15 * time.Minute
	// freshMatchInterval is how often the server checks for jobs briefed a
	// strong match while still fresh, as briefs are written every few minutes.
	freshMatchInterval = 5 * time.Minute
	// abandonedRunInterval is how often the server closes the agent runs
	// whose token expired before they reported an end.
	abandonedRunInterval = 5 * time.Minute
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
	if len(os.Args) > 1 && os.Args[1] == "database" {
		// A command run from a terminal: only warnings and errors are logged,
		// in text, and the progress goes to stdout.
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})))
		if err := runDatabaseCommand(os.Args[2:], os.Getenv, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "hub-server database:", err)
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

	// Closing it is the last thing the server does: the HTTP server and the
	// workers stop first, then the pool closes, then Postgres stops.
	database, err := openDatabase(ctx, settings)
	if err != nil {
		return err
	}
	defer database.Close()

	hub := store.New(database.pool)
	if err := seedAgentPrompts(ctx, hub, settings.agentPromptsDir); err != nil {
		return err
	}
	verifier := tokens.NewVerifier(settings.ownerToken, hub)
	requireOwner := auth.RequireBearerToken(verifier, &auth.RequireBearerTokenOptions{
		Scopes: []string{tokens.ScopeOwner}, AllowMissingExpiration: true,
	})

	rates := exchangerates.NewCache(exchangerates.DefaultAPIBase)
	routes := http.NewServeMux()
	routes.Handle("GET /v1/health", api.NewHealthHandler(database.pool))
	api.RegisterAgentRunRoutes(routes, hub, requireOwner)
	go abandonedruns.NewCloser(hub).Run(ctx, abandonedRunInterval)
	api.RegisterCompanyRoutes(routes, hub, requireOwner)
	api.RegisterProfileRoutes(routes, hub, requireOwner)
	api.RegisterPipelineRoutes(routes, hub, requireOwner)
	api.RegisterJobCriteriaRoutes(routes, hub, requireOwner)
	api.RegisterConnectionRoutes(routes, hub, requireOwner)
	api.RegisterRecruiterRoutes(routes, hub, rates, requireOwner)
	api.RegisterPeopleRoutes(routes, hub, rates, requireOwner)
	api.RegisterCompanySuggestionRoutes(routes, hub, rates, requireOwner)
	api.RegisterLinkedInProfileRoutes(routes, hub, rates, requireOwner)
	api.RegisterArtifactRoutes(routes, hub, requireOwner)
	api.RegisterAgentPromptRoutes(routes, hub, requireOwner)
	api.RegisterApplicationAnswerRoutes(routes, hub, requireOwner)
	api.RegisterProfileEntryRoutes(routes, hub, requireOwner)
	api.RegisterTaskRunRoutes(routes, hub, requireOwner)
	api.RegisterModelRoutingRoutes(routes, hub, settings.modelsDir, requireOwner)
	api.RegisterWarmPathRoutes(routes, hub, requireOwner)
	api.RegisterDeviceRoutes(routes, hub, requireOwner)
	broadcaster := hubevents.NewBroadcaster()
	updateRecorder := hubevents.NewRecorder(hub, broadcaster)
	api.RegisterUpdateRoutes(routes, hub, updateRecorder, requireOwner)
	api.RegisterTaskRoutes(routes, hub, updateRecorder, requireOwner)
	api.RegisterEventRoutes(routes, hub, broadcaster, requireOwner)
	if sender := makePushSender(ctx, settings); sender != nil {
		go push.NewNotifier(hub, sender, broadcaster).Run(ctx)
	}
	go followupreminders.NewReminder(hub, updateRecorder).Run(ctx, followUpReminderInterval)
	go freshmatches.NewTeller(hub, updateRecorder).Run(ctx, freshMatchInterval)
	api.RegisterClaudeSessionRoutes(routes, hub, rates, requireOwner)
	boards := jobboards.NewVerifier()
	boards.SearchTerms = func(ctx context.Context) []string {
		saved, err := hub.GetJobCriteria(ctx)
		if err != nil {
			return nil
		}
		return saved.Criteria.SearchTerms
	}
	api.RegisterJobRoutes(routes, hub, boards, rates, requireOwner)
	googleClient := makeGoogleClient(settings, hub)
	api.RegisterGoogleRoutes(routes, hub, googleClient, requireOwner)
	boardPoller := boardpoller.New(hub, boards)
	ownerTools := mcptools.NewServer(hub, boards, boardPoller, rates)
	routes.Handle("/mcp", mcptools.NewHandler(ownerTools, mcptools.NewAgentServer(hub, boards, boardPoller, rates), verifier))

	if settings.boardPollInterval > 0 {
		go boardPoller.Run(ctx, settings.boardPollInterval)
	}
	if settings.feedPollInterval > 0 {
		go feedpoller.New(hub, boards).Run(ctx, settings.feedPollInterval)
	}
	if settings.boardSearchInterval > 0 {
		go boardfinder.New(hub, boards, rates).Run(ctx, settings.boardSearchInterval)
	}
	if settings.boardDiscoveryInterval > 0 {
		go boarddiscovery.New(hub, boards).Run(ctx, settings.boardDiscoveryInterval)
	}
	go startupsgallery.NewReader(hub, boards).Run(ctx, galleryCheckInterval)
	// Alert jobs no board gave text to get it from Google for Jobs when a
	// JSearch key is set; without one they only get the reason they have none.
	var jsearch *postingtexts.JSearch
	if settings.jsearchAPIKey != "" {
		jsearch = postingtexts.NewJSearch(settings.jsearchURL, settings.jsearchAPIKey)
		slog.Info("Google for Jobs search on", "monthly requests", settings.jsearchMonthlyRequests)
	}
	go postingtexts.New(hub, jsearch, settings.jsearchMonthlyRequests).Run(ctx, postingTextInterval)
	// Each kind of task runs on the model it's routed to. The settings' model
	// server seeds the routes of a fresh database; routes changed since stay.
	// Errors return rather than exit, so the database closes on the way out.
	if err := hub.EnsureDefaultTaskRoutes(ctx, settings.jobFactsModelURL, settings.jobFactsModel, store.RoutedTaskKinds); err != nil {
		return fmt.Errorf("seed the task routes: %w", err)
	}
	taskRoutes, err := hub.ListTaskRoutes(ctx)
	if err != nil {
		return fmt.Errorf("read the task routes: %w", err)
	}
	// The hub's own model runtime starts llama-server only when a route to
	// it has work, and stops it when idle.
	logDirectory := ""
	if home, err := os.UserHomeDir(); err == nil {
		logDirectory = filepath.Join(home, "Library", "Logs", "JobSearchHub")
		_ = os.MkdirAll(logDirectory, 0o755)
	}
	modelRuntime := modelruntime.New(modelruntime.Settings{
		LlamaServer: settings.llamaServer, ModelsDir: settings.modelsDir, Port: settings.runtimePort,
		IdleTimeout: settings.runtimeIdleTimeout, LogPath: filepath.Join(logDirectory, "llama-server.log"),
	})
	go modelRuntime.Run(ctx)
	// Model calls take turns through one queue, which starts paused if the
	// owner left it paused.
	paused, err := hub.GetModelWorkPaused(ctx)
	if err != nil {
		return fmt.Errorf("read whether model work is paused: %w", err)
	}
	modelQueue := modelqueue.New(paused, modelqueue.Settings{
		GetLoadedModel: func() string { return modelRuntime.Status().Model },
		UnloadModel:    modelRuntime.Unload,
	})
	modelWork := &modelwork.Controls{Hub: hub, Queue: modelQueue, Runtime: modelRuntime}

	recordTaskRun := func(ctx context.Context, record chatcompletions.RunRecord) {
		if _, err := hub.RecordTaskRun(ctx, store.NewTaskRun{
			Kind: record.Kind, SubjectID: record.Task.SubjectID, BaseURL: record.BaseURL, Model: record.Model,
			PromptID: record.Task.PromptID, PromptVersion: record.Task.PromptVersion, InputHash: record.InputHash, Output: record.Output,
			PromptTokens: record.PromptTokens, CompletionTokens: record.CompletionTokens, StartedAt: record.StartedAt,
			Duration: record.Duration, Outcome: record.Outcome, Error: record.Error,
		}); err != nil {
			slog.Warn("record a task run", "kind", record.Kind, "error", err)
		}
	}
	var modelClient *modelrouter.Router
	if len(taskRoutes) > 0 {
		modelClient = modelrouter.New(hub)
		modelClient.Runtime = modelRuntime
		modelClient.Queue = modelQueue
		modelClient.RecordRun = recordTaskRun
	}
	if modelClient != nil {
		go conversationtriage.NewClassifier(hub, modelClient).Run(ctx, conversationTriageInterval)
	}
	var cvPrinter *cvpdfs.Printer
	if _, err := os.Stat(settings.cvPrintCommand); err != nil {
		slog.Warn("CV printing off: no hub-cvprint command", "command", settings.cvPrintCommand)
	} else {
		cvPrinter = cvpdfs.NewPrinter(hub, settings.cvPrintCommand, settings.cvFolder)
	}
	var fullBriefs *jobbriefs.Writer
	var cvDrafter *cvdrafts.Drafter
	var newClaudeClient func(model string) comparisons.ModelClient
	if modelClient != nil {
		briefWriter := jobbriefs.NewWriter(hub, modelClient, rates)
		go briefWriter.Run(ctx, jobBriefInterval)
		go cvscreens.NewScreener(hub, modelClient).Run(ctx, cvScreenInterval)
		go marketgaps.NewAnalyzer(hub, modelClient, rates).Run(ctx, marketGapsCheckInterval)
		go interviewpacks.NewPreparer(hub, modelClient).Run(ctx, interviewPackInterval)
		go hiringthread.NewReader(hub, modelClient).Run(ctx, hiringThreadInterval)
		if claudeBinary, err := exec.LookPath(settings.claudeBinary); err != nil {
			slog.Warn("full briefs off: the Claude CLI isn't found", "claude", settings.claudeBinary)
		} else if err := os.MkdirAll(settings.claudeFolder, 0o700); err != nil {
			slog.Warn("full briefs off: no folder to run Claude in", "error", err)
		} else {
			claude := &claudeprint.Client{Binary: claudeBinary, Directory: settings.claudeFolder, Model: settings.fullBriefModel, RecordRun: recordTaskRun}
			briefWriter.FullClient = claude
			fullBriefs = briefWriter
			cvDrafter = cvdrafts.NewDrafter(hub, claude)
			cvDrafter.Rates = rates
			if cvPrinter != nil {
				cvDrafter.Printer = cvPrinter
			}
			go cvDrafter.Run(ctx, cvDraftInterval)
			go briefWriter.RunNightly(ctx, fullBriefCheckInterval)
			newClaudeClient = func(model string) comparisons.ModelClient {
				return &claudeprint.Client{Binary: claudeBinary, Directory: settings.claudeFolder, Model: model, RecordRun: recordTaskRun}
			}
			slog.Info("full briefs on", "model", settings.fullBriefModel)
		}
	}
	if fullBriefs != nil {
		api.RegisterJobBriefRoutes(routes, hub, fullBriefs, requireOwner)
		mcptools.AddJobBriefTools(ownerTools, fullBriefs, hub)
	} else {
		api.RegisterJobBriefRoutes(routes, hub, nil, requireOwner)
	}
	if modelClient != nil {
		extractor := jobfacts.NewExtractor(hub, modelClient)
		modelWork.Facts = extractor
		if settings.jobFactsInterval > 0 {
			go extractor.Run(ctx, settings.jobFactsInterval)
			slog.Info("job facts reading on", "every", settings.jobFactsInterval.String(), "paused", paused)
		}
	}
	api.RegisterModelWorkRoutes(routes, modelWork, requireOwner)
	if modelClient != nil {
		comparisonRunner := comparisons.NewRunner(hub, modelClient)
		comparisonRunner.NewClaudeClient = newClaudeClient
		go comparisonRunner.RunUnfinished(ctx)
		api.RegisterComparisonRoutes(routes, hub, comparisonRunner, requireOwner)
	} else {
		api.RegisterComparisonRoutes(routes, hub, nil, requireOwner)
	}
	api.RegisterDecisionRoutes(routes, hub, rates, requireOwner)
	api.RegisterMarketGapRoutes(routes, hub, requireOwner)
	api.RegisterInterviewPackRoutes(routes, hub, requireOwner)
	if cvDrafter != nil {
		mcptools.AddCVTools(ownerTools, cvDrafter)
	}
	// Either can be off; a nil pointer passed as the interface wouldn't read as off.
	switch {
	case cvDrafter != nil && cvPrinter != nil:
		api.RegisterCVRoutes(routes, hub, cvDrafter, cvPrinter, requireOwner)
	case cvDrafter != nil:
		api.RegisterCVRoutes(routes, hub, cvDrafter, nil, requireOwner)
	case cvPrinter != nil:
		api.RegisterCVRoutes(routes, hub, nil, cvPrinter, requireOwner)
	default:
		api.RegisterCVRoutes(routes, hub, nil, nil, requireOwner)
	}
	mcptools.AddModelWorkTools(ownerTools, modelWork)

	var mailBackfiller api.MailBackfiller
	if googleClient != nil && settings.gmailSubscription != "" {
		watcher := gmailwatch.New(hub, googleClient, settings.gmailTopic, settings.gmailSubscription)
		mailBackfiller = watcher
		if modelClient != nil {
			classifier := mailtriage.NewClassifier(hub, googleClient, modelClient)
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

	if pgDump, err := exec.LookPath(database.pgDump); err != nil {
		slog.Warn("database backups off: pg_dump not found", "pg_dump", database.pgDump)
	} else {
		go databasebackup.NewDumper(database.url, settings.backupsDir, pgDump).Run(ctx, databaseBackupCheckInterval)
		slog.Info("database backups on", "folder", settings.backupsDir)
	}

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
	case <-database.Exited():
		// launchd restarts the server, and the server its Postgres.
		stop()
		return fmt.Errorf("the database stopped by itself: %w", database.ExitError())
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

// makePushSender reads the Firebase service account, or returns nil, leaving
// pushes off, when there is none or it can't be read.
func makePushSender(ctx context.Context, settings config) *push.Sender {
	if settings.firebaseServiceAccountFile == "" {
		return nil
	}
	serviceAccount, err := os.ReadFile(settings.firebaseServiceAccountFile)
	if errors.Is(err, os.ErrNotExist) {
		slog.Info("pushes stay off: no Firebase service account", "file", settings.firebaseServiceAccountFile)
		return nil
	}
	if err != nil {
		slog.Warn("pushes stay off: the Firebase service account can't be read", "file", settings.firebaseServiceAccountFile, "error", err)
		return nil
	}
	sender, err := push.NewSender(ctx, serviceAccount)
	if err != nil {
		slog.Warn("pushes stay off", "error", err)
		return nil
	}
	slog.Info("pushes on", "project", sender.ProjectID())
	return sender
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
