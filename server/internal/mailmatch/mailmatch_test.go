package mailmatch

import (
	"testing"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

func TestEachRuleMatchesItsMail(t *testing.T) {
	acme := store.Company{ID: uuid.New(), Name: "Acme", Domain: "acme.com"}
	wave := store.Company{ID: uuid.New(), Name: "Wave", Domain: "waveapps.com"}
	waveFinancial := store.Company{ID: uuid.New(), Name: "Wave Financial", Domain: "wavefinancial.com"}
	ada := store.Person{ID: uuid.New(), CompanyID: acme.ID, Name: "Ada Lovelace", Email: "ada@acme-partners.org"}
	directory := store.MailDirectory{
		Companies:       []store.Company{acme, wave, waveFinancial},
		PeopleWithEmail: []store.Person{ada},
		ThreadCompanies: map[string]uuid.UUID{"thread-acme": acme.ID},
	}
	received := func(sender, subject string) store.MailMessage {
		return store.MailMessage{Direction: store.MailReceived, Sender: sender, Subject: subject, ThreadID: "other"}
	}

	cases := []struct {
		name        string
		message     store.MailMessage
		wantCompany *store.Company
		wantRule    string
	}{
		{"a known person's address", received(`"Ada Lovelace" <Ada@Acme-Partners.org>`, "Hello"), &acme, store.MatchedByPerson},
		{"a company's domain", received("Bob <bob@acme.com>", "Re: Engineer"), &acme, store.MatchedByDomain},
		{"a subdomain of a company's domain", received("no-reply@mail.acme.com", "We received your application"), &acme, store.MatchedByDomain},
		{"a lookalike domain", received("x@notacme.com", "Hi"), nil, ""},
		{"a webmail sender", received("Carol <carol@gmail.com>", "Acme role"), nil, ""},
		{"a thread already matched", store.MailMessage{Direction: store.MailReceived, Sender: "dave@gmail.com", ThreadID: "thread-acme"}, &acme, store.MatchedByThread},
		{"the company in an applicant-tracking sender's name", received("Acme Hiring <no-reply@us.greenhouse-mail.io>", "Thank you"), &acme, store.MatchedByApplicantTracking},
		{"the company in an applicant-tracking subject", received("no-reply@ashbyhq.com", "Thank you for applying to Acme!"), &acme, store.MatchedByApplicantTracking},
		{"the longest company name first", received("jobs@lever.co", "Your application to Wave Financial"), &waveFinancial, store.MatchedByApplicantTracking},
		{"a company named by a sender that isn't an applicant-tracking system", received("news@digest.example.com", "Acme raises a Series B"), nil, ""},
		{"mail sent to a company", store.MailMessage{Direction: store.MailSent, Sender: "me@gmail.com", Recipients: "Bob <bob@acme.com>, x@gmail.com", ThreadID: "new"}, &acme, store.MatchedByDomain},
		{"mail sent to no one known", store.MailMessage{Direction: store.MailSent, Sender: "bob@acme.com", Recipients: "x@gmail.com", ThreadID: "new"}, nil, ""},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			match, found := FindMatch(testCase.message, directory)
			if testCase.wantCompany == nil {
				if found {
					t.Fatalf("matched %+v; want no match", match)
				}
				return
			}
			if !found || match.CompanyID != testCase.wantCompany.ID || match.MatchedBy != testCase.wantRule {
				t.Fatalf("match = %+v, %v; want %s by %s", match, found, testCase.wantCompany.Name, testCase.wantRule)
			}
			if (testCase.wantRule == store.MatchedByPerson) != (match.PersonID != nil) {
				t.Fatalf("person = %v; only a person match names one", match.PersonID)
			}
		})
	}
}
