package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/aaudin90/opencode-reviewer/internal/shared/config"
	"github.com/aaudin90/opencode-reviewer/internal/shared/models"
	"github.com/aaudin90/opencode-reviewer/internal/shared/runner"
)

func TestFinalizerStage_Run_Success(t *testing.T) {
	toolArgs := map[string]any{
		"summary": "All good",
		"verdict": "approve",
		"findings": []map[string]any{
			{
				"file":           "main.go",
				"start_line":     1,
				"end_line":       5,
				"existing_code":  "func main()",
				"confidence":     "high",
				"issue_content":  "test finding",
				"recommendation": "fix it",
				"sources":        []string{"reviewer-1"},
			},
		},
	}

	srv := newPipelineTestServer(t, "submit_final_review", toolArgs)

	r := runner.New(config.OpenCodeConfig{
		Endpoint:     srv.URL,
		Model:        "test/model",
		StageTimeout: 30,
	}, "/tmp", nil)

	stage := NewFinalizerStage(FinalizerStageConfig{
		Runner:           r,
		FinalizerMessage: "Consolidate the reviews",
	})

	phase1Results := []*models.ReviewResult{
		{Verdict: "approve", Summary: "LGTM"},
	}

	result, stats, err := stage.Run(context.Background(), phase1Results)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Verdict != "approve" {
		t.Errorf("verdict = %q, want approve", result.Verdict)
	}
	if stats.Cost <= 0 {
		t.Error("stats.Cost should be > 0")
	}
}

func TestFinalizerStage_Run_WithValidateFunc(t *testing.T) {
	// When tool args have invalid verdict, the stage still returns a result with ParseErr set.
	toolArgs := map[string]any{
		"summary":  "Consolidated",
		"verdict":  "INVALID_VERDICT",
		"findings": []any{},
	}

	srv := newPipelineTestServer(t, "submit_final_review", toolArgs)

	r := runner.New(config.OpenCodeConfig{
		Endpoint:     srv.URL,
		Model:        "test/model",
		StageTimeout: 30,
	}, "/tmp", nil)

	stage := NewFinalizerStage(FinalizerStageConfig{
		Runner:           r,
		FinalizerMessage: "Consolidate the reviews",
	})

	phase1Results := []*models.ReviewResult{
		{Verdict: "approve", Summary: "LGTM"},
	}

	result, _, err := stage.Run(context.Background(), phase1Results)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
	if result.ParseErr == nil {
		t.Error("ParseErr should be set for invalid verdict")
	}
	if result.Verdict != "INVALID_VERDICT" {
		t.Errorf("Verdict = %q, want INVALID_VERDICT", result.Verdict)
	}
}

func TestFinalizerStage_Run_FallbackText(t *testing.T) {
	// When the agent never calls the tool, the runner goes through the JSON fallback path.
	// The finalizer message response contains valid JSON that ParseFinal can parse.
	// The SSE never emits a tool call, so FallbackText is populated from the last sendMessage parts.
	var sessionCounter atomic.Int32

	validFallbackJSON := `{"summary":"Consolidated","verdict":"approve","findings":[{"file":"x.go","start_line":1,"end_line":2,"existing_code":"a","confidence":"high","issue_content":"bug","recommendation":"fix","sources":["r1"]}]}`

	mux := http.NewServeMux()

	mux.HandleFunc("GET /global/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("POST /session", func(w http.ResponseWriter, _ *http.Request) {
		n := sessionCounter.Add(1)
		id := fmt.Sprintf("sess-%d", n)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"id": id})
	})

	mux.HandleFunc("POST /session/{id}/message", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"info":  map[string]any{"id": "msg-1", "tokens": map[string]any{"input": 0, "output": 0}},
			"parts": []map[string]any{{"type": "text", "text": validFallbackJSON}},
		})
	})

	// SSE never emits a tool call — stream closes immediately, triggering retries and then JSON fallback.
	mux.HandleFunc("GET /event", func(w http.ResponseWriter, _ *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "no flusher", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()
	})

	mux.HandleFunc("GET /session/{id}/message", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"info": map[string]any{"role": "assistant", "cost": 0.05, "tokens": map[string]any{"input": 100, "output": 50}}},
		})
	})

	mux.HandleFunc("GET /session/{id}/children", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]any{})
	})

	mux.HandleFunc("DELETE /session/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	r := runner.New(config.OpenCodeConfig{
		Endpoint:     srv.URL,
		Model:        "test/model",
		StageTimeout: 30,
	}, "/tmp", nil)

	stage := NewFinalizerStage(FinalizerStageConfig{
		Runner:           r,
		FinalizerMessage: "Consolidate",
	})

	phase1Results := []*models.ReviewResult{
		{Verdict: "approve", Summary: "LGTM"},
	}

	result, _, err := stage.Run(context.Background(), phase1Results)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// ParseFinal should successfully parse the valid JSON from the fallback text.
	if result.Verdict != "approve" {
		t.Errorf("verdict = %q, want approve", result.Verdict)
	}
	if len(result.Findings) != 1 {
		t.Errorf("len(Findings) = %d, want 1", len(result.Findings))
	}
}
