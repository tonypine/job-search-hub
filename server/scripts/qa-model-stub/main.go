// Command qa-model-stub is a stand-in model for Symphony's QA pass: an
// OpenAI-compatible server that dev-server.sh starts beside the hub and
// routes every model kind to, so a run started from the app runs for a few
// seconds and lands. It is never used outside the QA pass.
//
// It answers each chat completion, after -delay, with a JSON object built
// from the schema the request's response_format names, or else the one the
// hub puts at the end of its system prompt for a provider that doesn't
// enforce schemas: the first value of an enum, a short stand-in text for a
// string, one item for an array, 0 or the minimum for a number. The answer is
// the same for every request of a schema, and fits it. A request with no
// schema gets an empty object.
//
//	qa-model-stub -listen 127.0.0.1:0 -url-file <path> -delay 3s
//
// It writes its base URL, such as http://127.0.0.1:52817/v1, to -url-file
// once it listens, and runs until TERM or Ctrl-C.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// Model is the model name the stub answers as, and the one dev-server-seed
// routes each kind to.
const Model = "qa-stand-in"

// standInText is every string the stub answers, cut to its schema's
// maxLength.
const standInText = "QA stand-in answer"

func main() {
	listen := flag.String("listen", "127.0.0.1:0", "the address to listen on")
	urlFile := flag.String("url-file", "", "a file to write the base URL to once listening")
	delay := flag.Duration("delay", 3*time.Second, "how long each answer takes")
	flag.Parse()

	listener, err := net.Listen("tcp", *listen)
	if err != nil {
		log.Fatalf("qa-model-stub: %v", err)
	}
	baseURL := "http://" + listener.Addr().String() + "/v1"
	if *urlFile != "" {
		// Renamed into place, so a reader never sees half of it.
		if err := os.WriteFile(*urlFile+".partial", []byte(baseURL+"\n"), 0o600); err != nil {
			log.Fatalf("qa-model-stub: %v", err)
		}
		if err := os.Rename(*urlFile+".partial", *urlFile); err != nil {
			log.Fatalf("qa-model-stub: %v", err)
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	server := &http.Server{Handler: newHandler(*delay), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	log.Printf("qa-model-stub: %s answers as %s after %s", baseURL, Model, *delay)
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("qa-model-stub: %v", err)
	}
}

// newHandler serves the chat-completions and models endpoints, answering
// each completion after delay.
func newHandler(delay time.Duration) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"object": "list",
			"data":   []map[string]any{{"id": Model, "object": "model", "owned_by": "qa"}},
		})
	})
	mux.HandleFunc("POST /v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
			ResponseFormat struct {
				JSONSchema struct {
					Schema json.RawMessage `json:"schema"`
				} `json:"json_schema"`
			} `json:"response_format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"message": "read the request: " + err.Error()}})
			return
		}
		schema := request.ResponseFormat.JSONSchema.Schema
		if len(schema) == 0 && len(request.Messages) > 0 && request.Messages[0].Role == "system" {
			schema = findPromptSchema(request.Messages[0].Content)
		}
		answer, err := buildAnswer(schema)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"message": err.Error()}})
			return
		}
		select {
		case <-time.After(delay):
		case <-r.Context().Done():
			return
		}
		model := request.Model
		if model == "" {
			model = Model
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id":      "qa-stand-in",
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"model":   model,
			"choices": []map[string]any{{
				"index":         0,
				"finish_reason": "stop",
				"message":       map[string]string{"role": "assistant", "content": string(answer)},
			}},
			"usage": map[string]int{"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0},
		})
	})
	return mux
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// promptSchemaIntro is what precedes the schema in the hub's system prompt,
// as chatcompletions writes it.
const promptSchemaIntro = "Answer with only a JSON object that matches this JSON Schema:\n"

// findPromptSchema is the schema at the end of a system prompt, nil when it
// has none.
func findPromptSchema(system string) json.RawMessage {
	index := strings.LastIndex(system, promptSchemaIntro)
	if index < 0 {
		return nil
	}
	var schema json.RawMessage
	if err := json.NewDecoder(strings.NewReader(system[index+len(promptSchemaIntro):])).Decode(&schema); err != nil {
		return nil
	}
	return schema
}

// buildAnswer is the JSON object the stub answers for schema, an empty
// object when there is none.
func buildAnswer(schema json.RawMessage) (json.RawMessage, error) {
	if len(schema) == 0 || string(schema) == "null" {
		return json.RawMessage(`{}`), nil
	}
	var root map[string]any
	if err := json.Unmarshal(schema, &root); err != nil {
		return nil, fmt.Errorf("read the schema: %w", err)
	}
	value := buildValue(root, root, 0)
	if _, isObject := value.(map[string]any); !isObject {
		value = map[string]any{}
	}
	return json.Marshal(value)
}

// maximumDepth stops a schema that refers to itself.
const maximumDepth = 16

// buildValue is a value that fits schema; root resolves its $refs.
func buildValue(schema, root map[string]any, depth int) any {
	if depth > maximumDepth {
		return nil
	}
	if ref, found := schema["$ref"].(string); found {
		if resolved := resolveRef(ref, root); resolved != nil {
			return buildValue(resolved, root, depth+1)
		}
		return nil
	}
	if value, found := schema["const"]; found {
		return value
	}
	if values, found := schema["enum"].([]any); found && len(values) > 0 {
		return values[0]
	}
	for _, keyword := range []string{"anyOf", "oneOf", "allOf"} {
		if branches, found := schema[keyword].([]any); found {
			if branch := pickBranch(branches); branch != nil {
				return buildValue(branch, root, depth+1)
			}
		}
	}
	switch getType(schema) {
	case "object":
		object := map[string]any{}
		properties, _ := schema["properties"].(map[string]any)
		for name, property := range properties {
			if propertySchema, isSchema := property.(map[string]any); isSchema {
				object[name] = buildValue(propertySchema, root, depth+1)
			}
		}
		return object
	case "array":
		count := 1
		if minimum, found := getNumber(schema, "minItems"); found {
			count = max(count, int(minimum))
		}
		if maximum, found := getNumber(schema, "maxItems"); found {
			count = min(count, int(maximum))
		}
		items, _ := schema["items"].(map[string]any)
		array := make([]any, 0, count)
		for range count {
			array = append(array, buildValue(items, root, depth+1))
		}
		return array
	case "string":
		return buildString(schema)
	case "integer":
		return math.Ceil(buildNumber(schema))
	case "number":
		return buildNumber(schema)
	case "boolean":
		return false
	case "null":
		return nil
	}
	return standInText
}

// pickBranch is the first branch that isn't null, or the first one.
func pickBranch(branches []any) map[string]any {
	var first map[string]any
	for _, branch := range branches {
		schema, isSchema := branch.(map[string]any)
		if !isSchema {
			continue
		}
		if first == nil {
			first = schema
		}
		if getType(schema) != "null" {
			return schema
		}
	}
	return first
}

// getType is the schema's type: its first that isn't null when it names
// several, or what its keywords imply when it names none.
func getType(schema map[string]any) string {
	switch typed := schema["type"].(type) {
	case string:
		return typed
	case []any:
		for _, name := range typed {
			if name, isString := name.(string); isString && name != "null" {
				return name
			}
		}
		return "null"
	}
	if _, found := schema["properties"]; found {
		return "object"
	}
	if _, found := schema["items"]; found {
		return "array"
	}
	return ""
}

func buildString(schema map[string]any) string {
	text := standInText
	switch schema["format"] {
	case "date-time":
		text = "2026-01-01T00:00:00Z"
	case "date":
		text = "2026-01-01"
	case "uri", "url":
		text = "https://qa.example/stand-in"
	case "email":
		text = "qa@stand-in.example"
	case "uuid":
		text = "00000000-0000-0000-0000-000000000000"
	}
	if minimum, found := getNumber(schema, "minLength"); found && len(text) < int(minimum) {
		text += strings.Repeat(".", int(minimum)-len(text))
	}
	if maximum, found := getNumber(schema, "maxLength"); found && len(text) > int(maximum) {
		text = text[:int(maximum)]
	}
	return text
}

func buildNumber(schema map[string]any) float64 {
	if minimum, found := getNumber(schema, "minimum"); found {
		return minimum
	}
	if minimum, found := getNumber(schema, "exclusiveMinimum"); found {
		return minimum + 1
	}
	if maximum, found := getNumber(schema, "maximum"); found && maximum < 0 {
		return maximum
	}
	return 0
}

func getNumber(schema map[string]any, keyword string) (float64, bool) {
	number, found := schema[keyword].(float64)
	return number, found
}

// resolveRef finds a local $ref, such as #/$defs/point, in root.
func resolveRef(ref string, root map[string]any) map[string]any {
	if ref == "#" {
		return root
	}
	path, isLocal := strings.CutPrefix(ref, "#/")
	if !isLocal {
		return nil
	}
	var node any = root
	for _, part := range strings.Split(path, "/") {
		object, isObject := node.(map[string]any)
		if !isObject {
			return nil
		}
		node = object[strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")]
	}
	resolved, _ := node.(map[string]any)
	return resolved
}
