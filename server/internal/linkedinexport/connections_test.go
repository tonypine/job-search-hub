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

// Made-up messages in LinkedIn's shape: a quoted multi-line message, a group
// conversation with two recipients, and a row without a date.
const madeUpMessages = `"CONVERSATION ID","CONVERSATION TITLE","FROM","SENDER PROFILE URL","TO","RECIPIENT PROFILE URLS","DATE","SUBJECT","CONTENT","FOLDER","ATTACHMENTS"
"c1","","Rita Recruiter","https://www.linkedin.com/in/rita-example","Owner Example","https://www.linkedin.com/in/owner-example","2025-03-01 10:00:00 UTC","Role at Acme","Hi,
a role you might like.","INBOX",""
"c1","","Owner Example","https://www.linkedin.com/in/owner-example","Rita Recruiter","https://www.linkedin.com/in/rita-example","2025-03-02 11:00:00 UTC","","Thanks, tell me more","INBOX",""
"c2","Team","Owner Example","https://www.linkedin.com/in/owner-example","Ada, Grace","https://www.linkedin.com/in/ada-example,https://www.linkedin.com/in/grace-example","2024-01-01 09:00:00 UTC","","Hello both","INBOX",""
"c3","","Nobody","https://www.linkedin.com/in/nobody","Owner Example","https://www.linkedin.com/in/owner-example","","","no date","INBOX",""
`

func TestMessagesAreReadWithTheirRecipientsAndLines(t *testing.T) {
	messages, err := ParseMessages(strings.NewReader(madeUpMessages))
	if err != nil || len(messages) != 3 {
		t.Fatalf("parse = %d messages, %v", len(messages), err)
	}
	if messages[0].Content != "Hi,\na role you might like." || messages[0].SentAt.Format("2006-01-02 15:04") != "2025-03-01 10:00" {
		t.Fatalf("first = %+v", messages[0])
	}
	if len(messages[2].RecipientProfileURLs) != 2 || messages[2].ConversationTitle != "Team" {
		t.Fatalf("group message = %+v", messages[2])
	}
	if _, err := ParseMessages(strings.NewReader("First Name,Last Name\n")); !errors.Is(err, ErrNotMessagesFile) {
		t.Fatalf("another file: err = %v", err)
	}
}

func TestInvitationsAreReadBothWays(t *testing.T) {
	invitations, err := ParseInvitations(strings.NewReader(`From,To,Sent At,Message,Direction,inviterProfileUrl,inviteeProfileUrl
Owner Example,Ada Lovelace,"9/25/26, 8:52 AM",,OUTGOING,https://www.linkedin.com/in/owner-example,https://www.linkedin.com/in/ada-example
Rita Recruiter,Owner Example,"9/20/26, 4:10 PM","Hi, a role for you",INCOMING,https://www.linkedin.com/in/rita-example,https://www.linkedin.com/in/owner-example
`))
	if err != nil || len(invitations) != 2 || invitations[0].Direction != "outgoing" || invitations[1].Message != "Hi, a role for you" ||
		invitations[1].SentAt == nil || invitations[1].SentAt.Format("2006-01-02 15:04") != "2026-09-20 16:10" {
		t.Fatalf("invitations = %+v, %v", invitations, err)
	}
}

func TestEndorsementsAndRecommendationsAreRead(t *testing.T) {
	endorsements, err := ParseEndorsementsReceived(strings.NewReader("Endorsement Date,Skill Name,Endorser First Name,Endorser Last Name,Endorser Public Url,Endorsement Status\n" +
		"2025/06/10 23:13:46 UTC,React,Ada,Lovelace,www.linkedin.com/in/ada-example,ACCEPTED\n"))
	if err != nil || len(endorsements) != 1 || endorsements[0].ProfileURL != "https://www.linkedin.com/in/ada-example" ||
		endorsements[0].Status != "accepted" || endorsements[0].EndorsedAt == nil {
		t.Fatalf("endorsements = %+v, %v", endorsements, err)
	}
	recommendations, err := ParseRecommendationsReceived(strings.NewReader("First Name,Last Name,Company,Job Title,Text,Creation Date,Status\n" +
		"Ada,Lovelace,Acme,CTO,\"Great, reliable engineer\",\"01/19/13, 12:10 AM\",VISIBLE\n"))
	if err != nil || len(recommendations) != 1 || recommendations[0].Text != "Great, reliable engineer" || recommendations[0].WrittenAt.Year() != 2013 {
		t.Fatalf("recommendations = %+v, %v", recommendations, err)
	}
}
