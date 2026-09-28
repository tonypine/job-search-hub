package api_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

type streamedEvent struct {
	id, name, data string
}

// openEventStream connects to the stream and returns its events as they
// arrive.
func openEventStream(t *testing.T, service apiUnderTest, lastEventID string) <-chan streamedEvent {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, service.url+"/v1/events", nil)
	request.Header.Set("Authorization", "Bearer "+ownerToken)
	if lastEventID != "" {
		request.Header.Set("Last-Event-ID", lastEventID)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil || response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("open the stream: %v %v", err, response)
	}
	events := make(chan streamedEvent, 16)
	go func() {
		defer response.Body.Close()
		scanner := bufio.NewScanner(response.Body)
		var event streamedEvent
		for scanner.Scan() {
			line := scanner.Text()
			switch {
			case strings.HasPrefix(line, "id: "):
				event.id = strings.TrimPrefix(line, "id: ")
			case strings.HasPrefix(line, "event: "):
				event.name = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				event.data = strings.TrimPrefix(line, "data: ")
			case line == "" && event.name != "":
				events <- event
				event = streamedEvent{}
			}
		}
	}()
	return events
}

func nextEvent(t *testing.T, events <-chan streamedEvent) streamedEvent {
	t.Helper()
	select {
	case event := <-events:
		return event
	case <-time.After(3 * time.Second):
		t.Fatal("no event within 3 seconds")
		return streamedEvent{}
	}
}

func TestARecordedUpdateIsStreamedAndAReconnectCatchesUp(t *testing.T) {
	service := startAPI(t)
	events := openEventStream(t, service, "")
	time.Sleep(100 * time.Millisecond)

	if status, body := send(t, http.MethodPost, service.url+"/v1/updates", ownerToken, `{"kind":"reply","title":"Acme replied"}`); status != http.StatusCreated {
		t.Fatalf("record: %d %s", status, body)
	}
	first := nextEvent(t, events)
	var update store.Update
	if err := json.Unmarshal([]byte(first.data), &update); err != nil || first.name != "update" || update.Title != "Acme replied" || first.id == "" {
		t.Fatalf("event = %+v, %v", first, err)
	}

	// Two more while nobody listens; a reconnect from the first gets both.
	for _, title := range []string{"Second", "Third"} {
		send(t, http.MethodPost, service.url+"/v1/updates", ownerToken, `{"kind":"note","title":"`+title+`"}`)
	}
	resumed := openEventStream(t, service, first.id)
	for _, want := range []string{"Second", "Third"} {
		if event := nextEvent(t, resumed); !strings.Contains(event.data, want) {
			t.Fatalf("resumed event = %+v, want %s", event, want)
		}
	}
}

func TestTheEventStreamIsForTheOwnerOnly(t *testing.T) {
	service := startAPI(t)
	agentToken := startTriage(t, service).Token

	if status, _ := send(t, http.MethodGet, service.url+"/v1/events", agentToken, ""); status != http.StatusForbidden {
		t.Errorf("agent token: %d, want 403", status)
	}
	if status, _ := send(t, http.MethodGet, service.url+"/v1/events", "", ""); status != http.StatusUnauthorized {
		t.Errorf("no token: %d, want 401", status)
	}
}
