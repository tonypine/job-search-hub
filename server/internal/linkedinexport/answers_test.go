package linkedinexport_test

import (
	"strings"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/linkedinexport"
)

func TestOnlyRealAnswersAreKeptFromSavedAnswers(t *testing.T) {
	file := "Question,Answer\n" +
		"First name,Sam\nMobile phone number,+1 555 0100\nStreet address line 1,1 Example Road\n" +
		"Company,Acme\nYour title,Engineer\nDates of employment,2020 - 2024\n" +
		"What is your level of proficiency in eng?,Professional\nCover letter,\"Dear team,\nI build things.\"\nNotice period,\n"

	answers, err := linkedinexport.ParseApplicationAnswers(strings.NewReader(file))

	if err != nil || len(answers) != 2 || answers[0].Question != "What is your level of proficiency in eng?" || answers[0].Answer != "Professional" ||
		answers[1].Question != "Cover letter" || !strings.Contains(answers[1].Answer, "I build things.") {
		t.Fatalf("answers = %+v, %v", answers, err)
	}
	if _, err := linkedinexport.ParseApplicationAnswers(strings.NewReader("Name,Value\nx,y\n")); err != linkedinexport.ErrNotAnswersFile {
		t.Errorf("another file: %v", err)
	}
}
