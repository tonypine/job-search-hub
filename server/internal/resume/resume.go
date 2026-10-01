// Package resume keeps a CV as JSON Resume content (jsonresume.org) and
// renders it as HTML in the owner's CV design.
package resume

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
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
	Name     string   `json:"name"`
	Label    string   `json:"label"`
	Location Location `json:"location"`
	Summary  string   `json:"summary"`
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
	"formatDateRange": formatDateRange,
	"formatLocation":  formatLocation,
	"formatDegree":    formatDegree,
	"joinSkillNames":  joinSkillNames,
	"joinLanguages":   joinLanguages,
	"join":            strings.Join,
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
