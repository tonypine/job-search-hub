// Package serverenv reads server.env, the settings file every installed
// version of the server shares, in ~/.config/job-search-hub.
package serverenv

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Path is server.env in home.
func Path(home string) string {
	return filepath.Join(home, ".config", "job-search-hub", "server.env")
}

// Parse reads NAME=value lines, as a shell sources them for the values the
// hub writes: blank lines and # comments are skipped, an export prefix is
// allowed, and one pair of quotes around a value is dropped. It expands
// nothing.
func Parse(r io.Reader) ([][2]string, error) {
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

// Read is the file at path as a map, the last line naming a variable
// winning; a missing file is an empty map.
func Read(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	variables, err := Parse(file)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	values := make(map[string]string, len(variables))
	for _, variable := range variables {
		values[variable[0]] = variable[1]
	}
	return values, nil
}
