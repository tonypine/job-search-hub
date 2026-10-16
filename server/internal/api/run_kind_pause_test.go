package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/modelcontextprotocol/go-sdk/auth"

	"github.com/tonypine/job-search-hub/server/internal/api"
	"github.com/tonypine/job-search-hub/server/internal/drain"
	"github.com/tonypine/job-search-hub/server/internal/modelqueue"
	"github.com/tonypine/job-search-hub/server/internal/modelwork"
	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
	"github.com/tonypine/job-search-hub/server/internal/tokens"
)

type runKindPauseAPI struct {
	url   string
	pool  *pgxpool.Pool
	hub   *store.Store
	queue *modelqueue.Queue
}

// startRunKindPauseAPI serves the run kind and model work routes over a
// queue that starts with the hub's saved pauses, as the hub does.
func startRunKindPauseAPI(t *testing.T) runKindPauseAPI {
	t.Helper()
	pool := testdatabase.New(t)
	hub := store.New(pool)
	requireOwner := auth.RequireBearerToken(tokens.NewVerifier(ownerToken, hub), &auth.RequireBearerTokenOptions{
		Scopes: []string{tokens.ScopeOwner}, AllowMissingExpiration: true,
	})
	pauses, err := hub.GetRunKindPauses(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	queue := modelqueue.New(pauses, modelqueue.Settings{})
	controls := &modelwork.Controls{Hub: hub, Queue: queue, Runtime: stubRuntime{}, Drain: drain.New(time.Hour)}
	routes := http.NewServeMux()
	api.RegisterRunKindRoutes(routes, controls, requireOwner)
	api.RegisterModelWorkRoutes(routes, controls, requireOwner)
	server := httptest.NewServer(routes)
	t.Cleanup(server.Close)
	return runKindPauseAPI{url: server.URL, pool: pool, hub: hub, queue: queue}
}

// sendForKinds sends the request and reads the kinds it answers with.
func sendForKinds(t *testing.T, method, url string) map[string]modelwork.RunKindState {
	t.Helper()
	status, answer := send(t, method, url, ownerToken, "")
	var listed struct {
		Kinds []modelwork.RunKindState `json:"kinds"`
	}
	if err := json.Unmarshal(answer, &listed); err != nil || status != http.StatusOK {
		t.Fatalf("%s %s: %d %s", method, url, status, answer)
	}
	kinds := map[string]modelwork.RunKindState{}
	for _, kind := range listed.Kinds {
		kinds[kind.ID] = kind
	}
	return kinds
}

// waitForTurn queues a background call of kind and hands over its release
// once it gets its turn.
func waitForTurn(queue *modelqueue.Queue, kind string) <-chan func() {
	granted := make(chan func(), 1)
	go func() {
		if release, err := queue.Wait(context.Background(), modelqueue.Ticket{Kind: kind, Model: "m", Priority: modelqueue.PriorityBackground}); err == nil {
			granted <- release
		}
	}()
	return granted
}

func receiveTurn(t *testing.T, granted <-chan func(), kind string) func() {
	t.Helper()
	select {
	case release := <-granted:
		return release
	case <-time.After(2 * time.Second):
		t.Fatalf("%s never got its turn", kind)
		return nil
	}
}

func TestPausingJobBriefsHoldsNewBriefsWhileJobFactsRun(t *testing.T) {
	service := startRunKindPauseAPI(t)
	if kinds := sendForKinds(t, http.MethodPost, service.url+"/v1/run-kinds/resume-all"); kinds[store.AgentPromptKindJobBrief].Paused {
		t.Fatal("resume all left job briefs paused")
	}

	kinds := sendForKinds(t, http.MethodPost, service.url+"/v1/run-kinds/job_brief/pause")
	if !kinds[store.AgentPromptKindJobBrief].Paused || kinds[store.AgentPromptKindJobFacts].Paused {
		t.Fatalf("after pausing job briefs: %+v", kinds)
	}
	brief := waitForTurn(service.queue, store.AgentPromptKindJobBrief)
	facts := waitForTurn(service.queue, store.AgentPromptKindJobFacts)
	releaseFacts := receiveTurn(t, facts, "job facts")
	select {
	case <-brief:
		t.Fatal("a brief started while job briefs are paused")
	case <-time.After(100 * time.Millisecond):
	}
	kinds = sendForKinds(t, http.MethodGet, service.url+"/v1/run-kinds")
	if kinds[store.AgentPromptKindJobFacts].Running != 1 || kinds[store.AgentPromptKindJobBrief].Waiting != 1 || !kinds[store.AgentPromptKindJobBrief].Paused {
		t.Fatalf("the kinds list = %+v", kinds)
	}
	releaseFacts()

	if kinds := sendForKinds(t, http.MethodPost, service.url+"/v1/run-kinds/job_brief/resume"); kinds[store.AgentPromptKindJobBrief].Paused {
		t.Fatal("job briefs still paused after the resume")
	}
	receiveTurn(t, brief, "the waiting brief")()
	if pauses, _ := service.hub.GetRunKindPauses(context.Background()); pauses[store.AgentPromptKindJobBrief] {
		t.Fatal("the resume wasn't saved")
	}
}

func TestPauseAllThenResumeOneAndOnDemandKindsRefuseAPause(t *testing.T) {
	service := startRunKindPauseAPI(t)
	sendForKinds(t, http.MethodPost, service.url+"/v1/run-kinds/resume-all")
	for id, kind := range sendForKinds(t, http.MethodPost, service.url+"/v1/run-kinds/pause-all") {
		if kind.Paused != kind.Automatic {
			t.Errorf("after pause all, %s paused = %v", id, kind.Paused)
		}
	}
	for id, kind := range sendForKinds(t, http.MethodPost, service.url+"/v1/run-kinds/mail_triage/resume") {
		if kind.Paused != (kind.Automatic && id != store.AgentPromptKindMailTriage) {
			t.Errorf("after resuming mail sorting, %s paused = %v", id, kind.Paused)
		}
	}
	pauses, _ := service.hub.GetRunKindPauses(context.Background())
	if pauses[store.AgentPromptKindMailTriage] || !pauses[store.AgentPromptKindJobFacts] {
		t.Fatalf("saved pauses = %v", pauses)
	}

	if status, answer := send(t, http.MethodPost, service.url+"/v1/run-kinds/job_finder/pause", ownerToken, ""); status != http.StatusConflict {
		t.Errorf("pausing find jobs: %d %s", status, answer)
	}
	if status, _ := send(t, http.MethodPost, service.url+"/v1/run-kinds/no_such_kind/pause", ownerToken, ""); status != http.StatusNotFound {
		t.Errorf("pausing an unknown kind: %d", status)
	}
	if status, _ := send(t, http.MethodPost, service.url+"/v1/run-kinds/pause-all", "", ""); status != http.StatusUnauthorized {
		t.Errorf("without the owner token: %d", status)
	}

	// Each change names who made it: a resume and a pause of every kind,
	// then mail sorting's resume.
	var ownerChanges int
	if err := service.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM changes WHERE entity_type = 'model_work' AND actor_kind = 'owner'`).Scan(&ownerChanges); err != nil {
		t.Fatal(err)
	}
	if want := 2*len(store.ListAutomaticRunKindIDs()) + 1; ownerChanges != want {
		t.Errorf("recorded %d changes by the owner, want %d", ownerChanges, want)
	}
}

func TestThePauseOfModelWorkPausesAndResumesEveryKind(t *testing.T) {
	service := startRunKindPauseAPI(t)
	sendForKinds(t, http.MethodPost, service.url+"/v1/run-kinds/resume-all")
	send(t, http.MethodPost, service.url+"/v1/model-work/pause", ownerToken, "")
	for id, kind := range sendForKinds(t, http.MethodGet, service.url+"/v1/run-kinds") {
		if kind.Paused != kind.Automatic {
			t.Errorf("after the model work pause, %s paused = %v", id, kind.Paused)
		}
	}
	send(t, http.MethodPost, service.url+"/v1/model-work/resume", ownerToken, "")
	for id, kind := range sendForKinds(t, http.MethodGet, service.url+"/v1/run-kinds") {
		if kind.Paused {
			t.Errorf("after the model work resume, %s is paused", id)
		}
	}
}
