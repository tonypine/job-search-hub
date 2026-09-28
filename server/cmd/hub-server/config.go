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
)

type config struct {
	address           string
	databaseURL       string
	ownerToken        string
	boardPollInterval time.Duration
	feedPollInterval  time.Duration
	// jobFactsModelURL is the chat-completions API root of the model that
	// reads job facts; empty turns reading off.
	jobFactsModelURL string
	jobFactsModel    string
	jobFactsInterval time.Duration
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
