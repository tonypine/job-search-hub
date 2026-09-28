package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestARecruiterReplyIsDraftedWithNoToolsAndPrinted(t *testing.T) {
	routes := http.NewServeMux()
	routes.HandleFunc("GET /v1/recruiters/c1/reply-prompt", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+testOwnerToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		fmt.Fprint(w, `{"prompt":"Draft a reply to Rita","version":1}`)
	})
	server := httptest.NewServer(routes)
	t.Cleanup(server.Close)
	argumentsPath := installFakeClaude(t, `{"type":"result","subtype":"success","is_error":false,"result":"  Hi Rita, you wrote to me in 2025 about Globex.  "}`)

	var out bytes.Buffer
	config := cliConfig{HubURL: server.URL, OwnerToken: testOwnerToken}
	if err := draftRecruiterReply(context.Background(), config, "c1", "", &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != "Hi Rita, you wrote to me in 2025 about Globex.\n" {
		t.Fatalf("printed %q", out.String())
	}
	arguments, _ := os.ReadFile(argumentsPath)
	lines := strings.Split(strings.TrimSpace(string(arguments)), "\n")
	if lines[1] != "Draft a reply to Rita" || !strings.Contains(string(arguments), "--strict-mcp-config") || strings.Contains(string(arguments), "--mcp-config") {
		t.Fatalf("arguments = %q", lines)
	}
	for index, line := range lines {
		if line == "--tools" && (index+1 >= len(lines) || lines[index+1] != "") {
			t.Fatalf("--tools should be empty: %q", lines)
		}
	}

	if err := draftRecruiterReply(context.Background(), config, "unknown", "", &out); err == nil {
		t.Fatal("an unknown conversation should fail")
	}
}
