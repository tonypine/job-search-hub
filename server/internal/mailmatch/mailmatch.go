// Package mailmatch ties a mail message to the company, and the person, it
// is about, from what the hub knows.
package mailmatch

import (
	"cmp"
	"context"
	"net/mail"
	"regexp"
	"slices"
	"strings"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/wordmatch"
)

// webmailDomains hold anyone's address, so they never name a company.
var webmailDomains = []string{
	"gmail.com", "googlemail.com", "outlook.com", "hotmail.com", "live.com", "msn.com", "yahoo.com", "icloud.com", "me.com",
	"proton.me", "protonmail.com", "aol.com", "hotmail.com.br", "yahoo.com.br", "outlook.com.br", "uol.com.br", "bol.com.br", "terra.com.br",
}

// applicantTrackingDomains send mail for many companies, naming the company
// in the sender's name or the subject.
var applicantTrackingDomains = []string{
	"greenhouse.io", "greenhouse-mail.io", "lever.co", "ashbyhq.com", "workable.com", "workablemail.com", "myworkday.com",
	"smartrecruiters.com", "gupy.io", "breezy.hr", "recruitee.com", "teamtailor.com", "teamtailor-mail.com", "jobvite.com",
	"bamboohr.com", "icims.com", "pinpointhq.com",
}

// maximumUnmatchedMessages bounds one pass over old mail.
const maximumUnmatchedMessages = 50000

var addressPattern = regexp.MustCompile(`[A-Za-z0-9._%+'-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)

// FindMatch returns what the message is about, trying in order: a known
// person's address, a company's domain, a thread already matched, and the
// company an applicant-tracking system's mail names. The addresses tried are
// the sender's for mail received and the recipients' for mail sent.
func FindMatch(message store.MailMessage, directory store.MailDirectory) (store.MailMatch, bool) {
	addresses := GetAddresses(message.Sender)
	if message.Direction == store.MailSent {
		addresses = GetAddresses(message.Recipients)
	}
	for _, address := range addresses {
		for _, person := range directory.PeopleWithEmail {
			if strings.EqualFold(person.Email, address) {
				return store.MailMatch{CompanyID: person.CompanyID, PersonID: &person.ID, MatchedBy: store.MatchedByPerson}, true
			}
		}
	}
	for _, address := range addresses {
		domain := GetDomain(address)
		if isOnDomainList(domain, webmailDomains) {
			continue
		}
		for _, company := range directory.Companies {
			if company.Domain != "" && isOnDomainList(domain, []string{company.Domain}) {
				return store.MailMatch{CompanyID: company.ID, MatchedBy: store.MatchedByDomain}, true
			}
		}
	}
	if companyID, found := directory.ThreadCompanies[message.ThreadID]; found {
		return store.MailMatch{CompanyID: companyID, MatchedBy: store.MatchedByThread}, true
	}
	if message.Direction == store.MailReceived && len(addresses) > 0 && isOnDomainList(GetDomain(addresses[0]), applicantTrackingDomains) {
		if company, found := findNamedCompany([]string{GetDisplayName(message.Sender), message.Subject}, directory.Companies); found {
			return store.MailMatch{CompanyID: company.ID, MatchedBy: store.MatchedByApplicantTracking}, true
		}
	}
	return store.MailMatch{}, false
}

// GetAddresses returns the email addresses in a header, lower-cased.
func GetAddresses(header string) []string {
	found := addressPattern.FindAllString(header, -1)
	for index := range found {
		found[index] = strings.ToLower(found[index])
	}
	return found
}

// GetDomain returns the part of an address after the @.
func GetDomain(address string) string {
	_, domain, _ := strings.Cut(address, "@")
	return domain
}

// isOnDomainList reports whether domain is one of domains or a subdomain of
// one, as mail.acme.com is of acme.com.
func isOnDomainList(domain string, domains []string) bool {
	return slices.ContainsFunc(domains, func(listed string) bool {
		listed = strings.ToLower(listed)
		return domain == listed || strings.HasSuffix(domain, "."+listed)
	})
}

// GetDisplayName returns the name part of a From header: "Acme Hiring" in
// "Acme Hiring <no-reply@greenhouse.io>".
func GetDisplayName(header string) string {
	if address, err := mail.ParseAddress(header); err == nil {
		return address.Name
	}
	name, _, _ := strings.Cut(header, "<")
	return strings.Trim(strings.TrimSpace(name), `"`)
}

// findNamedCompany returns the company whose name appears as whole words in
// any of texts, the longest name first, so "Wave Financial" wins over "Wave".
func findNamedCompany(texts []string, companies []store.Company) (store.Company, bool) {
	byLongestName := slices.Clone(companies)
	slices.SortFunc(byLongestName, func(a, b store.Company) int { return cmp.Compare(len(b.Name), len(a.Name)) })
	for _, company := range byLongestName {
		name := wordmatch.Normalize(company.Name)
		if name == "" {
			continue
		}
		for _, text := range texts {
			if wordmatch.Contains(wordmatch.Normalize(text), name) {
				return company, true
			}
		}
	}
	return store.Company{}, false
}

// MatchMessages matches each of messages that no rule has matched yet, oldest
// first, and returns how many it matched. A second pass lets an earlier
// message match through a thread only a later one matched.
func MatchMessages(ctx context.Context, hub *store.Store, messages []store.MailMessage) (matched int, err error) {
	directory, err := hub.GetMailDirectory(ctx)
	if err != nil {
		return 0, err
	}
	unmatched := slices.Clone(messages)
	slices.SortFunc(unmatched, func(a, b store.MailMessage) int { return a.SentAt.Compare(b.SentAt) })
	for range 2 {
		var stillUnmatched []store.MailMessage
		for _, message := range unmatched {
			if message.MatchedBy != "" {
				continue
			}
			match, found := FindMatch(message, directory)
			if !found {
				stillUnmatched = append(stillUnmatched, message)
				continue
			}
			if err := hub.SaveMailMatch(ctx, message.ID, match); err != nil {
				return matched, err
			}
			if _, known := directory.ThreadCompanies[message.ThreadID]; !known {
				directory.ThreadCompanies[message.ThreadID] = match.CompanyID
			}
			matched++
		}
		unmatched = stillUnmatched
	}
	return matched, nil
}

// MatchUnmatched matches every recorded message no rule has matched yet, for
// when the hub learns a company or a person that old mail is about.
func MatchUnmatched(ctx context.Context, hub *store.Store) (int, error) {
	messages, err := hub.ListMailMessages(ctx, store.MailFilter{UnmatchedOnly: true, Limit: maximumUnmatchedMessages})
	if err != nil {
		return 0, err
	}
	return MatchMessages(ctx, hub, messages)
}

// IsCompanyAddress reports whether an address belongs to one company: not
// webmail, and not an applicant-tracking system sending for many.
func IsCompanyAddress(address string) bool {
	domain := GetDomain(strings.ToLower(address))
	return domain != "" && !isOnDomainList(domain, webmailDomains) && !isOnDomainList(domain, applicantTrackingDomains)
}
