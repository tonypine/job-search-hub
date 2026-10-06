package api_test

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

func TestTheAnswersLibraryIsEditedAndAnImportNeverReplacesAnAnswer(t *testing.T) {
	service := startAPI(t)
	decode := func(body []byte) store.ApplicationAnswer {
		var answer store.ApplicationAnswer
		json.Unmarshal(body, &answer)
		return answer
	}

	status, body := send(t, http.MethodPost, service.url+"/v1/application-answers", ownerToken, `{"question":"Notice period","answer":"Two weeks"}`)
	notice := decode(body)
	if status != http.StatusCreated || notice.Answer != "Two weeks" || notice.Source != "owner" {
		t.Fatalf("add: %d %s", status, body)
	}
	if status, body := send(t, http.MethodPost, service.url+"/v1/application-answers", ownerToken, `{"question":"notice period?","answer":"One month"}`); status != http.StatusCreated ||
		decode(body).ID != notice.ID || decode(body).Answer != "One month" {
		t.Fatalf("the same question again: %d %s", status, body)
	}
	if status, body := send(t, http.MethodPut, service.url+"/v1/application-answers/"+notice.ID.String(), ownerToken, `{"question":"Notice period","answer":"30 days"}`); status != http.StatusOK ||
		decode(body).Answer != "30 days" {
		t.Fatalf("edit: %d %s", status, body)
	}

	csv := "Question,Answer\nNotice period,Immediately\nMobile phone number,+1 555 0100\nWhat is your level of proficiency in eng?,Professional\n"
	status, body = send(t, http.MethodPost, service.url+"/v1/linkedin/application-answers/import", ownerToken, csv)
	if status != http.StatusOK || !strings.Contains(string(body), `"added":1`) || !strings.Contains(string(body), `"already_answered":1`) {
		t.Fatalf("import: %d %s", status, body)
	}
	status, body = send(t, http.MethodGet, service.url+"/v1/application-answers", ownerToken, "")
	var listed struct {
		Answers []store.ApplicationAnswer `json:"answers"`
	}
	if err := json.Unmarshal(body, &listed); status != http.StatusOK || err != nil || len(listed.Answers) != 2 ||
		listed.Answers[0].Answer != "30 days" || listed.Answers[1].Source != "linkedin" || slices.ContainsFunc(listed.Answers, holdsThePhoneNumber) {
		t.Fatalf("list: %d %s", status, body)
	}

	if status, _ := send(t, http.MethodDelete, service.url+"/v1/application-answers/"+notice.ID.String(), ownerToken, ""); status != http.StatusNoContent {
		t.Fatalf("delete: %d", status)
	}
	agentToken := startTriage(t, service).Token
	if status, _ := send(t, http.MethodPost, service.url+"/v1/application-answers", agentToken, `{"question":"Salary","answer":"Any"}`); status != http.StatusForbidden {
		t.Errorf("an agent's save: %d, want 403", status)
	}
}

// holdsThePhoneNumber reports whether an answer carries the phone number the
// import must reject. It looks only at the question and answer, since an ID or
// an updated_at timestamp can hold "555" by chance.
func holdsThePhoneNumber(answer store.ApplicationAnswer) bool {
	return strings.Contains(answer.Question, "555 0100") || strings.Contains(answer.Answer, "555 0100")
}
