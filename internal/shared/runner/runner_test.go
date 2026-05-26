package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aaudin90/opencode-reviewer/internal/shared/config"
	"github.com/aaudin90/opencode-reviewer/internal/shared/workspace"
)

// newTestRunner creates a Runner with nil workspace for tests using external endpoint.
func newTestRunner(cfg config.OpenCodeConfig) *Runner {
	return New(cfg, "/tmp", nil, "test")
}

// collectRunResult drains a RunEvent channel and returns the final RunResult or error.
func collectRunResult(t *testing.T, ch <-chan RunEvent) (*RunResult, error) {
	t.Helper()
	var result *RunResult
	for event := range ch {
		switch {
		case event.Err != nil:
			return nil, event.Err
		case event.Final != nil:
			result = event.Final
		}
	}
	return result, nil
}

func TestExtractText(t *testing.T) {
	r := newTestRunner(config.OpenCodeConfig{})

	parts := []messagePart{
		{Type: "text", Text: "Hello "},
		{Type: "tool_use", Text: "ignored"},
		{Type: "text", Text: "World"},
	}

	got := r.extractText(parts)
	if got != "Hello World" {
		t.Errorf("extractText = %q, want %q", got, "Hello World")
	}
}

func TestExtractText_Empty(t *testing.T) {
	r := newTestRunner(config.OpenCodeConfig{})
	got := r.extractText(nil)
	if got != "" {
		t.Errorf("extractText(nil) = %q, want empty", got)
	}
}

func TestWaitHealthy(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/global/health" {
			http.NotFound(w, req)
			return
		}
		calls++
		if calls < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := newTestRunner(config.OpenCodeConfig{Endpoint: srv.URL})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := r.waitHealthy(ctx, nil); err != nil {
		t.Fatalf("waitHealthy: %v", err)
	}

	if calls < 3 {
		t.Errorf("expected at least 3 health calls, got %d", calls)
	}
}

func TestWaitHealthy_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("warming up"))
	}))
	defer srv.Close()

	r := newTestRunner(config.OpenCodeConfig{Endpoint: srv.URL})

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := r.waitHealthy(ctx, nil)
	if err == nil {
		t.Fatal("waitHealthy should return error on timeout")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("error = %v, want timed out", err)
	}
	if !strings.Contains(err.Error(), "503 Service Unavailable: warming up") {
		t.Errorf("error = %v, want last health status and body", err)
	}
}

func TestWaitHealthy_TimeoutIncludesRequestError(t *testing.T) {
	r := newTestRunner(config.OpenCodeConfig{Endpoint: "http://127.0.0.1:1"})

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := r.waitHealthy(ctx, nil)
	if err == nil {
		t.Fatal("waitHealthy should return error on timeout")
	}
	if !strings.Contains(err.Error(), "health request failed") {
		t.Errorf("error = %v, want request failure", err)
	}
}

func TestServeArgs_WithOpenCodeLogs(t *testing.T) {
	r := newTestRunner(config.OpenCodeConfig{
		PrintLogs: true,
		LogLevel:  "DEBUG",
	})

	got := strings.Join(r.serveArgs(4097), " ")
	want := "serve --port 4097 --print-logs --log-level DEBUG"
	if got != want {
		t.Fatalf("serveArgs = %q, want %q", got, want)
	}
}

func TestStartServeWritesProcessOutputToLogFile(t *testing.T) {
	if os.Getenv("OR_TEST_HELPER_OPENCODE_SERVE") == "1" {
		helperOpenCodeServe()
		return
	}

	dir := t.TempDir()
	script := filepath.Join(dir, "fake-opencode")
	body := fmt.Sprintf("#!/bin/sh\nexec %q -test.run=TestStartServeWritesProcessOutputToLogFile -- \"$@\"\n", os.Args[0])
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OR_TEST_HELPER_OPENCODE_SERVE", "1")

	r := New(config.OpenCodeConfig{
		Binary:    script,
		PrintLogs: true,
		LogLevel:  "INFO",
		LogDir:    "logs",
	}, dir, nil, "reviewer")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := r.StartServe(ctx); err != nil {
		t.Fatalf("StartServe: %v", err)
	}
	r.StopServe()

	logPath := r.LogPath()
	if logPath == "" {
		t.Fatal("LogPath is empty")
	}
	if !strings.Contains(filepath.Base(logPath), "reviewer-") {
		t.Fatalf("log path = %q, want reviewer prefix", logPath)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, "fake stdout") || !strings.Contains(got, "fake stderr") {
		t.Fatalf("log file = %q, want stdout and stderr", got)
	}
}

func TestStartServeSetsXDGDirs(t *testing.T) {
	if os.Getenv("OR_TEST_HELPER_OPENCODE_SERVE") == "1" {
		helperOpenCodeServe()
		return
	}

	dir := t.TempDir()
	script := filepath.Join(dir, "fake-opencode")
	body := fmt.Sprintf("#!/bin/sh\nexec %q -test.run=TestStartServeSetsXDGDirs -- \"$@\"\n", os.Args[0])
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OR_TEST_HELPER_OPENCODE_SERVE", "1")
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(dir, ".cache"))
	t.Setenv("BUN_INSTALL_CACHE_DIR", filepath.Join(dir, ".bun", "install", "cache"))

	ws, err := workspace.NewAgent(workspace.Config{Model: "test/model"}, workspace.AgentSpec{})
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}
	defer func() { _ = ws.Cleanup() }()

	r := New(config.OpenCodeConfig{
		Binary:    script,
		PrintLogs: true,
		LogLevel:  "INFO",
		LogDir:    "logs",
	}, dir, ws, "reviewer")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := r.StartServe(ctx); err != nil {
		t.Fatalf("StartServe: %v", err)
	}
	r.StopServe()

	data, err := os.ReadFile(r.LogPath())
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	got := string(data)
	for name, want := range map[string]string{
		"XDG_CONFIG_HOME":       ws.Dir(),
		"XDG_CACHE_HOME":        ws.CacheDir(),
		"XDG_DATA_HOME":         ws.DataDir(),
		"XDG_STATE_HOME":        ws.StateDir(),
		"BUN_INSTALL_CACHE_DIR": filepath.Join(ws.CacheDir(), "bun", "install", "cache"),
	} {
		if !strings.Contains(got, name+"="+want) {
			t.Fatalf("log file missing %s=%s:\n%s", name, want, got)
		}
	}
}

func TestStartServeIgnoresStaleProcDone(t *testing.T) {
	if os.Getenv("OR_TEST_HELPER_OPENCODE_STALE_PROCDONE") == "1" {
		helperOpenCodeServe()
		return
	}

	dir := t.TempDir()
	script := filepath.Join(dir, "fake-opencode")
	body := fmt.Sprintf("#!/bin/sh\nOR_TEST_HELPER_OPENCODE_STALE_PROCDONE=1 exec %q -test.run=TestStartServeIgnoresStaleProcDone -- \"$@\"\n", os.Args[0])
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}

	ws, err := workspace.NewAgent(workspace.Config{}, workspace.AgentSpec{})
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}
	defer func() { _ = ws.Cleanup() }()

	r := New(config.OpenCodeConfig{Binary: script}, dir, ws, "reviewer")
	r.procDone <- fmt.Errorf("stale process error")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := r.StartServe(ctx); err != nil {
		t.Fatalf("StartServe: %v", err)
	}
	r.StopServe()
}

func helperOpenCodeServe() {
	args := os.Args
	port := ""
	for i, arg := range args {
		if arg == "--port" && i+1 < len(args) {
			port = args[i+1]
			break
		}
	}
	if port == "" {
		os.Exit(2)
	}
	fmt.Fprintln(os.Stdout, "fake stdout")
	fmt.Fprintln(os.Stderr, "fake stderr")
	for _, name := range []string{"XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "BUN_INSTALL_CACHE_DIR"} {
		fmt.Fprintf(os.Stdout, "%s=%s\n", name, os.Getenv(name))
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/global/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	if err := http.ListenAndServe("127.0.0.1:"+port, mux); err != nil {
		os.Exit(1)
	}
}

func testModelString(model *messageModel) string {
	if model == nil {
		return ""
	}
	return model.ProviderID + "/" + model.ModelID
}

func assertStrings(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d; got %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("[%d] = %q, want %q; got %v", i, got[i], want[i], got)
		}
	}
}

func assertModelCostStrings(t *testing.T, got, want []ModelCost) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d; got %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].Model != want[i].Model || got[i].Cost != want[i].Cost || got[i].Tokens.Input != want[i].Tokens.Input || got[i].Tokens.Output != want[i].Tokens.Output {
			t.Fatalf("[%d] = %#v, want %#v; got %#v", i, got[i], want[i], got)
		}
	}
}

func TestPrecheckSendsDeterministicPrompt(t *testing.T) {
	var gotPrompt string
	var gotAgent string
	var gotModel *messageModel

	mux := http.NewServeMux()
	mux.HandleFunc("POST /session", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sessionResponse{ID: "sess-precheck"})
	})
	mux.HandleFunc("POST /session/{id}/message", func(w http.ResponseWriter, req *http.Request) {
		var mr messageRequest
		if err := json.NewDecoder(req.Body).Decode(&mr); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if len(mr.Parts) > 0 {
			gotPrompt = mr.Parts[0].Text
		}
		gotAgent = mr.Agent
		gotModel = mr.Model

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(messageResponse{
			Info: messageInfo{ID: "msg-precheck"},
			Parts: []messagePart{
				{Type: "text", Text: "OK"},
			},
		})
	})
	mux.HandleFunc("DELETE /session/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	r := newTestRunner(config.OpenCodeConfig{Endpoint: srv.URL, Model: "test/model"})
	if err := r.Precheck(context.Background(), "reviewer", "override/model"); err != nil {
		t.Fatalf("Precheck: %v", err)
	}
	if gotPrompt != precheckPrompt {
		t.Errorf("prompt = %q, want %q", gotPrompt, precheckPrompt)
	}
	if gotAgent != "reviewer" {
		t.Errorf("agent = %q, want reviewer", gotAgent)
	}
	if gotModel == nil {
		t.Fatal("model is nil")
	}
	if gotModel.ProviderID != "override" || gotModel.ModelID != "model" {
		t.Errorf("model = %+v, want override/model", gotModel)
	}
}

// makeToolCallSSE produces a valid SSE payload for a submit_review tool result
// in the real opencode format.
func makeToolCallSSE(sessionID string, args any) string {
	argsJSON, _ := json.Marshal(args)
	payload := fmt.Sprintf(`{"type":"message.part.updated","properties":{"part":{"sessionID":%q,"type":"tool","tool":"submit_review","state":{"status":"completed","input":%s}}}}`,
		sessionID, argsJSON)
	return "event: message.part.updated\ndata: " + payload + "\n\n"
}

func TestRun(t *testing.T) {
	toolArgs := map[string]any{
		"summary":  "Looks good",
		"verdict":  "approve",
		"findings": []any{},
	}
	ssePayload := makeToolCallSSE("sess-1", toolArgs)

	mux := http.NewServeMux()

	mux.HandleFunc("GET /event", func(w http.ResponseWriter, req *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, ssePayload)
		flusher.Flush()
	})

	mux.HandleFunc("POST /session", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sessionResponse{ID: "sess-1"})
	})

	mux.HandleFunc("POST /session/{id}/message", func(w http.ResponseWriter, req *http.Request) {
		var mr messageRequest
		if err := json.NewDecoder(req.Body).Decode(&mr); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if mr.Agent != "reviewer" {
			t.Errorf("agent = %q, want reviewer", mr.Agent)
		}
		if mr.Model == nil {
			t.Error("model is nil, expected object")
		} else {
			if mr.Model.ProviderID != "test" {
				t.Errorf("model.providerID = %q, want test", mr.Model.ProviderID)
			}
			if mr.Model.ModelID != "model" {
				t.Errorf("model.modelID = %q, want model", mr.Model.ModelID)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(messageResponse{
			Info: messageInfo{ID: "msg-1"},
			Parts: []messagePart{
				{Type: "text", Text: "Review result "},
				{Type: "text", Text: "part 2"},
			},
		})
	})

	mux.HandleFunc("DELETE /session/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("GET /session/{id}/message", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]sessionMessage{
			{Info: sessionMessageInfo{Role: "assistant", Cost: 0.05, Tokens: tokenUsage{Input: 100, Output: 50}}},
		})
	})

	mux.HandleFunc("GET /session/{id}/children", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]sessionResponse{})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	r := newTestRunner(config.OpenCodeConfig{
		Endpoint:     srv.URL,
		Model:        "test/model",
		StageTimeout: 10,
	})

	result, err := collectRunResult(t, r.Run(context.Background(), RunRequest{
		Prompt:    "review this",
		ToolName:  "submit_review",
		AgentName: "reviewer",
	}))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if result.ToolArgs == nil {
		t.Fatal("ToolArgs is nil, expected non-nil when agent calls submit_review")
	}

	var got map[string]any
	if err := json.Unmarshal(result.ToolArgs, &got); err != nil {
		t.Fatalf("unmarshal ToolArgs: %v", err)
	}
	if got["verdict"] != "approve" {
		t.Errorf("verdict = %v, want approve", got["verdict"])
	}
	if got["summary"] != "Looks good" {
		t.Errorf("summary = %v, want Looks good", got["summary"])
	}

	if result.Stats.Cost <= 0 {
		t.Error("Stats.Cost should be > 0")
	}
	if result.Stats.Tokens.Input <= 0 {
		t.Error("Stats.Tokens.Input should be > 0")
	}
}

func TestRunRequestModelOverride(t *testing.T) {
	toolArgs := map[string]any{
		"summary":  "Looks good",
		"verdict":  "approve",
		"findings": []any{},
	}
	ssePayload := makeToolCallSSE("sess-override", toolArgs)

	var gotModel *messageModel

	mux := http.NewServeMux()
	mux.HandleFunc("GET /event", func(w http.ResponseWriter, req *http.Request) {
		flusher := w.(http.Flusher)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, ssePayload)
		flusher.Flush()
	})
	mux.HandleFunc("POST /session", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sessionResponse{ID: "sess-override"})
	})
	mux.HandleFunc("POST /session/{id}/message", func(w http.ResponseWriter, req *http.Request) {
		var mr messageRequest
		if err := json.NewDecoder(req.Body).Decode(&mr); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		gotModel = mr.Model
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(messageResponse{
			Info:  messageInfo{ID: "msg-override"},
			Parts: []messagePart{{Type: "text", Text: "ok"}},
		})
	})
	mux.HandleFunc("DELETE /session/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /session/{id}/children", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]sessionResponse{})
	})
	mux.HandleFunc("GET /session/{id}/message", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]sessionMessage{})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	r := newTestRunner(config.OpenCodeConfig{
		Endpoint:     srv.URL,
		Model:        "default/model",
		StageTimeout: 10,
	})

	result, err := collectRunResult(t, r.Run(context.Background(), RunRequest{
		Prompt:    "review this",
		ToolName:  "submit_review",
		AgentName: "reviewer",
		Model:     "override/model",
	}))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result == nil || result.ToolArgs == nil {
		t.Fatal("expected tool args")
	}
	if gotModel == nil {
		t.Fatal("model is nil")
	}
	if gotModel.ProviderID != "override" || gotModel.ModelID != "model" {
		t.Errorf("model = %+v, want override/model", gotModel)
	}
	if r.cfg.Model != "default/model" {
		t.Errorf("runner cfg model mutated to %q", r.cfg.Model)
	}
}

func TestRunRetry(t *testing.T) {
	toolArgs := map[string]any{
		"summary":  "Issues found",
		"verdict":  "request_changes",
		"findings": []any{},
	}
	ssePayload := makeToolCallSSE("sess-retry", toolArgs)

	var sseCallCount atomic.Int32
	var msgPrompts []string

	mux := http.NewServeMux()

	mux.HandleFunc("GET /event", func(w http.ResponseWriter, req *http.Request) {
		count := sseCallCount.Add(1)
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		if count >= 3 {
			// On the 3rd attempt, return the tool result.
			fmt.Fprint(w, ssePayload)
		}
		// Otherwise close the stream immediately (no tool call event).
		flusher.Flush()
	})

	mux.HandleFunc("POST /session", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sessionResponse{ID: "sess-retry"})
	})

	mux.HandleFunc("POST /session/{id}/message", func(w http.ResponseWriter, req *http.Request) {
		var mr messageRequest
		if err := json.NewDecoder(req.Body).Decode(&mr); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if len(mr.Parts) > 0 {
			msgPrompts = append(msgPrompts, mr.Parts[0].Text)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(messageResponse{
			Info: messageInfo{ID: "msg-retry"},
			Parts: []messagePart{
				{Type: "text", Text: "some text"},
			},
		})
	})

	mux.HandleFunc("DELETE /session/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("GET /session/{id}/children", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]sessionResponse{})
	})

	mux.HandleFunc("GET /session/{id}/message", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]sessionMessage{})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	r := newTestRunner(config.OpenCodeConfig{
		Endpoint:     srv.URL,
		Model:        "test/model",
		StageTimeout: 30,
	})

	const testTool = "submit_review"
	result, err := collectRunResult(t, r.Run(context.Background(), RunRequest{Prompt: "review this", ToolName: testTool, AgentName: "reviewer"}))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if result.ToolArgs == nil {
		t.Fatal("ToolArgs is nil, expected non-nil after retry succeeded")
	}

	var got map[string]any
	if err := json.Unmarshal(result.ToolArgs, &got); err != nil {
		t.Fatalf("unmarshal ToolArgs: %v", err)
	}
	if got["verdict"] != "request_changes" {
		t.Errorf("verdict = %v, want request_changes", got["verdict"])
	}

	// Verify that retry prompts were sent on subsequent attempts.
	if len(msgPrompts) < 2 {
		t.Errorf("expected at least 2 messages (original + retry), got %d", len(msgPrompts))
	}
	wantRetry := retryPromptFor(testTool)
	for i, p := range msgPrompts[1:] {
		if p != wantRetry {
			t.Errorf("msgPrompts[%d] = %q, want retryPromptFor(%q)", i+1, p, testTool)
		}
	}
}

func TestRunRetriesRetryableSendMessageErrorWithContinuationPrompt(t *testing.T) {
	toolArgs := map[string]any{
		"summary":  "Recovered",
		"verdict":  "approve",
		"findings": []any{},
	}
	ssePayload := makeToolCallSSE("sess-send-retry", toolArgs)

	var messageCalls atomic.Int32
	var prompts []string
	var mu sync.Mutex

	mux := http.NewServeMux()
	mux.HandleFunc("GET /event", func(w http.ResponseWriter, req *http.Request) {
		flusher := w.(http.Flusher)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		deadline := time.NewTimer(100 * time.Millisecond)
		defer deadline.Stop()
		ticker := time.NewTicker(time.Millisecond)
		defer ticker.Stop()
		for messageCalls.Load() < 3 {
			select {
			case <-req.Context().Done():
				return
			case <-deadline.C:
				flusher.Flush()
				return
			case <-ticker.C:
			}
		}
		fmt.Fprint(w, ssePayload)
		flusher.Flush()
	})
	mux.HandleFunc("POST /session", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sessionResponse{ID: "sess-send-retry"})
	})
	mux.HandleFunc("POST /session/{id}/message", func(w http.ResponseWriter, req *http.Request) {
		var mr messageRequest
		if err := json.NewDecoder(req.Body).Decode(&mr); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if len(mr.Parts) > 0 {
			mu.Lock()
			prompts = append(prompts, mr.Parts[0].Text)
			mu.Unlock()
		}
		call := messageCalls.Add(1)
		if call < 3 {
			http.Error(w, "retry later", http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(messageResponse{
			Info:  messageInfo{ID: "msg-send-retry"},
			Parts: []messagePart{{Type: "text", Text: "ok"}},
		})
	})
	mux.HandleFunc("DELETE /session/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /session/{id}/children", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]sessionResponse{})
	})
	mux.HandleFunc("GET /session/{id}/message", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]sessionMessage{})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	r := newTestRunner(config.OpenCodeConfig{
		Endpoint:     srv.URL,
		Model:        "test/model",
		StageTimeout: 30,
	})

	result, err := collectRunResult(t, r.Run(context.Background(), RunRequest{
		Prompt:    "review this",
		ToolName:  "submit_review",
		AgentName: "reviewer",
	}))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result == nil || result.ToolArgs == nil {
		t.Fatal("expected tool result after retry")
	}

	mu.Lock()
	gotPrompts := append([]string(nil), prompts...)
	mu.Unlock()
	if len(gotPrompts) != 3 {
		t.Fatalf("prompts = %d, want 3", len(gotPrompts))
	}
	for i, prompt := range gotPrompts[1:] {
		if prompt != continuationPromptFor() {
			t.Errorf("prompt[%d] = %q, want continuation prompt", i+1, prompt)
		}
	}
}

func TestRunCodeErrorSwitchesModelInSameSession(t *testing.T) {
	toolArgs := map[string]any{
		"summary":  "Recovered",
		"verdict":  "approve",
		"findings": []any{},
	}
	ssePayload := makeToolCallSSE("sess-code-switch", toolArgs)

	var mu sync.Mutex
	var models []string
	var sessionIDs []string
	successReady := make(chan struct{})
	var successOnce sync.Once

	mux := http.NewServeMux()
	mux.HandleFunc("GET /event", func(w http.ResponseWriter, req *http.Request) {
		flusher := w.(http.Flusher)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		select {
		case <-successReady:
			fmt.Fprint(w, ssePayload)
		case <-req.Context().Done():
			return
		case <-time.After(200 * time.Millisecond):
		}
		flusher.Flush()
	})
	mux.HandleFunc("POST /session", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sessionResponse{ID: "sess-code-switch"})
	})
	mux.HandleFunc("POST /session/{id}/message", func(w http.ResponseWriter, req *http.Request) {
		var mr messageRequest
		if err := json.NewDecoder(req.Body).Decode(&mr); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		model := testModelString(mr.Model)
		mu.Lock()
		models = append(models, model)
		sessionIDs = append(sessionIDs, req.PathValue("id"))
		mu.Unlock()
		if model == "p/primary" {
			http.Error(w, "primary rejected", http.StatusBadRequest)
			return
		}
		successOnce.Do(func() { close(successReady) })
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(messageResponse{
			Info:  messageInfo{ID: "msg-code-switch"},
			Parts: []messagePart{{Type: "text", Text: "ok"}},
		})
	})
	mux.HandleFunc("DELETE /session/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /session/{id}/children", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]sessionResponse{})
	})
	mux.HandleFunc("GET /session/{id}/message", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]sessionMessage{})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	r := newTestRunner(config.OpenCodeConfig{
		Endpoint:     srv.URL,
		Model:        "p/default",
		StageTimeout: 30,
	})

	result, err := collectRunResult(t, r.Run(context.Background(), RunRequest{
		Prompt:     "review this",
		ToolName:   "submit_review",
		AgentName:  "reviewer",
		ModelChain: []string{"p/primary", "p/fallback"},
	}))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result == nil || result.ToolArgs == nil {
		t.Fatal("expected tool result after model switch")
	}
	assertStrings(t, result.Stats.Models, []string{"p/primary", "p/fallback"})
	assertStrings(t, result.Stats.FallbackModels, []string{"p/fallback"})

	mu.Lock()
	gotModels := append([]string(nil), models...)
	gotSessionIDs := append([]string(nil), sessionIDs...)
	mu.Unlock()
	assertStrings(t, gotModels, []string{"p/primary", "p/primary", "p/primary", "p/fallback"})
	assertStrings(t, gotSessionIDs, []string{"sess-code-switch", "sess-code-switch", "sess-code-switch", "sess-code-switch"})
}

func TestRunToolMissDoesNotSwitchModelChain(t *testing.T) {
	var mu sync.Mutex
	var models []string

	mux := http.NewServeMux()
	mux.HandleFunc("GET /event", func(w http.ResponseWriter, _ *http.Request) {
		flusher := w.(http.Flusher)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()
	})
	mux.HandleFunc("POST /session", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sessionResponse{ID: "sess-tool-miss"})
	})
	mux.HandleFunc("POST /session/{id}/message", func(w http.ResponseWriter, req *http.Request) {
		var mr messageRequest
		if err := json.NewDecoder(req.Body).Decode(&mr); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		models = append(models, testModelString(mr.Model))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(messageResponse{
			Info:  messageInfo{ID: "msg-tool-miss"},
			Parts: []messagePart{{Type: "text", Text: "fallback text"}},
		})
	})
	mux.HandleFunc("DELETE /session/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /session/{id}/children", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]sessionResponse{})
	})
	mux.HandleFunc("GET /session/{id}/message", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]sessionMessage{})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	r := newTestRunner(config.OpenCodeConfig{
		Endpoint:     srv.URL,
		Model:        "p/default",
		StageTimeout: 30,
	})

	result, err := collectRunResult(t, r.Run(context.Background(), RunRequest{
		Prompt:     "review this",
		ToolName:   "submit_review",
		AgentName:  "reviewer",
		ModelChain: []string{"p/primary", "p/fallback"},
	}))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result == nil || result.FallbackText != "fallback text" {
		t.Fatalf("fallback text = %q, want fallback text", result.FallbackText)
	}
	assertStrings(t, result.Stats.Models, []string{"p/primary"})
	assertStrings(t, result.Stats.FallbackModels, nil)

	mu.Lock()
	gotModels := append([]string(nil), models...)
	mu.Unlock()
	assertStrings(t, gotModels, []string{"p/primary", "p/primary", "p/primary", "p/primary"})
}

func TestRunConfiguredModelChainMarksFallbackAfterPrecheckSkip(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /event", func(w http.ResponseWriter, _ *http.Request) {
		flusher := w.(http.Flusher)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, makeToolCallSSE("sess-skip-primary", map[string]any{
			"summary":  "ok",
			"verdict":  "approve",
			"findings": []any{},
		}))
		flusher.Flush()
	})
	mux.HandleFunc("POST /session", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sessionResponse{ID: "sess-skip-primary"})
	})
	mux.HandleFunc("POST /session/{id}/message", func(w http.ResponseWriter, req *http.Request) {
		var mr messageRequest
		if err := json.NewDecoder(req.Body).Decode(&mr); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if got := testModelString(mr.Model); got != "p/fallback" {
			t.Fatalf("model = %q, want p/fallback", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(messageResponse{
			Info:  messageInfo{ID: "msg-skip-primary"},
			Parts: []messagePart{{Type: "text", Text: "ok"}},
		})
	})
	mux.HandleFunc("DELETE /session/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /session/{id}/children", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]sessionResponse{})
	})
	mux.HandleFunc("GET /session/{id}/message", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]sessionMessage{
			{Info: sessionMessageInfo{
				Role:       "assistant",
				Cost:       0.05,
				Tokens:     tokenUsage{Input: 10, Output: 5},
				ProviderID: "p",
				ModelID:    "fallback",
			}},
		})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	r := newTestRunner(config.OpenCodeConfig{
		Endpoint:     srv.URL,
		Model:        "p/default",
		StageTimeout: 30,
	})

	result, err := collectRunResult(t, r.Run(context.Background(), RunRequest{
		Prompt:               "review this",
		ToolName:             "submit_review",
		AgentName:            "reviewer",
		ModelChain:           []string{"p/fallback"},
		ConfiguredModelChain: []string{"p/primary", "p/fallback"},
	}))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertStrings(t, result.Stats.Models, []string{"p/fallback"})
	assertStrings(t, result.Stats.FallbackModels, []string{"p/fallback"})
}

func TestRunRetriesSSEToolErrorWithContinuationPrompt(t *testing.T) {
	errorPayload := makeSubmitReviewSSE("sess-sse-error", map[string]any{"findings": []any{}})
	errorPayload = strings.Replace(errorPayload, `"status":"completed"`, `"status":"error"`, 1)
	successPayload := makeToolCallSSE("sess-sse-error", map[string]any{
		"summary":  "Recovered",
		"verdict":  "approve",
		"findings": []any{},
	})

	var eventCalls atomic.Int32
	var prompts []string
	var mu sync.Mutex

	mux := http.NewServeMux()
	mux.HandleFunc("GET /event", func(w http.ResponseWriter, req *http.Request) {
		call := eventCalls.Add(1)
		flusher := w.(http.Flusher)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if call == 1 {
			fmt.Fprint(w, errorPayload)
		} else {
			fmt.Fprint(w, successPayload)
		}
		flusher.Flush()
	})
	mux.HandleFunc("POST /session", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sessionResponse{ID: "sess-sse-error"})
	})
	mux.HandleFunc("POST /session/{id}/message", func(w http.ResponseWriter, req *http.Request) {
		var mr messageRequest
		if err := json.NewDecoder(req.Body).Decode(&mr); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if len(mr.Parts) > 0 {
			mu.Lock()
			prompts = append(prompts, mr.Parts[0].Text)
			mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(messageResponse{
			Info:  messageInfo{ID: "msg-sse-error"},
			Parts: []messagePart{{Type: "text", Text: "ok"}},
		})
	})
	mux.HandleFunc("DELETE /session/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /session/{id}/children", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]sessionResponse{})
	})
	mux.HandleFunc("GET /session/{id}/message", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]sessionMessage{})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	r := newTestRunner(config.OpenCodeConfig{
		Endpoint:     srv.URL,
		Model:        "test/model",
		StageTimeout: 30,
	})

	result, err := collectRunResult(t, r.Run(context.Background(), RunRequest{
		Prompt:    "review this",
		ToolName:  "submit_review",
		AgentName: "reviewer",
	}))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result == nil || result.ToolArgs == nil {
		t.Fatal("expected tool result after retry")
	}

	mu.Lock()
	gotPrompts := append([]string(nil), prompts...)
	mu.Unlock()
	if len(gotPrompts) != 2 {
		t.Fatalf("prompts = %d, want 2", len(gotPrompts))
	}
	if gotPrompts[1] != continuationPromptFor() {
		t.Errorf("prompt[1] = %q, want continuation prompt", gotPrompts[1])
	}
}

func TestRunFallback(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /event", func(w http.ResponseWriter, req *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		// No tool call event — stream ends immediately.
		flusher.Flush()
	})

	mux.HandleFunc("POST /session", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sessionResponse{ID: "sess-fallback"})
	})

	mux.HandleFunc("POST /session/{id}/message", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(messageResponse{
			Info: messageInfo{ID: "msg-fallback"},
			Parts: []messagePart{
				{Type: "text", Text: "Review result "},
				{Type: "text", Text: "part 2"},
			},
		})
	})

	mux.HandleFunc("DELETE /session/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("GET /session/{id}/children", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]sessionResponse{})
	})

	mux.HandleFunc("GET /session/{id}/message", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]sessionMessage{})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	r := newTestRunner(config.OpenCodeConfig{
		Endpoint:     srv.URL,
		Model:        "test/model",
		StageTimeout: 30,
	})

	result, err := collectRunResult(t, r.Run(context.Background(), RunRequest{Prompt: "review this", ToolName: "submit_review", AgentName: "reviewer"}))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if result.ToolArgs != nil {
		t.Error("ToolArgs should be nil in fallback mode")
	}
	if result.FallbackText != "Review result part 2" {
		t.Errorf("FallbackText = %q, want %q", result.FallbackText, "Review result part 2")
	}
}

func TestRunTimeout(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /session", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sessionResponse{ID: "sess-timeout"})
	})

	abortCalled := make(chan struct{}, 1)
	handlerDone := make(chan struct{})

	mux.HandleFunc("POST /session/{id}/message", func(w http.ResponseWriter, _ *http.Request) {
		// Block until test signals completion so the client times out.
		<-handlerDone
	})

	mux.HandleFunc("POST /session/{id}/abort", func(w http.ResponseWriter, _ *http.Request) {
		abortCalled <- struct{}{}
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("DELETE /session/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("GET /session/{id}/children", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]sessionResponse{})
	})

	mux.HandleFunc("GET /event", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		<-req.Context().Done()
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()
	defer close(handlerDone)

	r := newTestRunner(config.OpenCodeConfig{
		Endpoint:     srv.URL,
		Model:        "test/model",
		StageTimeout: 1,
	})

	_, err := collectRunResult(t, r.Run(context.Background(), RunRequest{
		Prompt:    "slow review",
		ToolName:  "submit_review",
		AgentName: "reviewer",
	}))
	if err == nil {
		t.Fatal("Run should return error on timeout")
	}

	select {
	case <-abortCalled:
	case <-time.After(3 * time.Second):
		t.Error("abort was not called after timeout")
	}
}

func TestStopServeNilProc(t *testing.T) {
	r := newTestRunner(config.OpenCodeConfig{Endpoint: "http://localhost:9999"})
	r.StopServe() // should not panic
}

func TestFetchSessionStats_Non200(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /session/{id}/message", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	r := newTestRunner(config.OpenCodeConfig{Endpoint: srv.URL})
	_, err := r.fetchSessionStats("test")
	if err == nil {
		t.Fatal("fetchSessionStats should return error for non-200 status")
	}
	if !strings.Contains(err.Error(), "unexpected status 500") {
		t.Errorf("error = %v, want 'unexpected status 500'", err)
	}
}

func TestGetSessionStats_WithChildSessions(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /session/parent/message", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]sessionMessage{
			{Info: sessionMessageInfo{Role: "assistant", Cost: 0.10, Tokens: tokenUsage{Input: 100, Output: 50}, ProviderID: "p", ModelID: "primary"}},
		})
	})
	mux.HandleFunc("GET /session/child1/message", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]sessionMessage{
			{Info: sessionMessageInfo{Role: "assistant", Cost: 0.05, Tokens: tokenUsage{Input: 50, Output: 25}, Model: &messageModel{ProviderID: "p", ModelID: "fallback"}}},
		})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	r := newTestRunner(config.OpenCodeConfig{Endpoint: srv.URL})

	var childSessions sync.Map
	childSessions.Store("child1", true)

	stats := r.getSessionStats("parent", &childSessions)
	if stats.Cost < 0.1499 || stats.Cost > 0.1501 {
		t.Errorf("Cost = %v, want ~0.15", stats.Cost)
	}
	if stats.Tokens.Input != 150 {
		t.Errorf("Tokens.Input = %d, want 150", stats.Tokens.Input)
	}
	if stats.Tokens.Output != 75 {
		t.Errorf("Tokens.Output = %d, want 75", stats.Tokens.Output)
	}
	assertStrings(t, stats.Models, []string{"p/primary", "p/fallback"})
	assertModelCostStrings(t, stats.ModelCosts, []ModelCost{
		{Model: "p/primary", Cost: 0.10, Tokens: tokenUsage{Input: 100, Output: 50}},
		{Model: "p/fallback", Cost: 0.05, Tokens: tokenUsage{Input: 50, Output: 25}},
	})
}

func TestSanitizeLogValue(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{"empty", "", 512, ""},
		{"short", "hello", 512, "hello"},
		{"truncate", "abcdef", 3, "abc"},
		{"exact_len", "abc", 3, "abc"},
		{"newlines", "line1\nline2\rline3", 512, "line1 line2 line3"},
		{"truncate_and_sanitize", "aaa\nbbb\nccc", 7, "aaa bbb"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeLogValue(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("sanitizeLogValue(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}

func makeSubmitReviewSSE(sessionID string, args any) string {
	argsJSON, _ := json.Marshal(args)
	payload := fmt.Sprintf(`{"type":"message.part.updated","properties":{"part":{"sessionID":%q,"type":"tool","tool":%q,"state":{"status":"completed","input":%s}}}}`,
		sessionID, "submit_review", argsJSON)
	return "event: message.part.updated\ndata: " + payload + "\n\n"
}

func TestRunSchemaRetry(t *testing.T) {
	const toolName = "submit_review"
	const sessionID = "sess-schema"

	invalidArgs := map[string]any{
		"summary":  "Issues found",
		"verdict":  "INVALID",
		"findings": []any{},
	}
	validArgs := map[string]any{
		"summary":  "Issues found",
		"verdict":  "approve",
		"findings": []any{},
	}

	invalidSSE := makeSubmitReviewSSE(sessionID, invalidArgs)
	validSSE := makeSubmitReviewSSE(sessionID, validArgs)

	var sseCallCount atomic.Int32
	var msgCallCount atomic.Int32
	var msgPrompts []string
	var msgMu sync.Mutex

	mux := http.NewServeMux()

	mux.HandleFunc("GET /event", func(w http.ResponseWriter, req *http.Request) {
		count := sseCallCount.Add(1)
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		if count == 1 {
			fmt.Fprint(w, invalidSSE)
		} else {
			fmt.Fprint(w, validSSE)
		}
		flusher.Flush()
	})

	mux.HandleFunc("POST /session", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sessionResponse{ID: sessionID})
	})

	mux.HandleFunc("POST /session/{id}/message", func(w http.ResponseWriter, req *http.Request) {
		msgCallCount.Add(1)
		var mr messageRequest
		if err := json.NewDecoder(req.Body).Decode(&mr); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if len(mr.Parts) > 0 {
			msgMu.Lock()
			msgPrompts = append(msgPrompts, mr.Parts[0].Text)
			msgMu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(messageResponse{
			Info:  messageInfo{ID: "msg-schema"},
			Parts: []messagePart{{Type: "text", Text: "ok"}},
		})
	})

	mux.HandleFunc("DELETE /session/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("GET /session/{id}/children", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]sessionResponse{})
	})

	mux.HandleFunc("GET /session/{id}/message", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]sessionMessage{
			{Info: sessionMessageInfo{Role: "assistant", Cost: 0.01}},
		})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	validVerdicts := map[string]bool{
		"approve": true, "request_changes": true, "comment_only": true, "skipped": true,
	}
	validateFunc := func(data json.RawMessage) error {
		var v struct {
			Verdict string `json:"verdict"`
		}
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		if !validVerdicts[v.Verdict] {
			return fmt.Errorf("invalid verdict %q", v.Verdict)
		}
		return nil
	}

	r := newTestRunner(config.OpenCodeConfig{
		Endpoint:     srv.URL,
		Model:        "test/model",
		StageTimeout: 30,
	})

	result, err := collectRunResult(t, r.Run(context.Background(), RunRequest{
		Prompt:       "review this",
		ToolName:     toolName,
		AgentName:    "reviewer",
		ValidateFunc: validateFunc,
		SchemaHint:   `{"verdict":"approve|request_changes|comment_only|skipped"}`,
	}))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if result == nil {
		t.Fatal("result is nil")
	}
	if result.ToolArgs == nil {
		t.Fatal("ToolArgs is nil")
	}

	var got map[string]any
	if err := json.Unmarshal(result.ToolArgs, &got); err != nil {
		t.Fatalf("unmarshal ToolArgs: %v", err)
	}
	if got["verdict"] != "approve" {
		t.Errorf("verdict = %v, want approve", got["verdict"])
	}

	msgMu.Lock()
	prompts := msgPrompts
	msgMu.Unlock()

	foundSchemaRetry := false
	for _, p := range prompts {
		if strings.Contains(p, "did not match the required schema") {
			foundSchemaRetry = true
			break
		}
	}
	if !foundSchemaRetry {
		t.Errorf("expected at least one schema retry prompt, got prompts: %v", prompts)
	}
}

func TestRunSchemaRetryExhausted(t *testing.T) {
	const toolName = "submit_review"
	const sessionID = "sess-schema-exhaust"

	invalidArgs := map[string]any{
		"summary":  "Issues found",
		"verdict":  "INVALID",
		"findings": []any{},
	}
	invalidSSE := makeSubmitReviewSSE(sessionID, invalidArgs)

	var sseCallCount atomic.Int32
	var schemaRetryCount atomic.Int32
	var msgMu sync.Mutex
	var msgPrompts []string

	mux := http.NewServeMux()

	mux.HandleFunc("GET /event", func(w http.ResponseWriter, req *http.Request) {
		sseCallCount.Add(1)
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, invalidSSE)
		flusher.Flush()
	})

	mux.HandleFunc("POST /session", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sessionResponse{ID: sessionID})
	})

	mux.HandleFunc("POST /session/{id}/message", func(w http.ResponseWriter, req *http.Request) {
		var mr messageRequest
		if err := json.NewDecoder(req.Body).Decode(&mr); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if len(mr.Parts) > 0 {
			msgMu.Lock()
			msgPrompts = append(msgPrompts, mr.Parts[0].Text)
			msgMu.Unlock()
			if strings.Contains(mr.Parts[0].Text, "did not match the required schema") {
				schemaRetryCount.Add(1)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(messageResponse{
			Info:  messageInfo{ID: "msg-exhaust"},
			Parts: []messagePart{{Type: "text", Text: "ok"}},
		})
	})

	mux.HandleFunc("DELETE /session/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("GET /session/{id}/children", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]sessionResponse{})
	})

	mux.HandleFunc("GET /session/{id}/message", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]sessionMessage{
			{Info: sessionMessageInfo{Role: "assistant", Cost: 0.01}},
		})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	validVerdicts := map[string]bool{
		"approve": true, "request_changes": true, "comment_only": true, "skipped": true,
	}
	validateFunc := func(data json.RawMessage) error {
		var v struct {
			Verdict string `json:"verdict"`
		}
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		if !validVerdicts[v.Verdict] {
			return fmt.Errorf("invalid verdict %q", v.Verdict)
		}
		return nil
	}

	r := newTestRunner(config.OpenCodeConfig{
		Endpoint:     srv.URL,
		Model:        "test/model",
		StageTimeout: 30,
	})

	result, err := collectRunResult(t, r.Run(context.Background(), RunRequest{
		Prompt:       "review this",
		ToolName:     toolName,
		AgentName:    "reviewer",
		ValidateFunc: validateFunc,
		SchemaHint:   `{"verdict":"approve|request_changes|comment_only|skipped"}`,
	}))
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if result == nil {
		t.Fatal("result is nil, expected final result even when schema retries exhausted")
	}
	if result.ToolArgs == nil {
		t.Fatal("ToolArgs is nil, expected last invalid args to be emitted")
	}

	var got map[string]any
	if err := json.Unmarshal(result.ToolArgs, &got); err != nil {
		t.Fatalf("unmarshal ToolArgs: %v", err)
	}
	if got["verdict"] != "INVALID" {
		t.Errorf("verdict = %v, want INVALID (last invalid args)", got["verdict"])
	}

	if schemaRetryCount.Load() != 2 {
		t.Errorf("schema retry count = %d, want %d", schemaRetryCount.Load(), 2)
	}
}
