package linkedinexport

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

// ErrNotMessagesFile and ErrNotInvitationsFile mean the file lacks the
// columns of messages.csv or Invitations.csv.
var (
	ErrNotMessagesFile    = errors.New(`not a LinkedIn messages.csv: no "CONVERSATION ID" column`)
	ErrNotInvitationsFile = errors.New(`not a LinkedIn Invitations.csv: no "Direction" column`)
)

const (
	messageDateLayout     = "2006-01-02 15:04:05 MST"
	invitationDateLayout  = "1/2/06, 3:04 PM"
	recipientURLSeparator = ","
)

// ParseMessages reads messages.csv: one message per row, in any order,
// grouped by conversation id. A row without a conversation or a date is
// skipped.
func ParseMessages(file io.Reader) ([]store.NewLinkedInMessage, error) {
	rows, columns, err := readTable(file, "CONVERSATION ID")
	if errors.Is(err, errMissingColumn) {
		return nil, ErrNotMessagesFile
	}
	if err != nil {
		return nil, err
	}
	var messages []store.NewLinkedInMessage
	for _, row := range rows {
		field := columns.reader(row)
		sentAt, err := time.Parse(messageDateLayout, field("DATE"))
		if field("CONVERSATION ID") == "" || err != nil {
			continue
		}
		var recipients []string
		for _, url := range strings.Split(field("RECIPIENT PROFILE URLS"), recipientURLSeparator) {
			if url = strings.TrimSpace(url); url != "" {
				recipients = append(recipients, url)
			}
		}
		messages = append(messages, store.NewLinkedInMessage{
			ConversationID: field("CONVERSATION ID"), ConversationTitle: field("CONVERSATION TITLE"),
			SenderName: field("FROM"), SenderProfileURL: field("SENDER PROFILE URL"), RecipientProfileURLs: recipients,
			SentAt: sentAt.UTC(), Subject: field("SUBJECT"), Content: field("CONTENT"), Folder: field("FOLDER"),
		})
	}
	return messages, nil
}

// ParseInvitations reads Invitations.csv: the connection requests sent and
// received that are still open.
func ParseInvitations(file io.Reader) ([]store.NewLinkedInInvitation, error) {
	rows, columns, err := readTable(file, "Direction")
	if errors.Is(err, errMissingColumn) {
		return nil, ErrNotInvitationsFile
	}
	if err != nil {
		return nil, err
	}
	var invitations []store.NewLinkedInInvitation
	for _, row := range rows {
		field := columns.reader(row)
		invitation := store.NewLinkedInInvitation{
			Direction: strings.ToLower(field("Direction")), FromName: field("From"), ToName: field("To"),
			InviterURL: field("inviterProfileUrl"), InviteeURL: field("inviteeProfileUrl"), Message: field("Message"),
		}
		if sentAt, err := time.Parse(invitationDateLayout, field("Sent At")); err == nil {
			invitation.SentAt = &sentAt
		}
		if invitation.Direction != "incoming" && invitation.Direction != "outgoing" {
			continue
		}
		invitations = append(invitations, invitation)
	}
	return invitations, nil
}

var errMissingColumn = errors.New("missing column")

type tableColumns map[string]int

func (columns tableColumns) reader(row []string) func(string) string {
	return func(name string) string {
		index, found := columns[name]
		if !found || index >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[index])
	}
}

// readTable reads a CSV whose first row is its header, which must name
// requiredColumn.
func readTable(file io.Reader, requiredColumn string) ([][]string, tableColumns, error) {
	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true
	header, err := reader.Read()
	if errors.Is(err, io.EOF) {
		return nil, nil, errMissingColumn
	}
	if err != nil {
		return nil, nil, err
	}
	columns := tableColumns{}
	for index, name := range header {
		columns[strings.TrimSpace(strings.TrimPrefix(name, "\ufeff"))] = index
	}
	if _, found := columns[requiredColumn]; !found {
		return nil, nil, errMissingColumn
	}
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, nil, fmt.Errorf("read the file: %w", err)
	}
	return rows, columns, nil
}
