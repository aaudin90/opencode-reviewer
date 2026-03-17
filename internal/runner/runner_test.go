package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aaudin90/opencode-reviewer/internal/config"
)

// newTestRunner creates a Runner with nil workspace for tests using external endpoint.
func newTestRunner(cfg config.OpenCodeConfig) *Runner {
	return New(cfg, "/tmp", nil)
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
