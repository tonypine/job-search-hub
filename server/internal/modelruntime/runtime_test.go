package modelruntime_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/modelruntime"
)

// When FAKE_LLAMA_SERVER is set, this test binary acts as llama-server: it
// answers /health and says which model it loaded, so the runtime can be
// tested without a GPU. It exits once the test binary that started it is
// gone, so a test killed or timed out mid-run leaves no server behind.
func TestMain(m *testing.M) {
	if os.Getenv("FAKE_LLAMA_SERVER") == "1" {
		serveLikeLlamaServer()
		return
	}
	os.Exit(m.Run())
}

func serveLikeLlamaServer() {
	var model, port string
	for index, argument := range os.Args {
		switch argument {
		case "-m":
			model = filepath.Base(os.Args[index+1])
		case "--port":
			port = os.Args[index+1]
		}
	}
	parent := os.Getppid()
	go func() {
		for range time.Tick(100 * time.Millisecond) {
			if os.Getppid() != parent {
				os.Exit(0)
			}
		}
	}()
	routes := http.NewServeMux()
	routes.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte(`{"status":"ok"}`)) })
	routes.HandleFunc("/v1/model", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"model": model})
	})
	http.ListenAndServe("127.0.0.1:"+port, routes)
}

func startRuntime(t *testing.T, idle time.Duration) (*modelruntime.Runtime, context.CancelFunc) {
	t.Helper()
	t.Setenv("FAKE_LLAMA_SERVER", "1")
	models := t.TempDir()
	for _, name := range []string{"big.gguf", "small.gguf"} {
		os.WriteFile(filepath.Join(models, name), []byte("weights"), 0o644)
	}
	runtime := modelruntime.New(modelruntime.Settings{
		LlamaServer: os.Args[0], ModelsDir: models, Port: getFreePort(t), IdleTimeout: idle, LoadTimeout: 10 * time.Second,
		LogPath: filepath.Join(t.TempDir(), "llama-server.log"),
	})
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		runtime.Run(ctx)
		close(stopped)
	}()
	// Run stops the server once ctx ends; wait for it, so the server is gone
	// before the test binary exits.
	t.Cleanup(func() {
		cancel()
		<-stopped
	})
	return runtime, cancel
}

func getFreePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func loadedModel(t *testing.T, baseURL string) string {
	t.Helper()
	response, err := http.Get(baseURL + "/model")
	if err != nil {
		t.Fatalf("the server isn't answering: %v", err)
	}
	defer response.Body.Close()
	var body map[string]string
	json.NewDecoder(response.Body).Decode(&body)
	return body["model"]
}

func TestTheRuntimeLoadsTheModelARequestNeedsAndSwapsForAnother(t *testing.T) {
	runtime, _ := startRuntime(t, time.Hour)
	if runtime.Status().State != modelruntime.StateStopped {
		t.Fatalf("a new runtime is %s", runtime.Status().State)
	}

	baseURL, release, err := runtime.Acquire(context.Background(), "big.gguf")
	if err != nil {
		t.Fatal(err)
	}
	if status := runtime.Status(); status.State != modelruntime.StateReady || status.Model != "big.gguf" || !status.Busy {
		t.Fatalf("status = %+v", status)
	}
	if model := loadedModel(t, baseURL); model != "big.gguf" {
		t.Fatalf("serving %q", model)
	}
	release()

	baseURL, release, err = runtime.Acquire(context.Background(), "small.gguf")
	if err != nil {
		t.Fatal(err)
	}
	if model := loadedModel(t, baseURL); model != "small.gguf" || runtime.Status().Model != "small.gguf" {
		t.Fatalf("after a swap: serving %q, status %+v", model, runtime.Status())
	}
	release()

	for _, name := range []string{"missing.gguf", "../big.gguf", ""} {
		if _, _, err := runtime.Acquire(context.Background(), name); !errors.Is(err, modelruntime.ErrModelNotFound) {
			t.Errorf("%q: err = %v", name, err)
		}
	}
}

func TestRequestsTakeTurns(t *testing.T) {
	runtime, _ := startRuntime(t, time.Hour)
	_, release, err := runtime.Acquire(context.Background(), "big.gguf")
	if err != nil {
		t.Fatal(err)
	}
	waiting, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if _, _, err := runtime.Acquire(waiting, "big.gguf"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("a second request got in while the first held its turn: %v", err)
	}
	release()
	if _, release, err := runtime.Acquire(context.Background(), "big.gguf"); err != nil {
		t.Fatal(err)
	} else {
		release()
	}
}

func TestAnIdleModelIsUnloadedAndACrashedServerRestarts(t *testing.T) {
	runtime, _ := startRuntime(t, 300*time.Millisecond)
	baseURL, release, err := runtime.Acquire(context.Background(), "big.gguf")
	if err != nil {
		t.Fatal(err)
	}
	release()
	waitFor(t, func() bool { return runtime.Status().State == modelruntime.StateStopped })
	if _, err := http.Get(baseURL + "/model"); err == nil {
		t.Fatal("the server still answers after the idle unload")
	}

	baseURL, release, err = runtime.Acquire(context.Background(), "big.gguf")
	if err != nil {
		t.Fatal(err)
	}
	release()
	killServerOn(t, baseURL)
	waitFor(t, func() bool { return runtime.Status().State == modelruntime.StateStopped })
	baseURL, release, err = runtime.Acquire(context.Background(), "big.gguf")
	if err != nil || loadedModel(t, baseURL) != "big.gguf" {
		t.Fatalf("after a crash: %v", err)
	}
	release()
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	for range 100 {
		if condition() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("timed out waiting")
}

// killServerOn kills the process listening on the base URL's port, as a
// crash would.
func killServerOn(t *testing.T, baseURL string) {
	t.Helper()
	var port int
	fmt.Sscanf(baseURL, "http://127.0.0.1:%d/v1", &port)
	output, err := exec.Command("lsof", "-ti", "tcp:"+strconv.Itoa(port), "-sTCP:LISTEN").Output()
	if err != nil {
		t.Fatalf("find the server: %v", err)
	}
	var pid int
	fmt.Sscanf(string(output), "%d", &pid)
	if process, err := os.FindProcess(pid); err == nil {
		process.Kill()
	}
}
