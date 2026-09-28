// Package linkedinexport reads the files of a LinkedIn data export, which
// the owner downloads from LinkedIn and gives the hub.
package linkedinexport

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

// ErrNotConnectionsFile means the file has no Connections.csv header row.
var ErrNotConnectionsFile = errors.New(`not a LinkedIn Connections.csv: no row starting "First Name,Last Name"`)

// connectedOnLayouts are how the export writes the day a connection was made.
var connectedOnLayouts = []string{"02 Jan 2006", "2 Jan 2006"}

// ParseConnections reads a Connections.csv as LinkedIn writes it: notes
// first, then a header row, then one connection per row. A row without a
// profile URL can't be told apart from others and is skipped.
func ParseConnections(file io.Reader) (connections []store.NewConnection, skipped int, err error) {
	content, err := io.ReadAll(file)
	if err != nil {
		return nil, 0, err
	}
	content = bytes.TrimPrefix(content, []byte("\ufeff"))
	headerStart, err := findHeaderRow(content)
	if err != nil {
		return nil, 0, err
	}
	reader := csv.NewReader(bytes.NewReader(content[headerStart:]))
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true
	header, err := reader.Read()
	if err != nil {
		return nil, 0, err
	}
	columns := map[string]int{}
	for index, name := range header {
		columns[strings.TrimSpace(name)] = index
	}
	for {
		row, err := reader.Read()
		if errors.Is(err, io.EOF) {
			return connections, skipped, nil
		}
		if err != nil {
			return nil, 0, fmt.Errorf("read Connections.csv: %w", err)
		}
		field := func(name string) string {
			index, found := columns[name]
			if !found || index >= len(row) {
				return ""
			}
			return strings.TrimSpace(row[index])
		}
		connection := store.NewConnection{
			FirstName: field("First Name"), LastName: field("Last Name"), ProfileURL: field("URL"), Email: field("Email Address"),
			CompanyName: field("Company"), Position: field("Position"), ConnectedOn: parseConnectedOn(field("Connected On")),
		}
		if connection.ProfileURL == "" {
			skipped++
			continue
		}
		connections = append(connections, connection)
	}
}

// findHeaderRow returns where the header row starts, past the notes the
// export puts above it.
func findHeaderRow(content []byte) (int, error) {
	scanner := bufio.NewScanner(bytes.NewReader(content))
	scanner.Buffer(make([]byte, 0, 64*1024), len(content)+1)
	offset := 0
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(strings.TrimSpace(line), "First Name,Last Name") {
			return offset, nil
		}
		offset += len(scanner.Bytes()) + 1
	}
	return 0, ErrNotConnectionsFile
}

func parseConnectedOn(text string) *time.Time {
	for _, layout := range connectedOnLayouts {
		if day, err := time.Parse(layout, text); err == nil {
			return &day
		}
	}
	return nil
}
