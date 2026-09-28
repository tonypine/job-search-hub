package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

type pipelineBoard struct {
	Phases []store.PipelinePhase `json:"phases"`
	Cards  []store.PipelineCard  `json:"cards"`
}

type applicationAnswer struct {
	Application store.Application `json:"application"`
	Created     bool              `json:"created"`
}

func readPipeline(t *testing.T, service apiUnderTest) pipelineBoard {
	t.Helper()
	status, body := send(t, http.MethodGet, service.url+"/v1/pipeline", ownerToken, "")
	var board pipelineBoard
	if err := json.Unmarshal(body, &board); status != http.StatusOK || err != nil {
		t.Fatalf("pipeline: %d %s", status, body)
	}
	return board
}

func addTestJob(t *testing.T, service apiUnderTest) store.Job {
	t.Helper()
	ctx := context.Background()
	owner := store.Actor{Kind: store.ActorOwner}
	company, _, err := service.hub.CreateCompany(ctx, owner, store.NewCompany{Name: "Acme", Domain: "acme.com"})
	if err != nil {
		t.Fatal(err)
	}
	job, _, err := service.hub.AddManualJob(ctx, owner, store.ManualJobInput{CompanyID: &company.ID, Title: "Backend Engineer", URL: "https://acme.com/careers/backend"})
	if err != nil {
		t.Fatal(err)
	}
	return job
}

func TestAJobIsAddedMovedClosedAndAnnotatedOnThePipeline(t *testing.T) {
	service := startAPI(t)
	job := addTestJob(t, service)
	addBody := `{"job_id":"` + job.ID.String() + `"}`

	status, body := send(t, http.MethodPost, service.url+"/v1/applications", ownerToken, addBody)
	var added applicationAnswer
	if err := json.Unmarshal(body, &added); status != http.StatusCreated || err != nil || !added.Created {
		t.Fatalf("add: %d %s", status, body)
	}
	status, body = send(t, http.MethodPost, service.url+"/v1/applications", ownerToken, addBody)
	var again applicationAnswer
	if err := json.Unmarshal(body, &again); status != http.StatusOK || err != nil || again.Application.ID != added.Application.ID {
		t.Fatalf("second add: %d %s, want 200 with the same application", status, body)
	}

	board := readPipeline(t, service)
	if len(board.Phases) != 6 || len(board.Cards) != 1 {
		t.Fatalf("board has %d phases and %d cards, want 6 and 1", len(board.Phases), len(board.Cards))
	}
	card := board.Cards[0]
	if card.Application.PhaseID != board.Phases[0].ID || card.JobTitle == nil || *card.JobTitle != "Backend Engineer" ||
		card.CompanyName == nil || *card.CompanyName != "Acme" {
		t.Fatalf("card = %+v", card)
	}

	applicationURL := service.url + "/v1/applications/" + added.Application.ID.String()
	closed := board.Phases[len(board.Phases)-1]
	status, body = send(t, http.MethodPatch, applicationURL, ownerToken, `{"phase_id":"`+closed.ID.String()+`","closed_reason":"Rejected after the screen"}`)
	var moved applicationAnswer
	if err := json.Unmarshal(body, &moved); status != http.StatusOK || err != nil ||
		moved.Application.PhaseID != closed.ID || moved.Application.ClosedReason != "Rejected after the screen" {
		t.Fatalf("close: %d %s", status, body)
	}
	status, body = send(t, http.MethodPatch, applicationURL, ownerToken, `{"notes":"Ask about the team size."}`)
	var annotated applicationAnswer
	if err := json.Unmarshal(body, &annotated); status != http.StatusOK || err != nil ||
		annotated.Application.Notes != "Ask about the team size." || annotated.Application.PhaseID != closed.ID {
		t.Fatalf("notes: %d %s", status, body)
	}

	for _, attempt := range []struct {
		name, url, body string
		want            int
	}{
		{"empty patch", applicationURL, `{}`, http.StatusBadRequest},
		{"unknown phase", applicationURL, `{"phase_id":"` + uuid.NewString() + `"}`, http.StatusNotFound},
		{"unknown application", service.url + "/v1/applications/" + uuid.NewString(), `{"notes":"x"}`, http.StatusNotFound},
		{"malformed id", service.url + "/v1/applications/not-an-id", `{"notes":"x"}`, http.StatusNotFound},
	} {
		if status, body := send(t, http.MethodPatch, attempt.url, ownerToken, attempt.body); status != attempt.want {
			t.Errorf("%s: %d %s, want %d", attempt.name, status, body, attempt.want)
		}
	}
	if status, _ := send(t, http.MethodPost, service.url+"/v1/applications", ownerToken, `{"job_id":"`+uuid.NewString()+`"}`); status != http.StatusNotFound {
		t.Errorf("unknown job: %d, want 404", status)
	}
}

func TestPhasesAreAddedRenamedReorderedAndDeletedOnlyWhenEmpty(t *testing.T) {
	service := startAPI(t)
	job := addTestJob(t, service)
	if status, body := send(t, http.MethodPost, service.url+"/v1/applications", ownerToken, `{"job_id":"`+job.ID.String()+`"}`); status != http.StatusCreated {
		t.Fatalf("add application: %d %s", status, body)
	}

	status, body := send(t, http.MethodPost, service.url+"/v1/pipeline/phases", ownerToken, `{"name":"Take-home"}`)
	var added store.PipelinePhase
	if err := json.Unmarshal(body, &added); status != http.StatusCreated || err != nil || added.Name != "Take-home" {
		t.Fatalf("add phase: %d %s", status, body)
	}
	if status, body := send(t, http.MethodPost, service.url+"/v1/pipeline/phases", ownerToken, `{"name":"take-home"}`); status != http.StatusConflict {
		t.Errorf("duplicate phase: %d %s, want 409", status, body)
	}
	phaseURL := service.url + "/v1/pipeline/phases/" + added.ID.String()
	status, body = send(t, http.MethodPatch, phaseURL, ownerToken, `{"name":"Take-home task"}`)
	var renamed store.PipelinePhase
	if err := json.Unmarshal(body, &renamed); status != http.StatusOK || err != nil || renamed.Name != "Take-home task" {
		t.Fatalf("rename: %d %s", status, body)
	}

	phases := readPipeline(t, service).Phases
	reversed := make([]string, 0, len(phases))
	for index := len(phases) - 1; index >= 0; index-- {
		reversed = append(reversed, `"`+phases[index].ID.String()+`"`)
	}
	orderBody := `{"phase_ids":[` + strings.Join(reversed, ",") + `]}`
	status, body = send(t, http.MethodPut, service.url+"/v1/pipeline/phases/order", ownerToken, orderBody)
	var reordered struct {
		Phases []store.PipelinePhase `json:"phases"`
	}
	if err := json.Unmarshal(body, &reordered); status != http.StatusOK || err != nil || reordered.Phases[0].ID != phases[len(phases)-1].ID {
		t.Fatalf("reorder: %d %s", status, body)
	}
	if status, _ := send(t, http.MethodPut, service.url+"/v1/pipeline/phases/order", ownerToken, `{"phase_ids":[`+reversed[0]+`]}`); status != http.StatusBadRequest {
		t.Errorf("partial order: %d, want 400", status)
	}

	saved := phases[0]
	if status, _ := send(t, http.MethodDelete, service.url+"/v1/pipeline/phases/"+saved.ID.String(), ownerToken, ""); status != http.StatusConflict {
		t.Errorf("delete the phase holding a card: %d, want 409", status)
	}
	if status, body := send(t, http.MethodDelete, phaseURL, ownerToken, ""); status != http.StatusNoContent {
		t.Fatalf("delete the empty phase: %d %s", status, body)
	}
	if count := len(readPipeline(t, service).Phases); count != 6 {
		t.Fatalf("%d phases after the delete, want 6", count)
	}
}

func TestThePipelineRoutesAreForTheOwnerOnly(t *testing.T) {
	service := startAPI(t)
	agentToken := startTriage(t, service).Token
	someID := uuid.NewString()

	for _, attempt := range []struct{ method, path, body string }{
		{http.MethodGet, "/v1/pipeline", ""},
		{http.MethodPost, "/v1/applications", `{"job_id":"` + someID + `"}`},
		{http.MethodPatch, "/v1/applications/" + someID, `{"notes":"x"}`},
		{http.MethodPost, "/v1/pipeline/phases", `{"name":"x"}`},
		{http.MethodPatch, "/v1/pipeline/phases/" + someID, `{"name":"x"}`},
		{http.MethodPut, "/v1/pipeline/phases/order", `{"phase_ids":[]}`},
		{http.MethodDelete, "/v1/pipeline/phases/" + someID, ""},
	} {
		if status, _ := send(t, attempt.method, service.url+attempt.path, agentToken, attempt.body); status != http.StatusForbidden {
			t.Errorf("agent token on %s %s: %d, want 403", attempt.method, attempt.path, status)
		}
		if status, _ := send(t, attempt.method, service.url+attempt.path, "", attempt.body); status != http.StatusUnauthorized {
			t.Errorf("no token on %s %s: %d, want 401", attempt.method, attempt.path, status)
		}
	}
}
