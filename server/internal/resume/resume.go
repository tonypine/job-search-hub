// Package resume keeps a CV as JSON Resume content (jsonresume.org) and
// renders it as HTML in the owner's CV design.
package resume

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"net/url"
	"strings"
	"time"
)

// Resume is the part of JSON Resume the hub's CVs use. Work.Skills and
// Location.Country extend the standard, which allows extra fields.
type Resume struct {
	Basics    Basics      `json:"basics"`
	Work      []Work      `json:"work"`
	Education []Education `json:"education"`
	Skills    []Skill     `json:"skills"`
	Languages []Language  `json:"languages"`
}

type Basics struct {
	Name     string    `json:"name"`
	Label    string    `json:"label"`
	Email    string    `json:"email,omitempty"`
	Phone    string    `json:"phone,omitempty"`
	Location Location  `json:"location"`
	Summary  string    `json:"summary"`
	Profiles []Profile `json:"profiles,omitempty"`
}

// Profile is an account elsewhere, such as LinkedIn: its network, the
// owner's username there and the profile's address.
type Profile struct {
	Network  string `json:"network"`
	Username string `json:"username,omitempty"`
	URL      string `json:"url"`
}

type Location struct {
	City    string `json:"city,omitempty"`
	Region  string `json:"region,omitempty"`
	Country string `json:"country,omitempty"`
}

type Work struct {
	Name       string   `json:"name"`
	Position   string   `json:"position"`
	Location   string   `json:"location,omitempty"`
	StartDate  string   `json:"startDate"`
	EndDate    string   `json:"endDate,omitempty"`
	Highlights []string `json:"highlights"`
	Skills     []string `json:"skills,omitempty"`
}

type Education struct {
	Institution string `json:"institution"`
	StudyType   string `json:"studyType"`
	Area        string `json:"area"`
	StartDate   string `json:"startDate"`
	EndDate     string `json:"endDate,omitempty"`
}

type Skill struct {
	Name string `json:"name"`
}

type Language struct {
	Language string `json:"language"`
	Fluency  string `json:"fluency"`
}

// GetBulletID names a work highlight by its place, "w2h1" for the second
// highlight of the third role, so a tailored CV can cite the base bullet it
// reuses.
func GetBulletID(workIndex, highlightIndex int) string {
	return fmt.Sprintf("w%dh%d", workIndex, highlightIndex)
}

// formatMonth turns "2024-01" into "Jan 2024", and "2010" into "2010".
func formatMonth(date string) string {
	if parsed, err := time.Parse("2006-01", date); err == nil {
		return parsed.Format("Jan 2006")
	}
	return date
}

// formatDateRange is how the CV dates a role or a degree: "Jan 2024 -
// Present" when it hasn't ended.
func formatDateRange(startDate, endDate string) string {
	end := "Present"
	if endDate != "" {
		end = formatMonth(endDate)
	}
	return formatMonth(startDate) + " - " + end
}

// formatLocation is "São Paulo, São Paulo, Brazil", leaving out the parts
// that are empty.
func formatLocation(location Location) string {
	var parts []string
	for _, part := range []string{location.City, location.Region, location.Country} {
		if part != "" {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, ", ")
}

// contactPart is one piece of the CV's contact line: its text and, when it
// has one, the link it opens.
type contactPart struct {
	Text string
	Link template.URL
}

// listContactParts is the header's contact line: the location, then the
// phone, the email and each profile, leaving out the ones that are empty.
// The phone links as tel:, the email as mailto:, and a profile as its own
// http(s) address, shown without its scheme and "www.".
func listContactParts(basics Basics) []contactPart {
	var parts []contactPart
	if location := formatLocation(basics.Location); location != "" {
		parts = append(parts, contactPart{Text: location})
	}
	if phone := strings.TrimSpace(basics.Phone); phone != "" {
		part := contactPart{Text: phone}
		if number := buildPhoneNumber(phone); number != "" {
			part.Link = template.URL("tel:" + number)
		}
		parts = append(parts, part)
	}
	if email := strings.TrimSpace(basics.Email); email != "" {
		part := contactPart{Text: email}
		if strings.Contains(email, "@") && !strings.ContainsAny(email, " <>\"'") {
			part.Link = template.URL("mailto:" + email)
		}
		parts = append(parts, part)
	}
	for _, profile := range basics.Profiles {
		address := strings.TrimSpace(profile.URL)
		parsed, err := url.Parse(address)
		if address == "" || err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
			if text := strings.TrimSpace(profile.Username); text != "" {
				parts = append(parts, contactPart{Text: text})
			} else if address != "" {
				parts = append(parts, contactPart{Text: address})
			}
			continue
		}
		text := strings.TrimPrefix(parsed.Host, "www.") + strings.TrimSuffix(parsed.EscapedPath(), "/")
		parts = append(parts, contactPart{Text: text, Link: template.URL(parsed.String())})
	}
	return parts
}

// buildPhoneNumber is the phone as a tel: link dials it: its digits, with a
// leading "+" when it has one.
func buildPhoneNumber(phone string) string {
	var number strings.Builder
	for index, character := range phone {
		switch {
		case character >= '0' && character <= '9':
			number.WriteRune(character)
		case character == '+' && index == 0:
			number.WriteRune(character)
		}
	}
	if strings.Trim(number.String(), "+") == "" {
		return ""
	}
	return number.String()
}

// roleBullets splits a role's bullets so the print keeps the first with
// the role's heading and the last with its skills line, and lets a page end
// between the ones in the middle.
type roleBullets struct {
	First, Last       string
	HasFirst, HasLast bool
	Middle            []string
}

func splitHighlights(highlights []string) roleBullets {
	var bullets roleBullets
	if len(highlights) == 0 {
		return bullets
	}
	bullets.First, bullets.HasFirst = highlights[0], true
	if len(highlights) > 1 {
		bullets.Last, bullets.HasLast = highlights[len(highlights)-1], true
		bullets.Middle = highlights[1 : len(highlights)-1]
	}
	return bullets
}

// formatDegree is "Technical Degree in Digital Media Design".
func formatDegree(education Education) string {
	if education.Area == "" {
		return education.StudyType
	}
	if education.StudyType == "" {
		return education.Area
	}
	return education.StudyType + " in " + education.Area
}

func joinSkillNames(skills []Skill) string {
	names := make([]string, 0, len(skills))
	for _, skill := range skills {
		names = append(names, skill.Name)
	}
	return strings.Join(names, ", ")
}

func joinLanguages(languages []Language) string {
	parts := make([]string, 0, len(languages))
	for _, language := range languages {
		parts = append(parts, language.Language+": "+language.Fluency)
	}
	return strings.Join(parts, " | ")
}

//go:embed templates/cv.html
var templates embed.FS

var cvTemplate = template.Must(template.New("cv.html").Funcs(template.FuncMap{
	"formatDateRange":  formatDateRange,
	"formatLocation":   formatLocation,
	"listContactParts": listContactParts,
	"splitHighlights":  splitHighlights,
	"formatDegree":     formatDegree,
	"joinSkillNames":   joinSkillNames,
	"joinLanguages":    joinLanguages,
	"join":             strings.Join,
}).ParseFS(templates, "templates/cv.html"))

// RenderHTML renders the CV as a self-contained HTML page in the owner's
// design, ready to print to PDF.
func RenderHTML(cv Resume) ([]byte, error) {
	var page bytes.Buffer
	if err := cvTemplate.Execute(&page, cv); err != nil {
		return nil, err
	}
	return page.Bytes(), nil
}
