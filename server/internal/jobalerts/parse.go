// Package jobalerts reads the job alert emails Indeed, LinkedIn and Glassdoor
// send the owner into the jobs feed. Everything comes from the email itself:
// the hub never fetches the postings' pages. The text a card lacks comes from
// the company's board (boardfinder) or Google for Jobs (postingtexts).
package jobalerts

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"html"
	"io"
	"net/mail"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

// Alert is the owner's copy of one alert email.
type Alert struct {
	Sender  string
	Subject string
	SentAt  time.Time
	Text    string
	HTML    string
}

// senderSources are the addresses alerts come from, by the source their jobs
// are stored under.
var senderSources = map[string]string{
	"donotreply@match.indeed.com":    store.JobSourceIndeed,
	"jobs-noreply@linkedin.com":      store.JobSourceLinkedIn,
	"jobalerts-noreply@linkedin.com": store.JobSourceLinkedIn,
	"messages-noreply@linkedin.com":  store.JobSourceLinkedIn,
	"noreply@glassdoor.com":          store.JobSourceGlassdoor,
}

// GetSource returns the source of the jobs an alert from sender lists, and
// false for a sender that sends no alerts the hub reads.
func GetSource(sender string) (string, bool) {
	address, err := mail.ParseAddress(sender)
	if err != nil {
		return "", false
	}
	source, known := senderSources[strings.ToLower(address.Address)]
	return source, known
}

// ParsePostings returns the postings an alert lists, each with its external
// id, company, title, location and link, expiring after postingLifetime.
func ParsePostings(source string, alert Alert) []store.JobPosting {
	var postings []store.JobPosting
	switch source {
	case store.JobSourceIndeed:
		postings = parseIndeed(alert)
	case store.JobSourceLinkedIn:
		postings = parseLinkedIn(alert)
	case store.JobSourceGlassdoor:
		postings = parseGlassdoor(alert)
	}
	expiresAt := alert.SentAt.Add(postingLifetime)
	for index := range postings {
		postings[index].ExpiresAt = &expiresAt
	}
	return postings
}

// postingLifetime is how long a job read from an alert stays open: alerts
// never say when a posting closes.
const postingLifetime = 30 * 24 * time.Hour

var (
	indeedSubject = regexp.MustCompile(`^(.+?) (?:na empresa|at) (.+)$`)
	indeedLink    = regexp.MustCompile(`^(?:Ver vaga|View job): (https://\S+)`)
	indeedFact    = regexp.MustCompile(`^([^:]{2,40}): (.+)$`)
)

// parseIndeed reads Indeed's one-posting alert: the subject names the title
// and company, and the text lists the posting from its title line down to
// its link.
func parseIndeed(alert Alert) []store.JobPosting {
	subject := indeedSubject.FindStringSubmatch(strings.TrimSpace(alert.Subject))
	if subject == nil {
		return nil
	}
	title, company := strings.TrimSpace(subject[1]), strings.TrimSpace(subject[2])
	lines := splitLines(alert.Text)
	start := -1
	for index, line := range lines {
		if line == title && index+1 < len(lines) && lines[index+1] == company {
			start = index
			break
		}
	}
	if start < 0 {
		return nil
	}
	var block []string
	for _, line := range lines[start:] {
		if link := indeedLink.FindStringSubmatch(line); link != nil {
			externalID, postingURL := readIndeedLink(link[1], title, company)
			location := ""
			if start+2 < len(lines) && lines[start+2] != "" && !indeedFact.MatchString(lines[start+2]) {
				location = lines[start+2]
			}
			return []store.JobPosting{{
				ExternalID: externalID, CompanyName: company, Title: title, Location: location, URL: postingURL,
				Description: strings.TrimSpace(strings.Join(block, "\n")),
			}}
		}
		block = append(block, line)
	}
	return nil
}

// readIndeedLink finds the posting's job key inside Indeed's tracking link,
// whose first path segment is gzipped JSON holding the posting's address.
// It falls back to the tracking link itself.
func readIndeedLink(trackingLink, title, company string) (string, string) {
	fallback := func() (string, string) { return "listing:" + hashText(title+"\n"+company), trackingLink }
	parsed, err := url.Parse(trackingLink)
	if err != nil {
		return fallback()
	}
	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(segments) < 2 || segments[0] != "v3" {
		return fallback()
	}
	compressed, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(segments[1], "="))
	if err != nil {
		return fallback()
	}
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return fallback()
	}
	decoded, err := io.ReadAll(io.LimitReader(reader, 1<<16))
	if err != nil {
		return fallback()
	}
	var payload struct {
		URL string `json:"u"`
	}
	if json.Unmarshal(decoded, &payload) != nil {
		return fallback()
	}
	target, err := url.Parse(payload.URL)
	if err != nil || target.Query().Get("jk") == "" || !strings.HasSuffix(target.Hostname(), "indeed.com") {
		return fallback()
	}
	jobKey := target.Query().Get("jk")
	return jobKey, "https://" + target.Hostname() + "/viewjob?jk=" + url.QueryEscape(jobKey)
}

var (
	linkedInJobLink  = regexp.MustCompile(`https://www\.linkedin\.com/(?:comm/)?jobs/view/(\d+)`)
	separatorLine    = regexp.MustCompile(`^-{5,}$`)
	companyLocations = regexp.MustCompile(`\s+·\s+`)
)

// parseLinkedIn reads LinkedIn's alerts, where each posting is a paragraph
// followed by its link: title, company and location on their own lines, or
// title then "Company · Location".
func parseLinkedIn(alert Alert) []store.JobPosting {
	lines := splitLines(alert.Text)
	seen := map[string]bool{}
	var postings []store.JobPosting
	for index, line := range lines {
		link := linkedInJobLink.FindStringSubmatch(line)
		if link == nil || seen[link[1]] {
			continue
		}
		paragraph := getParagraphBefore(lines, index)
		if len(paragraph) < 2 {
			continue
		}
		posting := store.JobPosting{ExternalID: link[1], Title: paragraph[0], URL: "https://www.linkedin.com/jobs/view/" + link[1] + "/"}
		if parts := companyLocations.Split(paragraph[1], -1); len(parts) > 1 {
			posting.CompanyName, posting.Location = parts[0], parts[len(parts)-1]
		} else {
			posting.CompanyName = paragraph[1]
			if len(paragraph) > 2 {
				posting.Location = paragraph[2]
			}
		}
		seen[link[1]] = true
		postings = append(postings, posting)
	}
	return postings
}

// getParagraphBefore returns the lines of the paragraph that ends before the
// line at end, skipping blank lines between them.
func getParagraphBefore(lines []string, end int) []string {
	index := end - 1
	for index >= 0 && lines[index] == "" {
		index--
	}
	last := index
	for index >= 0 && lines[index] != "" && !separatorLine.MatchString(lines[index]) && !strings.Contains(lines[index], "https://") {
		index--
	}
	return lines[index+1 : last+1]
}

var (
	glassdoorCard      = regexp.MustCompile(`(?s)<a [^>]*href="(https://[^"/]*glassdoor[^"/]*)/[^"]*jobListingId=(\d+)[^"]*"[^>]*>(.*?)</a>`)
	glassdoorCompany   = regexp.MustCompile(`(?s)<span[^>]*>\s*<span[^>]*>(.*?)</span>`)
	glassdoorParagraph = regexp.MustCompile(`(?s)<p[^>]*>(.*?)</p>`)
	glassdoorAge       = regexp.MustCompile(`^(\d+)\+?\s*(dia\(s\)|d|days?|h)$`)
	glassdoorPay       = regexp.MustCompile(`(?:R\$|US\$|\$|€|£)\s*\d`)
	// glassdoorCheckInCard is a card of the "how's your search going" mail:
	// the title in a cell, then the company and the location in spans.
	glassdoorCheckInCard = regexp.MustCompile(`(?s)<td[^>]*>([^<]+)</td>.*?<span[^>]*>([^<]*)<!--\s*-->[^<]*</span>\s*<span[^>]*>([^<]*)</span>`)
	anyTag               = regexp.MustCompile(`<[^>]*>`)
)

// parseGlassdoor reads Glassdoor's alerts from their HTML, where each posting
// is a card linked to its listing: the company, then paragraphs for the
// title, the location, the pay, and how many days ago it was posted. A
// search check-in's cards give the title, the company and the location.
func parseGlassdoor(alert Alert) []store.JobPosting {
	seen := map[string]bool{}
	var postings []store.JobPosting
	for _, card := range glassdoorCard.FindAllStringSubmatch(alert.HTML, -1) {
		host, listingID, content := card[1], card[2], card[3]
		company := glassdoorCompany.FindStringSubmatch(content)
		var paragraphs []string
		for _, paragraph := range glassdoorParagraph.FindAllStringSubmatch(content, -1) {
			if text := getHTMLText(paragraph[1]); text != "" {
				paragraphs = append(paragraphs, text)
			}
		}
		if seen[listingID] {
			continue
		}
		if company == nil || len(paragraphs) == 0 {
			if checkIn := glassdoorCheckInCard.FindStringSubmatch(content); checkIn != nil && getHTMLText(checkIn[2]) != "" {
				seen[listingID] = true
				postings = append(postings, store.JobPosting{
					ExternalID: listingID, Title: getHTMLText(checkIn[1]), CompanyName: getHTMLText(checkIn[2]),
					Location: getHTMLText(checkIn[3]), URL: host + "/job-listing/j?jl=" + listingID,
				})
			}
			continue
		}
		posting := store.JobPosting{
			ExternalID: listingID, CompanyName: getHTMLText(company[1]), Title: paragraphs[0],
			URL: host + "/job-listing/j?jl=" + listingID,
		}
		var details []string
		for _, paragraph := range paragraphs[1:] {
			if age := glassdoorAge.FindStringSubmatch(paragraph); age != nil {
				count, _ := strconv.Atoi(age[1])
				unit := 24 * time.Hour
				if age[2] == "h" {
					unit = time.Hour
				}
				publishedAt := alert.SentAt.Add(-time.Duration(count) * unit)
				posting.PublishedAt = &publishedAt
				continue
			}
			if posting.Location == "" && !glassdoorPay.MatchString(paragraph) {
				posting.Location = paragraph
				continue
			}
			details = append(details, paragraph)
		}
		posting.Description = strings.Join(details, "\n")
		if posting.CompanyName == "" {
			continue
		}
		seen[listingID] = true
		postings = append(postings, posting)
	}
	return postings
}

func getHTMLText(markup string) string {
	text := html.UnescapeString(anyTag.ReplaceAllString(markup, " "))
	return strings.Join(strings.Fields(strings.ReplaceAll(text, " ", " ")), " ")
}

// splitLines returns the text's lines trimmed, with lines of only spaces
// made empty.
func splitLines(text string) []string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for index, line := range lines {
		lines[index] = strings.TrimSpace(line)
	}
	return lines
}

func hashText(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:8])
}
