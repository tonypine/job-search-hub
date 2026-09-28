package linkedinexport

import (
	"fmt"
	"strings"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

// preferenceSeparator joins the values of one job-seeker preference.
const preferenceSeparator = " | "

// ProfileFileNames are the files of the export that make up the profile.
var ProfileFileNames = []string{
	"Profile.csv", "Profile Summary.csv", "Positions.csv", "Skills.csv", "Education.csv", "Languages.csv", "Projects.csv",
	"Courses.csv", "Job Seeker Preferences.csv",
}

// ApplyProfileFiles reads each profile file given, by name, into profile,
// replacing the part it holds and keeping the parts of files not given. It
// returns the names of the files it read. Address, birth date and phone are
// never read.
func ApplyProfileFiles(profile *store.LinkedInProfile, files map[string]string) ([]string, error) {
	var read []string
	for _, name := range ProfileFileNames {
		content, given := files[name]
		if !given {
			continue
		}
		if err := applyProfileFile(profile, name, content); err != nil {
			return read, fmt.Errorf("read %s: %w", name, err)
		}
		read = append(read, name)
	}
	return read, nil
}

func applyProfileFile(profile *store.LinkedInProfile, name, content string) error {
	firstColumn := map[string]string{
		"Profile.csv": "Headline", "Profile Summary.csv": "Profile Summary", "Positions.csv": "Company Name", "Skills.csv": "Name",
		"Education.csv": "School Name", "Languages.csv": "Name", "Projects.csv": "Title", "Courses.csv": "Name",
		"Job Seeker Preferences.csv": "Job Titles",
	}[name]
	rows, columns, err := readTable(strings.NewReader(content), firstColumn)
	if err != nil {
		return err
	}
	switch name {
	case "Profile.csv":
		for _, row := range rows {
			field := columns.reader(row)
			profile.Headline, profile.Industry, profile.Location = field("Headline"), field("Industry"), field("Geo Location")
			profile.Websites = splitValues(field("Websites"), ",")
			if summary := field("Summary"); summary != "" {
				profile.Summary = summary
			}
		}
	case "Profile Summary.csv":
		for _, row := range rows {
			if summary := columns.reader(row)("Profile Summary"); summary != "" {
				profile.Summary = summary
			}
		}
	case "Positions.csv":
		profile.Positions = nil
		for _, row := range rows {
			field := columns.reader(row)
			profile.Positions = append(profile.Positions, store.LinkedInPosition{
				Company: field("Company Name"), Title: field("Title"), Description: field("Description"), Location: field("Location"),
				StartedOn: field("Started On"), FinishedOn: field("Finished On"),
			})
		}
	case "Skills.csv":
		profile.Skills = readColumn(rows, columns, "Name")
	case "Education.csv":
		profile.Education = nil
		for _, row := range rows {
			field := columns.reader(row)
			profile.Education = append(profile.Education, store.LinkedInEducation{
				School: field("School Name"), Degree: field("Degree Name"), StartDate: field("Start Date"), EndDate: field("End Date"),
			})
		}
	case "Languages.csv":
		profile.Languages = nil
		for _, row := range rows {
			field := columns.reader(row)
			profile.Languages = append(profile.Languages, store.LinkedInLanguage{Name: field("Name"), Proficiency: field("Proficiency")})
		}
	case "Projects.csv":
		profile.Projects = nil
		for _, row := range rows {
			field := columns.reader(row)
			profile.Projects = append(profile.Projects, store.LinkedInProject{Title: field("Title"), Description: field("Description"), URL: field("Url")})
		}
	case "Courses.csv":
		profile.Courses = readColumn(rows, columns, "Name")
	case "Job Seeker Preferences.csv":
		for _, row := range rows {
			field := columns.reader(row)
			profile.Preferences = &store.LinkedInPreferences{
				JobTitles: splitValues(field("Job Titles"), preferenceSeparator), Locations: splitValues(field("Locations"), preferenceSeparator),
				Industries: splitValues(field("Industries"), preferenceSeparator), JobTypes: splitValues(field("Preferred Job Types"), preferenceSeparator),
				CompanySize: field("Company Employee Count"), OpenToRecruiters: field("Open To Recruiters"),
			}
		}
	}
	return nil
}

func readColumn(rows [][]string, columns tableColumns, name string) []string {
	var values []string
	for _, row := range rows {
		if value := columns.reader(row)(name); value != "" {
			values = append(values, value)
		}
	}
	return values
}

func splitValues(text, separator string) []string {
	var values []string
	for _, value := range strings.Split(text, separator) {
		if value = strings.TrimSpace(value); value != "" {
			values = append(values, value)
		}
	}
	return values
}
