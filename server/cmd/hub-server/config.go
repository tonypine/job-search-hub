package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/postingtexts"
)

const (
	minimumOwnerTokenLength       = 32
	defaultBoardPollInterval      = 15 * time.Minute
	defaultJobFactsModel          = "qwen/qwen3.5-9b"
	defaultJobFactsInterval       = time.Minute
	defaultRuntimePort            = 8095
	defaultRuntimeIdleTimeout     = 10 * time.Minute
	defaultFeedPollInterval       = time.Hour
	defaultBoardSearchInterval    = 30 * time.Minute
	defaultBoardDiscoveryInterval = 6 * time.Hour
	defaultPublicURL              = "http://localhost:8090"
)

type config struct {
	address string
	// databaseURL is a Postgres the server uses as it is; empty, the server
	// runs its own, from the newest engine in postgresEngines, with its
	// clusters in postgresDir.
	databaseURL       string
	postgresEngines   string
	postgresDir       string
	ownerToken        string
	boardPollInterval time.Duration
	feedPollInterval  time.Duration
	// boardSearchInterval paces the search for the boards of the employers
	// behind good and unclear feed jobs.
	boardSearchInterval time.Duration
	// boardDiscoveryInterval paces the bulk discovery of boards from
	// Common Crawl's index.
	boardDiscoveryInterval time.Duration
	// jobFactsModelURL and jobFactsModel seed the task routes of a database
	// that has none: the chat-completions API root and model every routed
	// task starts on. Empty leaves a fresh database without model work.
	jobFactsModelURL string
	jobFactsModel    string
	jobFactsInterval time.Duration
	// googleClientFile is the OAuth client Google issued for the hub; empty
	// leaves Google off.
	googleClientFile string
	// publicURL is where the owner's browser reaches the hub, for Google's
	// redirect after sign-in.
	publicURL string
	// gmailTopic and gmailSubscription are the full Pub/Sub names Gmail
	// announces mailbox changes through; empty leaves listening off.
	gmailTopic        string
	gmailSubscription string
	// agentPromptsDir holds the prompts a fresh database starts with, kept
	// out of the repo; empty leaves seeding off.
	agentPromptsDir string
	// firebaseServiceAccountFile is the key the hub pushes to phones with;
	// empty, or no file there, leaves pushes off.
	firebaseServiceAccountFile string
	// llamaServer, modelsDir, runtimePort and runtimeIdleTimeout set up the hub's
	// own model runtime: the llama-server binary, the folder of GGUF files
	// routes can name, its port, and how long an idle model stays loaded.
	llamaServer        string
	modelsDir          string
	runtimePort        int
	runtimeIdleTimeout time.Duration
	// backupsDir is the private folder nightly database dumps go to, and
	// pgDump the pg_dump binary that writes them; empty, the engine's own
	// when the server runs the database, or pg_dump from the PATH.
	backupsDir string
	pgDump     string
	// claudeBinary is the Claude Code CLI full briefs are written with,
	// fullBriefModel the model it runs, and claudeFolder the empty folder
	// it runs in, so no project's instructions reach the model.
	claudeBinary   string
	fullBriefModel string
	claudeFolder   string
	// cvPrintCommand is the hub-cvprint command CVs print to PDF with, and
	// cvFolder where the PDFs go, one folder per job.
	cvPrintCommand string
	cvFolder       string
	// jsearchAPIKey lets alert jobs no board has get their text from Google
	// for Jobs, through JSearch at jsearchURL, up to jsearchMonthlyRequests
	// requests a month; empty leaves the search off.
	jsearchAPIKey          string
	jsearchURL             string
	jsearchMonthlyRequests int
}

// parseEnvironment reads the server's settings through lookup, which is
// os.Getenv in production. Every missing variable is reported at once, so a
// fresh setup fails with the whole list rather than one name per restart.
func parseEnvironment(lookup func(string) string) (config, error) {
	parsed := config{
		address:     lookup("HUB_ADDR"),
		databaseURL: lookup("HUB_DATABASE_URL"),
		ownerToken:  lookup("HUB_OWNER_TOKEN"),
	}

	var missing []string
	if parsed.address == "" {
		missing = append(missing, "HUB_ADDR")
	}
	if parsed.ownerToken == "" {
		missing = append(missing, "HUB_OWNER_TOKEN")
	}
	if len(missing) > 0 {
		return config{}, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}

	if len(parsed.ownerToken) < minimumOwnerTokenLength {
		return config{}, errors.New("HUB_OWNER_TOKEN must be at least 32 characters; generate one with: openssl rand -hex 32")
	}

	// Intervals are Go durations such as 30m; 0 turns the work off.
	var err error
	if parsed.boardPollInterval, err = parseInterval(lookup, "HUB_BOARD_POLL_INTERVAL", defaultBoardPollInterval); err != nil {
		return config{}, err
	}
	if parsed.feedPollInterval, err = parseInterval(lookup, "HUB_FEED_POLL_INTERVAL", defaultFeedPollInterval); err != nil {
		return config{}, err
	}
	if parsed.boardSearchInterval, err = parseInterval(lookup, "HUB_BOARD_SEARCH_INTERVAL", defaultBoardSearchInterval); err != nil {
		return config{}, err
	}
	if parsed.boardDiscoveryInterval, err = parseInterval(lookup, "HUB_BOARD_DISCOVERY_INTERVAL", defaultBoardDiscoveryInterval); err != nil {
		return config{}, err
	}
	if parsed.jobFactsInterval, err = parseInterval(lookup, "HUB_JOB_FACTS_INTERVAL", defaultJobFactsInterval); err != nil {
		return config{}, err
	}
	parsed.googleClientFile = lookup("HUB_GOOGLE_OAUTH_CLIENT_FILE")
	parsed.publicURL = strings.TrimSuffix(lookup("HUB_PUBLIC_URL"), "/")
	if parsed.publicURL == "" {
		parsed.publicURL = defaultPublicURL
	}
	parsed.gmailTopic = lookup("HUB_GMAIL_PUBSUB_TOPIC")
	parsed.gmailSubscription = lookup("HUB_GMAIL_PUBSUB_SUBSCRIPTION")
	if (parsed.gmailTopic == "") != (parsed.gmailSubscription == "") {
		return config{}, errors.New("set both HUB_GMAIL_PUBSUB_TOPIC and HUB_GMAIL_PUBSUB_SUBSCRIPTION, or neither")
	}
	parsed.agentPromptsDir = lookup("HUB_AGENT_PROMPTS_DIR")
	parsed.firebaseServiceAccountFile = lookup("HUB_FIREBASE_SERVICE_ACCOUNT_FILE")
	parsed.jobFactsModelURL = lookup("HUB_JOB_FACTS_MODEL_URL")
	parsed.jobFactsModel = lookup("HUB_JOB_FACTS_MODEL")
	if parsed.jobFactsModel == "" {
		parsed.jobFactsModel = defaultJobFactsModel
	}
	parsed.llamaServer = lookup("HUB_LLAMA_SERVER")
	if parsed.llamaServer == "" {
		parsed.llamaServer = "llama-server"
	}
	parsed.modelsDir = lookup("HUB_MODELS_DIR")
	if parsed.modelsDir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			parsed.modelsDir = filepath.Join(home, "Library", "Application Support", "JobSearchHub", "models")
		}
	}
	parsed.runtimePort = defaultRuntimePort
	if raw := lookup("HUB_RUNTIME_PORT"); raw != "" {
		port, err := strconv.Atoi(raw)
		if err != nil || port <= 0 || port > 65535 {
			return config{}, fmt.Errorf("HUB_RUNTIME_PORT must be a port number, got %q", raw)
		}
		parsed.runtimePort = port
	}
	if parsed.runtimeIdleTimeout, err = parseInterval(lookup, "HUB_RUNTIME_IDLE_TIMEOUT", defaultRuntimeIdleTimeout); err != nil {
		return config{}, err
	}
	parsed.backupsDir = parseBackupsDir(lookup)
	parsed.pgDump = lookup("HUB_PG_DUMP")
	parsed.postgresEngines, parsed.postgresDir = parseDatabasePaths(lookup)
	parsed.claudeBinary = lookup("HUB_CLAUDE_BIN")
	if parsed.claudeBinary == "" {
		parsed.claudeBinary = "claude"
	}
	parsed.fullBriefModel = lookup("HUB_FULL_BRIEF_MODEL")
	if parsed.fullBriefModel == "" {
		parsed.fullBriefModel = "sonnet"
	}
	parsed.jsearchAPIKey = lookup("HUB_JSEARCH_API_KEY")
	parsed.jsearchURL = lookup("HUB_JSEARCH_URL")
	if parsed.jsearchURL == "" {
		parsed.jsearchURL = postingtexts.DefaultJSearchURL
	}
	parsed.jsearchMonthlyRequests = postingtexts.DefaultMonthlySearches
	if raw := lookup("HUB_JSEARCH_MONTHLY_REQUESTS"); raw != "" {
		requests, err := strconv.Atoi(raw)
		if err != nil || requests < 0 {
			return config{}, fmt.Errorf("HUB_JSEARCH_MONTHLY_REQUESTS must be a count of requests, got %q", raw)
		}
		parsed.jsearchMonthlyRequests = requests
	}
	parsed.cvPrintCommand = lookup("HUB_CV_PRINT_BIN")
	parsed.cvFolder = lookup("HUB_CV_FOLDER")
	if home, err := os.UserHomeDir(); err == nil {
		parsed.claudeFolder = filepath.Join(home, "Library", "Application Support", "JobSearchHub", "claude-print")
		if parsed.cvPrintCommand == "" {
			parsed.cvPrintCommand = filepath.Join(home, "Library", "Application Support", "JobSearchHub", "bin", "hub-cvprint")
		}
		if parsed.cvFolder == "" {
			parsed.cvFolder = filepath.Join(home, "Interview", "CVs")
		}
	}
	return parsed, nil
}

// parseDatabasePaths reads where the engines and the clusters of the database
// the server owns are, which the database commands need without the rest.
func parseDatabasePaths(lookup func(string) string) (engines, dir string) {
	engines = lookup("HUB_POSTGRES_ENGINES")
	if engines == "" {
		// The engines sit beside the server's own folder, wherever the
		// app puts it.
		if executable, err := os.Executable(); err == nil {
			engines = filepath.Join(filepath.Dir(executable), "..", "engines")
		}
	}
	dir = lookup("HUB_POSTGRES_DIR")
	if dir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			dir = filepath.Join(home, "Library", "Application Support", "JobSearchHub", "postgres")
		}
	}
	return engines, dir
}

// parseBackupsDir reads the folder the database dumps go to, which the
// import command needs without the rest.
func parseBackupsDir(lookup func(string) string) string {
	if dir := lookup("HUB_BACKUPS_DIR"); dir != "" {
		return dir
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, "Library", "Application Support", "JobSearchHub", "backups")
	}
	return ""
}

func parseInterval(lookup func(string) string, name string, fallback time.Duration) (time.Duration, error) {
	raw := lookup(name)
	if raw == "" {
		return fallback, nil
	}
	interval, err := time.ParseDuration(raw)
	if err != nil || interval < 0 {
		return 0, fmt.Errorf("%s must be a duration such as 30m or 0, got %q", name, raw)
	}
	return interval, nil
}
