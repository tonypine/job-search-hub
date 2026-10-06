package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

func TestPrintWatchListSaysWhenItIsEmpty(t *testing.T) {
	var out bytes.Buffer
	printWatchList(&out, nil)
	if out.String() != "The watch list is empty.\n" {
		t.Fatalf("output = %q", out.String())
	}
}

func TestPrintDossierShowsBoardsAndPeopleWithSources(t *testing.T) {
	watchedSince := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	openPostings := 12
	var out bytes.Buffer
	printDossier(&out, store.CompanyDossier{
		Company:      store.Company{Name: "Acme", Domain: "acme.com", CareersURL: "https://acme.com/careers", Industry: "Hardware", Summary: "Makes anvils."},
		WatchedSince: &watchedSince,
		JobBoards: []store.JobBoard{{
			Provider: "greenhouse", BoardToken: "acme", BoardURL: "https://job-boards.greenhouse.io/acme",
			VerifiedAt: &watchedSince, OpenPostingCount: &openPostings,
		}},
		People: []store.Person{{Name: "Ada Lovelace", RoleTitle: "Engineering Manager", Relevance: "hiring_manager", SourceURL: "https://acme.com/team"}},
	})

	for _, want := range []string{
		"Acme (acme.com)", "careers: https://acme.com/careers", "industry: Hardware", "Makes anvils.", "On the watch list since 2026-09-28.",
		"greenhouse/acme  verified 2026-09-28, 12 open postings", "Ada Lovelace (Engineering Manager, hiring_manager)",
		"source: https://acme.com/team",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, out.String())
		}
	}
}

func TestDescribeVerificationCoversEachState(t *testing.T) {
	verifiedAt := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	openPostings := 7
	for want, board := range map[string]store.JobBoard{
		"unverified": {},
		"verified 2026-09-28, open postings unknown": {VerifiedAt: &verifiedAt},
		"verified 2026-09-28, 7 open postings":       {VerifiedAt: &verifiedAt, OpenPostingCount: &openPostings},
	} {
		if got := describeVerification(board); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
}
