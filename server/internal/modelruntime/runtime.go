// Package modelruntime runs local models in the hub's own llama-server: one
// model in memory at a time, loaded when a request needs it and unloaded
// after it has been idle.
package modelruntime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// DefaultServerArgs are the flags the 2026-09-30 model benchmark measured
// with: every layer on the GPU, a 16k context, one request at a time, flash
// attention, and thinking off.
var DefaultServerArgs = []string{"-ngl", "99", "-c", "16384", "--parallel", "1", "--jinja", "--no-webui", "-fa", "on", "--reasoning", "off"}

// ErrModelNotFound means the requested model file isn't in the models folder.
var ErrModelNotFound = errors.New("no such model file in the models folder")

// StopTimeout is how long llama-server gets to stop after SIGTERM before
// it is killed.
const StopTimeout = 30 * time.Second

type Settings struct {
	// LlamaServer is the llama-server binary.
	LlamaServer string
	// ModelsDir holds the GGUF files a route can name.
	ModelsDir string
	Port      int
	// IdleTimeout unloads the model after it has served nothing that long.
	IdleTimeout time.Duration
	// LoadTimeout bounds how long a model may take to load.
	LoadTimeout time.Duration
	// LogPath receives llama-server's output.
	LogPath    string
	ServerArgs []string
}

// State is what the runtime is doing.
type State string

const (
	StateStopped State = "stopped"
	StateLoading State = "loading"
	StateReady   State = "ready"
)

// Status is the runtime's state for display: the model it holds, whether a
// request is using it, and when it was loaded and last used.
type Status struct {
	State      State      `json:"state"`
	Model      string     `json:"model,omitempty"`
	Busy       bool       `json:"busy"`
	LoadedAt   *time.Time `json:"loaded_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

type serverProcess struct {
	command *exec.Cmd
	model   string
	// exited closes when the process ends, whoever ended it.
	exited chan struct{}
}

// Runtime serves one request at a time on the model it names.
type Runtime struct {
	settings Settings
	// turn is held by the request using the server, so requests take turns.
	turn chan struct{}

	mutex   sync.Mutex
	process *serverProcess
	status  Status
}

func New(settings Settings) *Runtime {
	if len(settings.ServerArgs) == 0 {
		settings.ServerArgs = DefaultServerArgs
	}
	if settings.LoadTimeout == 0 {
		settings.LoadTimeout = 5 * time.Minute
	}
	return &Runtime{settings: settings, turn: make(chan struct{}, 1), status: Status{State: StateStopped}}
}

// Status returns what the runtime is doing now.
func (runtime *Runtime) Status() Status {
	runtime.mutex.Lock()
	defer runtime.mutex.Unlock()
	return runtime.status
}

// Acquire waits for its turn, makes sure modelFile is the loaded model, and
// returns the chat-completions base URL. release must be called when the
// request ends.
func (runtime *Runtime) Acquire(ctx context.Context, modelFile string) (baseURL string, release func(), err error) {
	if modelFile == "" || filepath.Base(modelFile) != modelFile {
		return "", nil, fmt.Errorf("%w: %q", ErrModelNotFound, modelFile)
	}
	modelPath := filepath.Join(runtime.settings.ModelsDir, modelFile)
	if _, err := os.Stat(modelPath); err != nil {
		return "", nil, fmt.Errorf("%w: %s", ErrModelNotFound, modelPath)
	}
	select {
	case runtime.turn <- struct{}{}:
	case <-ctx.Done():
		return "", nil, ctx.Err()
	}
	if err := runtime.ensureLoaded(ctx, modelFile, modelPath); err != nil {
		<-runtime.turn
		return "", nil, err
	}
	runtime.mutex.Lock()
	runtime.status.Busy = true
	runtime.mutex.Unlock()
	release = func() {
		runtime.mutex.Lock()
		now := time.Now()
		runtime.status.Busy, runtime.status.LastUsedAt = false, &now
		runtime.mutex.Unlock()
		<-runtime.turn
	}
	return fmt.Sprintf("http://127.0.0.1:%d/v1", runtime.settings.Port), release, nil
}

// Unload stops the server and frees its memory, once no request is using it.
func (runtime *Runtime) Unload() {
	runtime.turn <- struct{}{}
	defer func() { <-runtime.turn }()
	runtime.stopServer()
}

// Run unloads the model once it has been idle for the idle timeout, and
// stops the server when ctx ends.
func (runtime *Runtime) Run(ctx context.Context) {
	ticker := time.NewTicker(getIdleCheckInterval(runtime.settings.IdleTimeout))
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			runtime.stopServer()
			return
		case <-ticker.C:
			runtime.unloadIfIdle()
		}
	}
}

func getIdleCheckInterval(idleTimeout time.Duration) time.Duration {
	return min(max(idleTimeout/4, 50*time.Millisecond), 15*time.Second)
}

func (runtime *Runtime) unloadIfIdle() {
	select {
	case runtime.turn <- struct{}{}:
	default:
		return // a request is using the server
	}
	defer func() { <-runtime.turn }()
	runtime.mutex.Lock()
	idle := runtime.process != nil && runtime.status.LastUsedAt != nil && time.Since(*runtime.status.LastUsedAt) >= runtime.settings.IdleTimeout
	runtime.mutex.Unlock()
	if idle {
		slog.Info("model runtime idle; unloading", "model", runtime.Status().Model)
		runtime.stopServer()
	}
}

// ensureLoaded runs while holding the turn.
func (runtime *Runtime) ensureLoaded(ctx context.Context, modelFile, modelPath string) error {
	runtime.mutex.Lock()
	process := runtime.process
	runtime.mutex.Unlock()
	if process != nil && process.model == modelFile {
		return nil
	}
	if process != nil {
		slog.Info("model runtime swapping models", "from", process.model, "to", modelFile)
		runtime.stopServer()
	}
	return runtime.startServer(ctx, modelFile, modelPath)
}

func (runtime *Runtime) startServer(ctx context.Context, modelFile, modelPath string) error {
	arguments := append([]string{"-m", modelPath, "--host", "127.0.0.1", "--port", strconv.Itoa(runtime.settings.Port)}, runtime.settings.ServerArgs...)
	command := exec.Command(runtime.settings.LlamaServer, arguments...)
	if runtime.settings.LogPath != "" {
		if log, err := os.OpenFile(runtime.settings.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
			command.Stdout, command.Stderr = log, log
			defer log.Close()
		}
	}
	if err := command.Start(); err != nil {
		return fmt.Errorf("start llama-server: %w", err)
	}
	process := &serverProcess{command: command, model: modelFile, exited: make(chan struct{})}
	go func() {
		_ = command.Wait()
		close(process.exited)
		runtime.mutex.Lock()
		defer runtime.mutex.Unlock()
		if runtime.process == process { // it died; stopServer clears process first
			runtime.process = nil
			runtime.status = Status{State: StateStopped}
			slog.Warn("llama-server stopped by itself", "model", process.model)
		}
	}()
	runtime.mutex.Lock()
	runtime.process = process
	runtime.status = Status{State: StateLoading, Model: modelFile}
	runtime.mutex.Unlock()
	slog.Info("model runtime loading", "model", modelFile)

	if err := runtime.waitUntilHealthy(ctx, process); err != nil {
		runtime.stopServer()
		return err
	}
	now := time.Now()
	runtime.mutex.Lock()
	runtime.status = Status{State: StateReady, Model: modelFile, LoadedAt: &now, LastUsedAt: &now}
	runtime.mutex.Unlock()
	slog.Info("model runtime ready", "model", modelFile)
	return nil
}

func (runtime *Runtime) waitUntilHealthy(ctx context.Context, process *serverProcess) error {
	deadline := time.After(runtime.settings.LoadTimeout)
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", runtime.settings.Port)
	client := &http.Client{Timeout: 2 * time.Second}
	for {
		if response, err := client.Get(healthURL); err == nil {
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return nil
			}
		}
		select {
		case <-process.exited:
			return fmt.Errorf("llama-server exited while loading %s; see %s", process.model, runtime.settings.LogPath)
		case <-deadline:
			return fmt.Errorf("llama-server didn't load %s within %s", process.model, runtime.settings.LoadTimeout)
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}

// stopServer ends the running server, gently first.
func (runtime *Runtime) stopServer() {
	runtime.mutex.Lock()
	process := runtime.process
	runtime.process = nil
	runtime.status = Status{State: StateStopped}
	runtime.mutex.Unlock()
	if process == nil {
		return
	}
	_ = process.command.Process.Signal(syscall.SIGTERM)
	select {
	case <-process.exited:
	case <-time.After(StopTimeout):
		_ = process.command.Process.Kill()
		<-process.exited
	}
	slog.Info("model runtime stopped", "model", process.model)
}
