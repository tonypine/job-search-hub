package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const defaultHubURL = "http://localhost:8090"

type cliConfig struct {
	HubURL     string `json:"url"`
	OwnerToken string `json:"owner_token"`
}

// readConfig takes HUB_URL and HUB_OWNER_TOKEN from the environment, falling
// back to ~/.config/job-search-hub/config.json. The file holds the owner token,
// so it is refused when anyone but its owner can read it.
func readConfig(lookup func(string) string, homeDirectory string) (cliConfig, error) {
	config := cliConfig{HubURL: lookup("HUB_URL"), OwnerToken: lookup("HUB_OWNER_TOKEN")}

	path := filepath.Join(homeDirectory, ".config", "job-search-hub", "config.json")
	info, err := os.Stat(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return cliConfig{}, err
	case info.Mode().Perm()&0o077 != 0:
		return cliConfig{}, fmt.Errorf("%s is readable by others; run: chmod 600 %s", path, path)
	default:
		var fromFile cliConfig
		contents, err := os.ReadFile(path)
		if err != nil {
			return cliConfig{}, err
		}
		if err := json.Unmarshal(contents, &fromFile); err != nil {
			return cliConfig{}, fmt.Errorf("parse %s: %w", path, err)
		}
		if config.HubURL == "" {
			config.HubURL = fromFile.HubURL
		}
		if config.OwnerToken == "" {
			config.OwnerToken = fromFile.OwnerToken
		}
	}

	if config.HubURL == "" {
		config.HubURL = defaultHubURL
	}
	if config.OwnerToken == "" {
		return cliConfig{}, fmt.Errorf("set HUB_OWNER_TOKEN, or owner_token in %s", path)
	}
	return config, nil
}
