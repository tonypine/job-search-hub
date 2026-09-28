package linkedinexport

import (
	"errors"
	"strings"
	"testing"
)

// A made-up export in LinkedIn's shape: notes, a blank line, the header, and
// rows with quoted commas, a missing email, and a row without a URL.
const madeUpExport = "Notes:\n" +
	"\"When exporting your connection data, you may notice that some of the email addresses are missing. You will only see email addresses for connections who have allowed their connections to see or download their email address using this setting https://www.linkedin.com/psettings/privacy/email\"\n" +
	"\n" +
	"First Name,Last Name,URL,Email Address,Company,Position,Connected On\n" +
	"Ada,Lovelace,https://www.linkedin.com/in/ada-example,ada@example.com,\"Acme, Inc.\",Engineering Manager,28 Sep 2026\n" +
	"Grace,Hopper,https://www.linkedin.com/in/grace-example,,Globex,\"Senior Engineer, Platform\",3 Jan 2019\n" +
	"No,Url,,,Initech,Recruiter,01 Feb 2020\n"

func TestAnExportIsReadPastItsNotes(t *testing.T) {
	connections, skipped, err := ParseConnections(strings.NewReader("\ufeff" + madeUpExport))
	if err != nil || len(connections) != 2 || skipped != 1 {
		t.Fatalf("parse = %d connections, %d skipped, %v", len(connections), skipped, err)
	}
	ada, grace := connections[0], connections[1]
	if ada.CompanyName != "Acme, Inc." || ada.Email != "ada@example.com" || ada.ConnectedOn == nil || ada.ConnectedOn.Format("2006-01-02") != "2026-09-28" {
		t.Fatalf("ada = %+v", ada)
	}
	if grace.Email != "" || grace.Position != "Senior Engineer, Platform" || grace.ConnectedOn.Format("2006-01-02") != "2019-01-03" {
		t.Fatalf("grace = %+v", grace)
	}
}

func TestAnotherFileIsRefused(t *testing.T) {
	if _, _, err := ParseConnections(strings.NewReader("Name,Email\nAda,ada@example.com\n")); !errors.Is(err, ErrNotConnectionsFile) {
		t.Fatalf("err = %v", err)
	}
}
