package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aaudin90/opencode-reviewer/internal/config"
)

// newTestRunner creates a Runner with nil workspace for tests using external endpoint.
func newTestRunner(cfg config.OpenCodeConfig) *Runner {
	return New(cfg, "/tmp", nil)
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

func TestRun(t *testing.T) {
	mux := http.NewServeMux()

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

	srv := httptest.NewServer(mux)
	defer srv.Close()

	r := newTestRunner(config.OpenCodeConfig{
		Endpoint:     srv.URL,
		Model:        "test/model",
		StageTimeout: 10,
	})

	result, err := r.Run(context.Background(), RunRequest{
		Prompt: "review this",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if result != "Review result part 2" {
		t.Errorf("result = %q, want %q", result, "Review result part 2")
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

	srv := httptest.NewServer(mux)
	defer srv.Close()
	defer close(handlerDone)

	r := newTestRunner(config.OpenCodeConfig{
		Endpoint:     srv.URL,
		Model:        "test/model",
		StageTimeout: 1,
	})

	_, err := r.Run(context.Background(), RunRequest{
		Prompt: "slow review",
	})
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
