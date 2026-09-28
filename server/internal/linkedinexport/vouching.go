package linkedinexport

import (
	"errors"
	"io"
	"strings"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

// ErrNotVouchingFile means the file lacks the columns of an endorsement or a
// recommendation file.
var ErrNotVouchingFile = errors.New("not a LinkedIn endorsement or recommendation file")

const (
	endorsementDateLayout    = "2006/01/02 15:04:05 MST"
	recommendationDateLayout = "01/02/06, 3:04 PM"
)

// ParseEndorsementsReceived reads Endorsement_Received_Info.csv.
func ParseEndorsementsReceived(file io.Reader) ([]store.NewLinkedInEndorsement, error) {
	return parseEndorsements(file, store.VouchedReceived, "Endorser")
}

// ParseEndorsementsGiven reads Endorsement_Given_Info.csv.
func ParseEndorsementsGiven(file io.Reader) ([]store.NewLinkedInEndorsement, error) {
	return parseEndorsements(file, store.VouchedGiven, "Endorsee")
}

func parseEndorsements(file io.Reader, direction, person string) ([]store.NewLinkedInEndorsement, error) {
	rows, columns, err := readTable(file, person+" Public Url")
	if errors.Is(err, errMissingColumn) {
		return nil, ErrNotVouchingFile
	}
	if err != nil {
		return nil, err
	}
	var endorsements []store.NewLinkedInEndorsement
	for _, row := range rows {
		field := columns.reader(row)
		endorsement := store.NewLinkedInEndorsement{
			Direction: direction, Skill: field("Skill Name"), FirstName: field(person + " First Name"), LastName: field(person + " Last Name"),
			ProfileURL: addScheme(field(person + " Public Url")), Status: strings.ToLower(field("Endorsement Status")),
		}
		if at, err := time.Parse(endorsementDateLayout, field("Endorsement Date")); err == nil {
			endorsement.EndorsedAt = &at
		}
		if endorsement.Skill == "" || endorsement.ProfileURL == "" {
			continue
		}
		endorsements = append(endorsements, endorsement)
	}
	return endorsements, nil
}

// ParseRecommendationsReceived reads Recommendations_Received.csv.
func ParseRecommendationsReceived(file io.Reader) ([]store.NewLinkedInRecommendation, error) {
	return parseRecommendations(file, store.VouchedReceived)
}

// ParseRecommendationsGiven reads Recommendations_Given.csv.
func ParseRecommendationsGiven(file io.Reader) ([]store.NewLinkedInRecommendation, error) {
	return parseRecommendations(file, store.VouchedGiven)
}

func parseRecommendations(file io.Reader, direction string) ([]store.NewLinkedInRecommendation, error) {
	rows, columns, err := readTable(file, "Text")
	if errors.Is(err, errMissingColumn) {
		return nil, ErrNotVouchingFile
	}
	if err != nil {
		return nil, err
	}
	var recommendations []store.NewLinkedInRecommendation
	for _, row := range rows {
		field := columns.reader(row)
		recommendation := store.NewLinkedInRecommendation{
			Direction: direction, FirstName: field("First Name"), LastName: field("Last Name"), Company: field("Company"),
			JobTitle: field("Job Title"), Text: field("Text"), Status: strings.ToLower(field("Status")),
		}
		if at, err := time.Parse(recommendationDateLayout, field("Creation Date")); err == nil {
			recommendation.WrittenAt = &at
		}
		if recommendation.Text == "" {
			continue
		}
		recommendations = append(recommendations, recommendation)
	}
	return recommendations, nil
}

// addScheme turns "www.linkedin.com/in/x", as the endorsement files write
// profiles, into the https URL the rest of the export uses.
func addScheme(url string) string {
	if url == "" || strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		return url
	}
	return "https://" + url
}
