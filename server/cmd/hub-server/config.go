package main

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	minimumOwnerTokenLength  = 32
	defaultBoardPollInterval = time.Hour
	defaultJobFactsModel     = "qwen/qwen3.5-9b"
	defaultJobFactsInterval  = 10 * time.Minute
	defaultFeedPollInterval  = 3 * time.Hour
	defaultPublicURL         = "http://localhost:8090"
)

type config struct {
	address           string
	databaseURL       string
	ownerToken        string
	boardPollInterval time.Duration
	feedPollInterval  time.Duration
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
	if parsed.databaseURL == "" {
		missing = append(missing, "HUB_DATABASE_URL")
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
	return parsed, nil
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
