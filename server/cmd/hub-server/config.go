package main

import (
	"errors"
	"fmt"
	"strings"
)

const minimumOwnerTokenLength = 32

type config struct {
	address     string
	databaseURL string
	ownerToken  string
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
	return parsed, nil
}
