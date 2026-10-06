package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// serverEnvPath is the settings file every version of the server shares,
// outside the app's bundle: launchd starts the server from the bundle with
// none of them, so the server reads the file itself.
func serverEnvPath(home string) string {
	return filepath.Join(home, ".config", "job-search-hub", "server.env")
}

// prepareEnvironment gives the server its settings from server.env and the
// PATH its tools are found on, before anything reads the environment.
func prepareEnvironment() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("find the home folder: %w", err)
	}
	if err := loadEnvFile(serverEnvPath(home), os.LookupEnv, os.Setenv); err != nil {
		return err
	}
	return os.Setenv("PATH", makeSearchPath(os.Getenv("PATH"), home))
}

// loadEnvFile sets each variable path names that the environment doesn't
// already have, so a variable set by hand wins over the file. A missing file
// sets nothing.
func loadEnvFile(path string, lookup func(string) (string, bool), set func(string, string) error) error {
	file, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open the server's settings: %w", err)
	}
	defer file.Close()
	variables, err := parseEnvFile(file)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	for _, variable := range variables {
		if _, ok := lookup(variable[0]); ok {
			continue
		}
		if err := set(variable[0], variable[1]); err != nil {
			return fmt.Errorf("set %s from %s: %w", variable[0], path, err)
		}
	}
	return nil
}

// parseEnvFile reads NAME=value lines, as a shell sources them for the
// values the hub writes: blank lines and # comments are skipped, an export
// prefix is allowed, and one pair of quotes around a value is dropped. It
// expands nothing.
func parseEnvFile(r io.Reader) ([][2]string, error) {
	var variables [][2]string
	scanner := bufio.NewScanner(r)
	for number := 1; scanner.Scan(); number++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		name, value, found := strings.Cut(line, "=")
		name = strings.TrimSpace(name)
		if !found || name == "" || strings.ContainsAny(name, " \t") {
			return nil, fmt.Errorf("line %d isn't NAME=value", number)
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
			value = value[1 : len(value)-1]
		}
		variables = append(variables, [2]string{name, value})
	}
	return variables, scanner.Err()
}

// makeSearchPath is the PATH the server runs with: Homebrew's tools
// (llama-server), the user's own (Claude Code), then the system's, followed
// by whatever else the inherited one has. launchd's own PATH holds only the
// system's.
func makeSearchPath(inherited, home string) string {
	path := []string{"/opt/homebrew/bin", "/usr/local/bin", filepath.Join(home, ".local", "bin"), "/usr/bin", "/bin", "/usr/sbin", "/sbin"}
	seen := map[string]bool{}
	for _, folder := range path {
		seen[folder] = true
	}
	for folder := range strings.SplitSeq(inherited, string(os.PathListSeparator)) {
		if folder != "" && !seen[folder] {
			seen[folder] = true
			path = append(path, folder)
		}
	}
	return strings.Join(path, string(os.PathListSeparator))
}

// logToFile sends the server's output, and its children's, to the file path
// names, appending. launchd starts the server from the app's bundle, whose
// plist can't name a file in the user's home, so it passes the log as
// HUB_LOG_FILE with a leading ~/. Empty leaves the output where it is.
func logToFile(path string) error {
	if path == "" {
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("find the home folder: %w", err)
	}
	path = expandHome(path, home)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create the log's folder: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open the log: %w", err)
	}
	defer file.Close()
	for _, output := range []*os.File{os.Stdout, os.Stderr} {
		if err := unix.Dup2(int(file.Fd()), int(output.Fd())); err != nil {
			return fmt.Errorf("send the output to %s: %w", path, err)
		}
	}
	return nil
}

// expandHome replaces a leading ~/ in path with home.
func expandHome(path, home string) string {
	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		return filepath.Join(home, rest)
	}
	return path
}
