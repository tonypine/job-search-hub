package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"testing"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

func uploadArtifact(t *testing.T, url, token string, fields map[string]string, name string, content []byte) (int, []byte) {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	for key, value := range fields {
		form.WriteField(key, value)
	}
	part, _ := form.CreateFormFile("file", name)
	part.Write(content)
	form.Close()
	request, _ := http.NewRequest(http.MethodPost, url+"/v1/artifacts", &body)
	request.Header.Set("Content-Type", form.FormDataContentType())
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	defer response.Body.Close()
	payload, _ := io.ReadAll(response.Body)
	return response.StatusCode, payload
}

func TestTheOwnerUploadsListsAndDeletesFilesOnce(t *testing.T) {
	service := startAPI(t)
	ctx := context.Background()
	company, _, _ := service.hub.CreateCompany(ctx, store.Actor{Kind: store.ActorOwner}, store.NewCompany{Name: "Acme", Domain: "acme.com"})
	resume := []byte("Sam Example\nSenior engineer.\n")

	status, body := uploadArtifact(t, service.url, ownerToken, map[string]string{"kind": "resume"}, "resume.txt", resume)
	var uploaded struct {
		Artifact store.Artifact `json:"artifact"`
		Created  bool           `json:"created"`
	}
	if err := json.Unmarshal(body, &uploaded); status != http.StatusCreated || err != nil || !uploaded.Created ||
		uploaded.Artifact.Name != "resume.txt" || uploaded.Artifact.Size != len(resume) || uploaded.Artifact.CompanyID != nil ||
		uploaded.Artifact.TextLength != len("Sam Example\nSenior engineer.") {
		t.Fatalf("upload: %d %s", status, body)
	}
	if status, body := uploadArtifact(t, service.url, ownerToken, map[string]string{"kind": "other"}, "copy.txt", resume); status != http.StatusOK ||
		!bytes.Contains(body, []byte(`"created":false`)) || !bytes.Contains(body, []byte(uploaded.Artifact.ID.String())) {
		t.Fatalf("the same file again: %d %s", status, body)
	}
	if status, body := uploadArtifact(t, service.url, ownerToken, map[string]string{"kind": "saved_page", "company_domain": "acme.com"}, "team.html", []byte("<p>Team</p>")); status != http.StatusCreated ||
		!bytes.Contains(body, []byte(company.ID.String())) {
		t.Fatalf("a company's file: %d %s", status, body)
	}

	status, body = send(t, http.MethodGet, service.url+"/v1/artifacts", ownerToken, "")
	var owners struct {
		Artifacts []store.Artifact `json:"artifacts"`
	}
	if err := json.Unmarshal(body, &owners); status != http.StatusOK || err != nil || len(owners.Artifacts) != 1 || owners.Artifacts[0].ID != uploaded.Artifact.ID {
		t.Fatalf("the owner's files: %d %s", status, body)
	}
	if status, body := send(t, http.MethodGet, service.url+"/v1/artifacts?company_id="+company.ID.String(), ownerToken, ""); status != http.StatusOK || !bytes.Contains(body, []byte("team.html")) {
		t.Fatalf("the company's files: %d %s", status, body)
	}

	if status, _ := send(t, http.MethodDelete, service.url+"/v1/artifacts/"+uploaded.Artifact.ID.String(), ownerToken, ""); status != http.StatusNoContent {
		t.Fatalf("delete: %d", status)
	}
	if status, _ := send(t, http.MethodDelete, service.url+"/v1/artifacts/"+uploaded.Artifact.ID.String(), ownerToken, ""); status != http.StatusNotFound {
		t.Errorf("delete again: %d, want 404", status)
	}
}

func TestFilesAreRefusedToAgentsAndOverTheSizeLimit(t *testing.T) {
	service := startAPI(t)
	agentToken := startTriage(t, service).Token

	if status, _ := uploadArtifact(t, service.url, agentToken, map[string]string{"kind": "resume"}, "resume.txt", []byte("text")); status != http.StatusForbidden {
		t.Errorf("an agent's upload: %d, want 403", status)
	}
	if status, _ := send(t, http.MethodDelete, service.url+"/v1/artifacts/00000000-0000-0000-0000-000000000000", agentToken, ""); status != http.StatusForbidden {
		t.Errorf("an agent's delete: %d, want 403", status)
	}
	if status, body := uploadArtifact(t, service.url, ownerToken, map[string]string{"kind": "other"}, "big.bin", make([]byte, store.MaximumArtifactSize+1)); status != http.StatusRequestEntityTooLarge ||
		!bytes.Contains(body, []byte("10 MB")) {
		t.Errorf("an oversized file: %d %s", status, body)
	}
	if status, body := uploadArtifact(t, service.url, ownerToken, map[string]string{"kind": "other"}, "photo.png", []byte("\x89PNG\r\n\x1a\n")); status != http.StatusCreated ||
		!bytes.Contains(body, []byte(`"text_error":"can't read text from a image/png file"`)) {
		t.Errorf("a file without text: %d %s", status, body)
	}
	if status, _ := uploadArtifact(t, service.url, ownerToken, map[string]string{"kind": "poster"}, "a.txt", []byte("text")); status != http.StatusBadRequest {
		t.Errorf("an unknown kind: %d, want 400", status)
	}
}
