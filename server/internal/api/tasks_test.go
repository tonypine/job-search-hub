package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

func TestThePhoneQueuesWorkTheMacClaimsOnceAndFinishes(t *testing.T) {
	service := startAPI(t)
	ctx := context.Background()
	company, _, _ := service.hub.CreateCompany(ctx, store.Actor{Kind: store.ActorOwner}, store.NewCompany{Name: "Acme", Domain: "acme.com"})
	_, body := send(t, http.MethodPost, service.url+"/v1/devices", ownerToken, `{"name":"Sam's phone"}`)
	var paired struct {
		Token string `json:"token"`
	}
	json.Unmarshal(body, &paired)

	status, body := send(t, http.MethodPost, service.url+"/v1/tasks", paired.Token, `{"kind":"find_jobs","company_id":"`+company.ID.String()+`"}`)
	var task store.TaskRequest
	if err := json.Unmarshal(body, &task); status != http.StatusCreated || err != nil || task.Status != store.TaskQueued {
		t.Fatalf("queue: %d %s", status, body)
	}
	if status, _ := send(t, http.MethodPost, service.url+"/v1/tasks", paired.Token, `{"kind":"research_company"}`); status != http.StatusBadRequest {
		t.Errorf("research without a company: %d, want 400", status)
	}

	status, body = send(t, http.MethodGet, service.url+"/v1/tasks?status=queued", ownerToken, "")
	if status != http.StatusOK || !strings.Contains(string(body), task.ID.String()) {
		t.Fatalf("queued: %d %s", status, body)
	}
	if status, _ := send(t, http.MethodPost, service.url+"/v1/tasks/"+task.ID.String()+"/claim", ownerToken, ""); status != http.StatusOK {
		t.Fatalf("claim: %d", status)
	}
	if status, _ := send(t, http.MethodPost, service.url+"/v1/tasks/"+task.ID.String()+"/claim", ownerToken, ""); status != http.StatusConflict {
		t.Errorf("a second claim: %d, want 409", status)
	}
	status, body = send(t, http.MethodPost, service.url+"/v1/tasks/"+task.ID.String()+"/finish", ownerToken, `{"succeeded":true,"result":"Acme has 3 open jobs\nRead from its Workable board."}`)
	if status != http.StatusOK || !strings.Contains(string(body), `"status":"succeeded"`) {
		t.Fatalf("finish: %d %s", status, body)
	}

	updates, err := service.hub.ListUpdates(ctx, 10, false)
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, update := range updates.Updates {
		titles = append(titles, update.Title)
	}
	if len(titles) != 2 || titles[0] != "Acme has 3 open jobs" || titles[1] != "From your phone: find jobs at Acme" {
		t.Errorf("updates = %q", titles)
	}
	if finished := updates.Updates[0]; finished.Body != "Read from its Workable board." {
		t.Errorf("finished body = %q", finished.Body)
	}
}

func TestAJobFixIsQueuedWithItsJobAndNote(t *testing.T) {
	service := startAPI(t)
	ctx := context.Background()
	job, _, _ := service.hub.AddManualJob(ctx, store.Actor{Kind: store.ActorOwner}, store.ManualJobInput{Title: "Frontend Engineer - Track&Field", URL: "https://indeed.example/7"})

	status, body := send(t, http.MethodPost, service.url+"/v1/tasks", ownerToken, `{"kind":"fix_job","job_id":"`+job.ID.String()+`","note":" the company is Track&Field "}`)
	var task store.TaskRequest
	if err := json.Unmarshal(body, &task); status != http.StatusCreated || err != nil || task.JobID == nil || *task.JobID != job.ID || task.Input != "the company is Track&Field" {
		t.Fatalf("queue: %d %s", status, body)
	}
	updates, _ := service.hub.ListUpdates(ctx, 10, false)
	if len(updates.Updates) != 1 || updates.Updates[0].Title != "Fix Frontend Engineer - Track&Field: the company is Track&Field" {
		t.Errorf("updates = %+v", updates.Updates)
	}
	if status, _ := send(t, http.MethodPost, service.url+"/v1/tasks", ownerToken, `{"kind":"fix_job","job_id":"`+job.ID.String()+`"}`); status != http.StatusBadRequest {
		t.Errorf("a fix without a note: %d, want 400", status)
	}
	if status, _ := send(t, http.MethodPost, service.url+"/v1/tasks", ownerToken, `{"kind":"fix_job","job_id":"7c9e6679-7425-40de-944b-e07fc1f90ae7","note":"x"}`); status != http.StatusNotFound {
		t.Errorf("a fix for an unknown job: %d, want 404", status)
	}
}
