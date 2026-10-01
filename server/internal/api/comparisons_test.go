package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/auth"

	"github.com/tonypine/job-search-hub/server/internal/api"
	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
	"github.com/tonypine/job-search-hub/server/internal/tokens"
)

type recordingComparisonRunner struct{ ran chan uuid.UUID }

func (runner recordingComparisonRunner) Run(_ context.Context, comparisonID uuid.UUID) error {
	runner.ran <- comparisonID
	return nil
}

type comparisonAnswer struct {
	ID     uuid.UUID   `json:"id"`
	Status string      `json:"status"`
	JobIDs []uuid.UUID `json:"job_ids"`
	Jobs   []struct {
		Title string `json:"title"`
	} `json:"jobs"`
	Answers []struct {
		Readings []struct {
			Field    string `json:"field"`
			Text     string `json:"text"`
			Evidence string `json:"evidence"`
		} `json:"readings"`
	} `json:"answers"`
	Stacks []struct {
		ID uuid.UUID `json:"id"`
	} `json:"stacks"`
	Summary struct {
		Stacks []struct {
			Fields []struct {
				Field    string `json:"field"`
				Agreed   int    `json:"agreed"`
				Compared int    `json:"compared"`
				Right    int    `json:"right"`
			} `json:"fields"`
		} `json:"stacks"`
	} `json:"summary"`
}

func TestAnImportedComparisonIsSummarizedAndJudged(t *testing.T) {
	hub := store.New(testdatabase.New(t))
	requireOwner := auth.RequireBearerToken(tokens.NewVerifier(ownerToken, hub), &auth.RequireBearerTokenOptions{
		Scopes: []string{tokens.ScopeOwner}, AllowMissingExpiration: true,
	})
	runner := recordingComparisonRunner{ran: make(chan uuid.UUID, 1)}
	routes := http.NewServeMux()
	api.RegisterComparisonRoutes(routes, hub, runner, requireOwner)
	server := httptest.NewServer(routes)
	t.Cleanup(server.Close)
	owner := store.Actor{Kind: store.ActorOwner}
	first, _, _ := hub.AddManualJob(context.Background(), owner, store.ManualJobInput{Title: "Engineer", URL: "https://acme.com/1", Description: "Go."})
	second, _, _ := hub.AddManualJob(context.Background(), owner, store.ManualJobInput{Title: "Lead", URL: "https://acme.com/2", Description: "Rust."})

	body := `{"task_kind":"job_facts","title":"Key against local","job_ids":["` + first.ID.String() + `","` + second.ID.String() + `"],"stacks":[
		{"label":"Key","answers":[{"job_id":"` + first.ID.String() + `","answer":{"stack":{"value":"Go","evidence":"Go."}}},{"job_id":"` + second.ID.String() + `","answer":{"stack":{"value":"Rust"}}}]},
		{"label":"Local","answers":[{"job_id":"` + first.ID.String() + `","answer":{"stack":{"value":"go","evidence":"We use Go"}}},{"job_id":"` + second.ID.String() + `","error":"timed out"}]}]}`
	status, answer := send(t, http.MethodPost, server.URL+"/v1/comparisons/import", ownerToken, body)
	if status != http.StatusCreated {
		t.Fatalf("import: %d %s", status, answer)
	}
	var imported comparisonAnswer
	if err := json.Unmarshal([]byte(answer), &imported); err != nil {
		t.Fatal(err)
	}
	local := imported.Summary.Stacks[1].Fields[0]
	if imported.Status != store.ComparisonStatusDone || local.Field != "stack.value" || local.Agreed != 1 || local.Compared != 1 {
		t.Fatalf("imported = %s", answer)
	}
	if len(imported.Jobs) != 2 || imported.Jobs[0].Title != "Engineer" {
		t.Errorf("jobs = %+v", imported.Jobs)
	}
	var evidence []string
	for _, imported := range imported.Answers {
		for _, reading := range imported.Readings {
			evidence = append(evidence, reading.Field+"="+reading.Text+"|"+reading.Evidence)
		}
	}
	if !slices.Contains(evidence, "stack.value=go|We use Go") {
		t.Errorf("readings = %v, want the local answer's with its evidence", evidence)
	}

	verdict := `{"verdicts":[{"stack_id":"` + imported.Stacks[1].ID.String() + `","job_id":"` + first.ID.String() + `","field":"stack.value","verdict":"right"}]}`
	status, answer = send(t, http.MethodPut, server.URL+"/v1/comparisons/"+imported.ID.String()+"/verdicts", ownerToken, verdict)
	var judged comparisonAnswer
	json.Unmarshal([]byte(answer), &judged)
	if status != http.StatusOK || judged.Summary.Stacks[1].Fields[0].Right != 1 {
		t.Fatalf("verdict: %d %s", status, answer)
	}
	if status, _ := send(t, http.MethodPut, server.URL+"/v1/comparisons/"+uuid.NewString()+"/verdicts", ownerToken, verdict); status != http.StatusNotFound {
		t.Errorf("a verdict on another comparison: %d, want 404", status)
	}

	run := `{"title":"Fresh run","fresh_jobs":1,"stacks":[{"label":"Sonnet","source":"claude","model":"sonnet"},{"label":"Opus","source":"claude","model":"opus"}]}`
	status, answer = send(t, http.MethodPost, server.URL+"/v1/comparisons", ownerToken, run)
	var started comparisonAnswer
	json.Unmarshal([]byte(answer), &started)
	if status != http.StatusAccepted || len(started.JobIDs) != 1 || started.JobIDs[0] != second.ID {
		t.Fatalf("run: %d %s, want the newest job", status, answer)
	}
	select {
	case ran := <-runner.ran:
		if ran != started.ID {
			t.Errorf("ran %s, want %s", ran, started.ID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the comparison never ran")
	}
	if status, _ := send(t, http.MethodPost, server.URL+"/v1/comparisons", ownerToken, `{"title":"Bad","fresh_jobs":1,"stacks":[{"label":"A","source":"imported"},{"label":"B","source":"claude"}]}`); status != http.StatusBadRequest {
		t.Errorf("an imported stack in a run: %d, want 400", status)
	}
	status, answer = send(t, http.MethodGet, server.URL+"/v1/comparisons", ownerToken, "")
	var list struct {
		Comparisons []comparisonAnswer `json:"comparisons"`
	}
	json.Unmarshal([]byte(answer), &list)
	if status != http.StatusOK || len(list.Comparisons) != 2 || list.Comparisons[0].ID != started.ID {
		t.Errorf("list: %d %s, want both, the newest first", status, answer)
	}
}
