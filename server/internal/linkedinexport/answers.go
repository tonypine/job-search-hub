package linkedinexport

import (
	"errors"
	"io"
	"strings"

	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/wordmatch"
)

// ErrNotAnswersFile means the file lacks the columns of LinkedIn's saved
// answers files.
var ErrNotAnswersFile = errors.New(`not a LinkedIn Job Applicant Saved Answers or Screening Question Responses file: no "Question" column`)

// unkeptQuestions are the saved answers that aren't answers: contact details,
// which the hub never stores, and copies of the resume's jobs and schools.
var unkeptQuestions = map[string]bool{}

func init() {
	for _, question := range []string{
		"First name", "Last name", "Mobile phone number", "Phone", "Email address", "Street address line 1", "Street address line 2",
		"City", "State / Province", "ZIP / Postal Code", "Country", "Date of birth",
		"Dates of employment", "Company", "Your title", "Description", "Degree", "Major / Field of study", "School", "Dates attended",
		"Please submit a resume or LinkedIn profile",
	} {
		unkeptQuestions[wordmatch.Normalize(question)] = true
	}
}

// ParseApplicationAnswers reads Job Applicant Saved Answers.csv or Job
// Applicant Saved Screening Question Responses.csv, keeping the questions
// with an answer that are neither contact details nor resume copies.
func ParseApplicationAnswers(file io.Reader) ([]store.NewApplicationAnswer, error) {
	rows, columns, err := readTable(file, "Question")
	if errors.Is(err, errMissingColumn) {
		return nil, ErrNotAnswersFile
	}
	if err != nil {
		return nil, err
	}
	var answers []store.NewApplicationAnswer
	for _, row := range rows {
		field := columns.reader(row)
		question, answer := strings.TrimSpace(field("Question")), strings.TrimSpace(field("Answer"))
		if question == "" || answer == "" || unkeptQuestions[wordmatch.Normalize(question)] {
			continue
		}
		answers = append(answers, store.NewApplicationAnswer{Question: question, Answer: answer})
	}
	return answers, nil
}
