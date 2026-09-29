package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

func TestTaskRunsAreListedLatestFirstByKind(t *testing.T) {
	service := startAPI(t)
	ctx := context.Background()
	for index, run := range []store.NewTaskRun{
		{Kind: "job_facts", Model: "qwen", Outcome: "succeeded", Output: json.RawMessage(`{"seniority":"Senior"}`), PromptTokens: 900},
		{Kind: "mail_triage", Model: "qwen", Outcome: "failed", Error: "the model server is unreachable"},
		{Kind: "job_facts", Model: "qwen", Outcome: "invalid", Error: "the answer is not a JSON object"},
	} {
		run.StartedAt = time.Now().Add(time.Duration(index) * time.Minute)
		run.Duration = 2 * time.Second
		if _, err := service.hub.RecordTaskRun(ctx, run); err != nil {
			t.Fatal(err)
		}
	}

	status, answer := send(t, http.MethodGet, service.url+"/v1/task-runs?kind=job_facts", ownerToken, "")
	var listed struct {
		Runs []store.TaskRun `json:"runs"`
	}
	if json.Unmarshal(answer, &listed); status != http.StatusOK || len(listed.Runs) != 2 {
		t.Fatalf("job facts runs: %d %s", status, answer)
	}
	if listed.Runs[0].Outcome != "invalid" || listed.Runs[1].PromptTokens != 900 || listed.Runs[1].DurationMS != 2000 {
		t.Errorf("runs = %+v", listed.Runs)
	}
	status, answer = send(t, http.MethodGet, service.url+"/v1/task-runs?outcome=failed", ownerToken, "")
	if json.Unmarshal(answer, &listed); status != http.StatusOK || len(listed.Runs) != 1 || listed.Runs[0].Kind != "mail_triage" {
		t.Errorf("failed runs: %d %s", status, answer)
	}
}
