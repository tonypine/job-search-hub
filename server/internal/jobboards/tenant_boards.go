package jobboards

import (
	"regexp"
	"strconv"
	"strings"
)

// tenantName is a board token that can be its own subdomain, as on Recruitee,
// BambooHR, Personio and Pinpoint.
var tenantName = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// getTenantBase returns base when set, as tests set it, or the board's own
// site on the provider's domain, as https://acme.recruitee.com. A token that
// can't be a subdomain names no board.
func getTenantBase(base, boardToken, domain string) (string, error) {
	if base != "" {
		return base, nil
	}
	if !tenantName.MatchString(boardToken) {
		return "", ErrPostingAPIOff
	}
	return "https://" + boardToken + "." + domain, nil
}

// joinSections joins a posting's text sections with a blank line, skipping
// empty ones.
func joinSections(sections ...string) string {
	return joinNonEmpty("\n\n", sections...)
}

// formatSection is a titled part of a posting: its title as a heading over
// its text.
func formatSection(title, text string) string {
	if title = strings.TrimSpace(title); title != "" {
		title = "### " + title
	}
	return joinNonEmpty("\n\n", title, text)
}

func joinNonEmpty(separator string, parts ...string) string {
	var kept []string
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			kept = append(kept, trimmed)
		}
	}
	return strings.Join(kept, separator)
}

// looseNumber reads a number sent bare or quoted, as Recruitee and Pinpoint
// send pay; anything else leaves it unset.
type looseNumber struct {
	value *float64
}

func (number *looseNumber) UnmarshalJSON(data []byte) error {
	value, err := strconv.ParseFloat(strings.Trim(string(data), `"`), 64)
	if err == nil {
		number.value = &value
	}
	return nil
}
