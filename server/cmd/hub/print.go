package main

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

func printWatchList(out io.Writer, watched []store.WatchedCompany) {
	if len(watched) == 0 {
		fmt.Fprintln(out, "The watch list is empty.")
		return
	}
	for _, entry := range watched {
		fmt.Fprintf(out, "%-28s %-24s watched since %s\n", entry.Company.Name, entry.Company.Domain, entry.WatchedSince.Format(time.DateOnly))
	}
}

func printDossier(out io.Writer, dossier store.CompanyDossier) {
	company := dossier.Company
	fmt.Fprintf(out, "%s (%s)\n", company.Name, company.Domain)

	var facts []string
	for _, fact := range []struct{ label, value string }{
		{"", company.WebsiteURL},
		{"careers: ", company.CareersURL},
		{"HQ: ", company.HeadquartersCountry},
		{"employees: ", company.EmployeeCountRange},
	} {
		if fact.value != "" {
			facts = append(facts, fact.label+fact.value)
		}
	}
	if len(facts) > 0 {
		fmt.Fprintln(out, strings.Join(facts, " · "))
	}
	if company.Summary != "" {
		fmt.Fprintf(out, "\n%s\n", company.Summary)
	}

	fmt.Fprintln(out)
	if dossier.WatchedSince != nil {
		fmt.Fprintf(out, "On the watch list since %s.\n", dossier.WatchedSince.Format(time.DateOnly))
	} else {
		fmt.Fprintln(out, "Not on the watch list.")
	}

	fmt.Fprintln(out, "\nJob boards:")
	if len(dossier.JobBoards) == 0 {
		fmt.Fprintln(out, "  none stored")
	}
	for _, board := range dossier.JobBoards {
		fmt.Fprintf(out, "  %s/%s  %s  %s\n", board.Provider, board.BoardToken, describeVerification(board), board.BoardURL)
	}

	fmt.Fprintln(out, "\nPeople:")
	if len(dossier.People) == 0 {
		fmt.Fprintln(out, "  none stored")
	}
	for _, person := range dossier.People {
		role := person.Relevance
		if person.RoleTitle != "" {
			role = person.RoleTitle + ", " + person.Relevance
		}
		fmt.Fprintf(out, "  %s (%s)\n    source: %s\n", person.Name, role, person.SourceURL)
	}
}

func describeVerification(board store.JobBoard) string {
	if board.VerifiedAt == nil || board.OpenPostingCount == nil {
		return "unverified"
	}
	return fmt.Sprintf("verified %s, %d open postings", board.VerifiedAt.Format(time.DateOnly), *board.OpenPostingCount)
}
