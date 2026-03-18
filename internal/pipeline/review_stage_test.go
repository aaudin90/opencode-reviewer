package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/aaudin90/opencode-reviewer/internal/config"
	"github.com/aaudin90/opencode-reviewer/internal/runner"
)

func TestReviewStage_Run_EmptyMessages(t *testing.T) {
	stage := NewReviewStage(ReviewStageConfig{Messages: nil})
	_, _, err := stage.Run(context.Background())
	if err == nil {
		t.Fatal("expected error for empty messages")
	}
	if !strings.Contains(err.Error(), "no review messages configured") {
		t.Errorf("error = %v, want 'no review messages configured'", err)
	}
}

func TestReviewStage_Run_SingleSuccess(t *testing.T) {
	toolArgs := map[string]any{
		"reviewer_name": "security",
		"summary":       "LGTM",
		"verdict":       "approve",
		"findings":      []any{},
	}

	srv := newPipelineTestServer(t, "submit_review", toolArgs)

	r := runner.New(config.OpenCodeConfig{
		Endpoint:     srv.URL,
		Model:        "test/model",
		StageTimeout: 30,
	}, "/tmp", nil)

	stage := NewReviewStage(ReviewStageConfig{
		Runner:   r,
		Messages: []string{"review this code"},
	})

	results, stats, err := stage.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].Verdict != "approve" {
		t.Errorf("verdict = %q, want approve", results[0].Verdict)
	}
	if len(stats) != 1 {
		t.Fatalf("len(stats) = %d, want 1", len(stats))
	}
	if stats[0].Cost <= 0 {
		t.Error("stats.Cost should be > 0")
	}
}

func TestReviewStage_Run_AllSessionsFail(t *testing.T) {
	// Server where ALL review sessions' sendMessage returns 500, so both fail.
	var sessionCounter atomic.Int32

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

	mux.HandleFunc("POST /session/{id}/message", func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.PathValue("id")
		// Precheck session (sess-1) always succeeds
		if sessionID == "sess-1" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"info":  map[string]any{"id": "msg-1", "tokens": map[string]any{"input": 0, "output": 0}},
				"parts": []map[string]any{{"type": "text", "text": "pong"}},
			})
			return
		}
		// All review sessions fail
		w.WriteHeader(http.StatusInternalServerError)
	})

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
		_ = json.NewEncoder(w).Encode([]any{})
	})

	mux.HandleFunc("GET /session/{id}/children", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]any{})
	})

	mux.HandleFunc("DELETE /session/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("POST /session/{id}/abort", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	r := runner.New(config.OpenCodeConfig{
		Endpoint:     srv.URL,
		Model:        "test/model",
		StageTimeout: 10,
	}, "/tmp", nil)

	stage := NewReviewStage(ReviewStageConfig{
		Runner:   r,
		Messages: []string{"review part 1", "review part 2"},
	})

	_, _, err := stage.Run(context.Background())
	if err == nil {
		t.Fatal("expected error when all sessions fail")
	}
	if !strings.Contains(err.Error(), "2 of 2 review sessions failed") {
		t.Errorf("error = %v, want '2 of 2 review sessions failed'", err)
	}
}

func TestReviewStage_ParseResult_ToolArgsValid(t *testing.T) {
	stage := NewReviewStage(ReviewStageConfig{})

	toolArgs, _ := json.Marshal(map[string]any{
		"reviewer_name": "security",
		"summary":       "Looks good",
		"verdict":       "approve",
		"findings":      []any{},
	})

	result := stage.parseResult(&runner.RunResult{ToolArgs: toolArgs})
	if result.ParseErr != nil {
		t.Errorf("ParseErr = %v, want nil", result.ParseErr)
	}
	if result.Verdict != "approve" {
		t.Errorf("Verdict = %q, want approve", result.Verdict)
	}
	if result.ReviewerName != "security" {
		t.Errorf("ReviewerName = %q, want security", result.ReviewerName)
	}
}

func TestReviewStage_ParseResult_FallbackWins(t *testing.T) {
	stage := NewReviewStage(ReviewStageConfig{})

	// ToolArgs has invalid findings field (string instead of array) -> ParseErr != nil
	toolArgs := json.RawMessage(`{"verdict":"approve","findings":"bad"}`)

	// FallbackText has valid JSON
	fallbackText := `{"reviewer_name":"sec","summary":"ok","verdict":"approve","findings":[{"file":"a.go","start_line":1,"end_line":2,"existing_code":"x","confidence":"high","issue_content":"bug","recommendation":"fix"}]}`

	result := stage.parseResult(&runner.RunResult{
		ToolArgs:     toolArgs,
		FallbackText: fallbackText,
	})
	if result.ParseErr != nil {
		t.Errorf("ParseErr = %v, want nil (fallback should win)", result.ParseErr)
	}
	if result.Verdict != "approve" {
		t.Errorf("Verdict = %q, want approve", result.Verdict)
	}
	if len(result.Findings) != 1 {
		t.Errorf("len(Findings) = %d, want 1", len(result.Findings))
	}
}

func TestReviewStage_ParseResult_NoToolArgs(t *testing.T) {
	stage := NewReviewStage(ReviewStageConfig{})

	fallbackText := `{"reviewer_name":"perf","summary":"ok","verdict":"request_changes","findings":[{"file":"b.go","start_line":5,"end_line":10,"existing_code":"y","confidence":"medium","issue_content":"slow","recommendation":"optimize"}]}`

	result := stage.parseResult(&runner.RunResult{
		FallbackText: fallbackText,
	})
	if result.ParseErr != nil {
		t.Errorf("ParseErr = %v, want nil", result.ParseErr)
	}
	if result.Verdict != "request_changes" {
		t.Errorf("Verdict = %q, want request_changes", result.Verdict)
	}
}
