package postingpages

import (
	"encoding/json"
	"html"
	"strconv"
	"strings"
	"time"

	nethtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/textextract"
)

// jobPostingData is the part of a schema.org JobPosting the hub keeps.
type jobPostingData struct {
	Type                          json.RawMessage   `json:"@type"`
	Title                         string            `json:"title"`
	Description                   string            `json:"description"`
	DatePosted                    string            `json:"datePosted"`
	EmploymentType                json.RawMessage   `json:"employmentType"`
	JobLocation                   json.RawMessage   `json:"jobLocation"`
	JobLocationType               string            `json:"jobLocationType"`
	ApplicantLocationRequirements json.RawMessage   `json:"applicantLocationRequirements"`
	BaseSalary                    json.RawMessage   `json:"baseSalary"`
	Identifier                    json.RawMessage   `json:"identifier"`
	Graph                         []json.RawMessage `json:"@graph"`
}

// readJobPostingData reads the first schema.org JobPosting in the page's
// JSON-LD, with a title and a description.
func readJobPostingData(document *nethtml.Node) (store.JobPosting, bool) {
	var found *jobPostingData
	walk(document, func(node *nethtml.Node) bool {
		if found != nil {
			return false
		}
		if node.Type != nethtml.ElementNode || node.DataAtom != atom.Script ||
			!strings.EqualFold(strings.TrimSpace(getAttribute(node, "type")), "application/ld+json") || node.FirstChild == nil {
			return true
		}
		found = findJobPosting(json.RawMessage(node.FirstChild.Data), 0)
		return false
	})
	if found == nil {
		return store.JobPosting{}, false
	}
	return found.toPosting(), true
}

// findJobPosting finds a JobPosting in a JSON-LD value: an object, a list of
// them, or a @graph.
func findJobPosting(raw json.RawMessage, depth int) *jobPostingData {
	if depth > 3 {
		return nil
	}
	var list []json.RawMessage
	if json.Unmarshal(raw, &list) == nil {
		for _, item := range list {
			if posting := findJobPosting(item, depth+1); posting != nil {
				return posting
			}
		}
		return nil
	}
	var data jobPostingData
	if json.Unmarshal(raw, &data) != nil {
		return nil
	}
	if hasType(data.Type, "JobPosting") && strings.TrimSpace(data.Title) != "" && strings.TrimSpace(data.Description) != "" {
		return &data
	}
	for _, item := range data.Graph {
		if posting := findJobPosting(item, depth+1); posting != nil {
			return posting
		}
	}
	return nil
}

// readStrings reads a JSON-LD value that is one string or a list of them.
func readStrings(raw json.RawMessage) []string {
	var one string
	if json.Unmarshal(raw, &one) == nil {
		if one = strings.TrimSpace(one); one != "" {
			return []string{one}
		}
		return nil
	}
	var many []string
	_ = json.Unmarshal(raw, &many)
	return many
}

func hasType(raw json.RawMessage, want string) bool {
	for _, kind := range readStrings(raw) {
		if kind == want || strings.HasSuffix(kind, "/"+want) {
			return true
		}
	}
	return false
}

// readObjects reads a JSON-LD value that is one object or a list of them.
func readObjects(raw json.RawMessage) []json.RawMessage {
	var many []json.RawMessage
	if json.Unmarshal(raw, &many) == nil {
		return many
	}
	if len(raw) > 0 && raw[0] == '{' {
		return []json.RawMessage{raw}
	}
	return nil
}

func (data jobPostingData) toPosting() store.JobPosting {
	description := data.Description
	if strings.Contains(description, "&lt;") {
		description = html.UnescapeString(description)
	}
	posting := store.JobPosting{
		Title:       strings.TrimSpace(html.UnescapeString(data.Title)),
		Description: strings.TrimSpace(textextract.ConvertHTMLToMarkdown(description)),
	}
	posting.ExternalID = readIdentifier(data.Identifier)
	for _, location := range readObjects(data.JobLocation) {
		if name := readPlace(location); name != "" {
			posting.OtherLocations = append(posting.OtherLocations, name)
		}
	}
	if strings.EqualFold(data.JobLocationType, "TELECOMMUTE") {
		posting.WorkplaceType = "Remote"
		if len(posting.OtherLocations) == 0 {
			posting.Location = "Remote"
			for _, requirement := range readObjects(data.ApplicantLocationRequirements) {
				if name := readName(requirement); name != "" {
					posting.Location = "Remote (" + name + ")"
					break
				}
			}
		}
	}
	if len(posting.OtherLocations) > 0 {
		posting.Location, posting.OtherLocations = posting.OtherLocations[0], posting.OtherLocations[1:]
	}
	if len(posting.OtherLocations) == 0 {
		posting.OtherLocations = nil
	}
	var types []string
	for _, kind := range readStrings(data.EmploymentType) {
		types = append(types, describeEmploymentType(kind))
	}
	posting.EmploymentType = strings.Join(types, ", ")
	if published, ok := readDate(data.DatePosted); ok {
		posting.PublishedAt = &published
	}
	posting.Pay = readSalary(data.BaseSalary)
	return posting
}

// readIdentifier is the posting's own id, as a PropertyValue's value or a
// plain one.
func readIdentifier(raw json.RawMessage) string {
	if values := readStrings(raw); len(values) > 0 {
		return values[0]
	}
	var value struct {
		Value json.RawMessage `json:"value"`
	}
	if json.Unmarshal(raw, &value) != nil {
		return ""
	}
	if values := readStrings(value.Value); len(values) > 0 {
		return values[0]
	}
	var number json.Number
	if json.Unmarshal(value.Value, &number) == nil {
		return number.String()
	}
	return ""
}

func readName(raw json.RawMessage) string {
	var named struct {
		Name string `json:"name"`
	}
	_ = json.Unmarshal(raw, &named)
	return strings.TrimSpace(named.Name)
}

// readPlace names a Place: its address as "City, Region, Country", or its
// name.
func readPlace(raw json.RawMessage) string {
	var place struct {
		Name    string          `json:"name"`
		Address json.RawMessage `json:"address"`
	}
	if json.Unmarshal(raw, &place) != nil {
		return ""
	}
	if values := readStrings(place.Address); len(values) > 0 {
		return values[0]
	}
	var address struct {
		Locality string          `json:"addressLocality"`
		Region   string          `json:"addressRegion"`
		Country  json.RawMessage `json:"addressCountry"`
	}
	_ = json.Unmarshal(place.Address, &address)
	country := ""
	if values := readStrings(address.Country); len(values) > 0 {
		country = values[0]
	} else {
		country = readName(address.Country)
	}
	var parts []string
	for _, part := range []string{address.Locality, address.Region, country} {
		if part = strings.TrimSpace(part); part != "" {
			parts = append(parts, part)
		}
	}
	if len(parts) > 0 {
		return strings.Join(parts, ", ")
	}
	return strings.TrimSpace(place.Name)
}

func describeEmploymentType(kind string) string {
	switch strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(kind), "-", "_")) {
	case "FULL_TIME":
		return "Full-time"
	case "PART_TIME":
		return "Part-time"
	case "CONTRACTOR", "CONTRACT":
		return "Contract"
	case "TEMPORARY":
		return "Temporary"
	case "INTERN", "INTERNSHIP":
		return "Internship"
	default:
		return strings.TrimSpace(kind)
	}
}

func readDate(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

// salaryIntervals are schema.org's unit texts as the hub's pay intervals.
var salaryIntervals = map[string]string{"YEAR": "year", "MONTH": "month", "WEEK": "week", "DAY": "day", "HOUR": "hour"}

// readSalary reads a MonetaryAmount: its currency and a QuantitativeValue
// with a minimum and a maximum, or one value. Nil when it gives no amount.
func readSalary(raw json.RawMessage) *store.Pay {
	var ranges []store.PayRange
	for _, amount := range readObjects(raw) {
		var salary struct {
			Currency string          `json:"currency"`
			Value    json.RawMessage `json:"value"`
		}
		if json.Unmarshal(amount, &salary) != nil {
			continue
		}
		var value struct {
			MinValue json.RawMessage `json:"minValue"`
			MaxValue json.RawMessage `json:"maxValue"`
			Value    json.RawMessage `json:"value"`
			UnitText string          `json:"unitText"`
		}
		low, high := 0.0, 0.0
		if number, ok := readNumber(salary.Value); ok {
			low, high = number, number
		} else if json.Unmarshal(salary.Value, &value) == nil {
			low, _ = readNumber(value.MinValue)
			high, _ = readNumber(value.MaxValue)
			if one, ok := readNumber(value.Value); ok && low == 0 && high == 0 {
				low, high = one, one
			}
			if high == 0 {
				high = low
			}
			if low == 0 {
				low = high
			}
		}
		if high == 0 || strings.TrimSpace(salary.Currency) == "" {
			continue
		}
		ranges = append(ranges, store.PayRange{
			Min: low, Max: high, Currency: strings.ToUpper(strings.TrimSpace(salary.Currency)),
			Interval: salaryIntervals[strings.ToUpper(strings.TrimSpace(value.UnitText))],
		})
	}
	if len(ranges) == 0 {
		return nil
	}
	return &store.Pay{Ranges: ranges}
}

// readNumber reads a number written as one or as a string.
func readNumber(raw json.RawMessage) (float64, bool) {
	var number float64
	if json.Unmarshal(raw, &number) == nil {
		return number, true
	}
	var text string
	if json.Unmarshal(raw, &text) != nil {
		return 0, false
	}
	number, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(text), ",", ""), 64)
	return number, err == nil
}
