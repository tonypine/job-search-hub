package push

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"golang.org/x/oauth2"

	"github.com/tonypine/job-search-hub/server/internal/hubevents"
	"github.com/tonypine/job-search-hub/server/internal/store"
	"github.com/tonypine/job-search-hub/server/internal/testdatabase"
	"github.com/tonypine/job-search-hub/server/internal/tokens"
)

const goneToken = "gone-token"

// fakeFCM answers like FCM's v1 send endpoint: 404 UNREGISTERED for
// goneToken, 200 for any other.
type fakeFCM struct {
	mutex    sync.Mutex
	requests []sendRequest
	headers  []string
	paths    []string
}

func (fake *fakeFCM) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var request sendRequest
	json.Unmarshal(body, &request)
	fake.mutex.Lock()
	fake.requests = append(fake.requests, request)
	fake.headers = append(fake.headers, r.Header.Get("Authorization"))
	fake.paths = append(fake.paths, r.URL.Path)
	fake.mutex.Unlock()
	if request.Message.Token == goneToken {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, `{"error":{"code":404,"status":"NOT_FOUND","message":"Requested entity was not found.","details":[{"@type":"type.googleapis.com/google.firebase.fcm.v1.FcmError","errorCode":"UNREGISTERED"}]}}`)
		return
	}
	io.WriteString(w, `{"name":"projects/test-project/messages/1"}`)
}

func startFakeFCM(t *testing.T) (*fakeFCM, *Sender) {
	fake := &fakeFCM{}
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	return fake, &Sender{
		projectID: "test-project",
		tokens:    oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "test-access", TokenType: "Bearer"}),
		client:    server.Client(),
		baseURL:   server.URL,
	}
}

func TestSendPostsADataMessageToTheProject(t *testing.T) {
	fake, sender := startFakeFCM(t)
	err := sender.Send(context.Background(), "phone-token", Message{UpdateID: "u1", Kind: "human_reply", Title: "Acme replied", Body: "They'd like a call.", CompanyID: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(fake.requests) != 1 {
		t.Fatalf("requests = %d", len(fake.requests))
	}
	if fake.paths[0] != "/v1/projects/test-project/messages:send" || fake.headers[0] != "Bearer test-access" {
		t.Errorf("path %q, authorization %q", fake.paths[0], fake.headers[0])
	}
	message := fake.requests[0].Message
	want := map[string]string{"update_id": "u1", "kind": "human_reply", "title": "Acme replied", "body": "They'd like a call.", "company_id": "c1"}
	if message.Token != "phone-token" || message.Android.Priority != "HIGH" || len(message.Data) != len(want) {
		t.Fatalf("message = %+v", message)
	}
	for key, value := range want {
		if message.Data[key] != value {
			t.Errorf("data[%s] = %q, want %q", key, message.Data[key], value)
		}
	}
}

func TestSendReportsAGoneToken(t *testing.T) {
	_, sender := startFakeFCM(t)
	if err := sender.Send(context.Background(), goneToken, Message{Title: "x"}); err != ErrTokenGone {
		t.Errorf("err = %v, want ErrTokenGone", err)
	}
}

func TestNewSenderReadsTheProjectFromTheServiceAccount(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	encodedKey, _ := x509.MarshalPKCS8PrivateKey(key)
	serviceAccount, _ := json.Marshal(map[string]string{
		"type": "service_account", "project_id": "test-project", "client_email": "hub@test-project.iam.gserviceaccount.com",
		"private_key": string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encodedKey})), "token_uri": "https://oauth2.googleapis.com/token",
	})
	sender, err := NewSender(context.Background(), serviceAccount)
	if err != nil {
		t.Fatal(err)
	}
	if sender.ProjectID() != "test-project" {
		t.Errorf("project = %q", sender.ProjectID())
	}
	if _, err := NewSender(context.Background(), []byte(`{"type":"authorized_user"}`)); err == nil {
		t.Error("a user credential was accepted")
	}
}

func TestPushSendsAnUpdateToEachPhoneAndForgetsGoneTokens(t *testing.T) {
	ctx := context.Background()
	hub := store.New(testdatabase.New(t))
	owner := store.Actor{Kind: store.ActorOwner}
	for name, pushToken := range map[string]string{"Sam's phone": "live-token", "Old phone": goneToken} {
		_, tokenHash := tokens.NewDeviceToken()
		device, err := hub.CreateDevice(ctx, owner, name, tokenHash)
		if err != nil {
			t.Fatal(err)
		}
		if err := hub.SetDevicePushToken(ctx, device.ID, pushToken); err != nil {
			t.Fatal(err)
		}
	}
	fake, sender := startFakeFCM(t)
	broadcaster := hubevents.NewBroadcaster()
	recorder := hubevents.NewRecorder(hub, broadcaster)
	notifier := NewNotifier(hub, sender, broadcaster)

	queued, err := recorder.Record(ctx, store.NewUpdate{Kind: "task_queued", Title: "From your phone: find jobs at Acme"})
	if err != nil {
		t.Fatal(err)
	}
	notifier.Push(ctx, queued)
	if len(fake.requests) != 0 {
		t.Fatalf("the phone's own request was pushed: %d sends", len(fake.requests))
	}

	update, err := recorder.Record(ctx, store.NewUpdate{Kind: "recruiter_outreach", Title: "Acme reached out", Body: "A recruiter wrote."})
	if err != nil {
		t.Fatal(err)
	}
	notifier.Push(ctx, update)
	if len(fake.requests) != 2 {
		t.Fatalf("sends = %d, want 2", len(fake.requests))
	}
	for _, request := range fake.requests {
		if request.Message.Data["update_id"] != update.ID.String() || request.Message.Data["kind"] != "recruiter_outreach" || request.Message.Data["title"] != "Acme reached out" {
			t.Errorf("data = %v", request.Message.Data)
		}
	}
	remaining, err := hub.ListDevicePushTokens(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 1 || remaining[0] != "live-token" {
		t.Errorf("tokens left = %q, want only the live one", remaining)
	}
}
