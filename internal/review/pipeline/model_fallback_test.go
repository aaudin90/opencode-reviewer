package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aaudin90/opencode-reviewer/internal/shared/config"
	"github.com/aaudin90/opencode-reviewer/internal/shared/models"
	"github.com/aaudin90/opencode-reviewer/internal/shared/runner"
)

type modelFallbackServer struct {
	server *httptest.Server

	sessionCounter atomic.Int32
	toolSession    atomic.Value

	mu           sync.Mutex
	precheckSeen []string
	runSeen      []string
	runSessions  []string

	messageStatus func(model string, precheck bool) int
	messageDelay  func(model string, precheck bool) time.Duration
	toolName      string
	toolArgs      any
}

func newModelFallbackServer(t *testing.T, toolName string, toolArgs any, messageStatus func(model string, precheck bool) int) *modelFallbackServer {
	t.Helper()
	s := &modelFallbackServer{
		messageStatus: messageStatus,
		toolName:      toolName,
		toolArgs:      toolArgs,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /global/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("POST /session", func(w http.ResponseWriter, _ *http.Request) {
		id := fmt.Sprintf("sess-%d", s.sessionCounter.Add(1))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"id": id})
	})
	mux.HandleFunc("POST /session/{id}/message", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
			Model *struct {
				ProviderID string `json:"providerID"`
				ModelID    string `json:"modelID"`
			} `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		model := ""
		if body.Model != nil {
			model = body.Model.ProviderID + "/" + body.Model.ModelID
		}
		precheck := len(body.Parts) > 0 && strings.HasPrefix(body.Parts[0].Text, "Health check.")
		s.mu.Lock()
		if precheck {
			s.precheckSeen = append(s.precheckSeen, model)
		} else {
			s.runSeen = append(s.runSeen, model)
			s.runSessions = append(s.runSessions, r.PathValue("id"))
		}
		s.mu.Unlock()

		if s.messageDelay != nil {
			delay := s.messageDelay(model, precheck)
			if delay > 0 {
				select {
				case <-time.After(delay):
				case <-r.Context().Done():
					return
				}
			}
		}

		status := s.messageStatus(model, precheck)
		if status != http.StatusOK {
			http.Error(w, "model failed", status)
			return
		}
		if !precheck {
			s.toolSession.Store(r.PathValue("id"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"info":  map[string]any{"id": "msg-1", "tokens": map[string]any{"input": 0, "output": 1}},
			"parts": []map[string]any{{"type": "text", "text": "ok"}},
		})
	})
	mux.HandleFunc("GET /event", func(w http.ResponseWriter, _ *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "no flusher", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		deadline := time.Now().Add(100 * time.Millisecond)
		for {
			if sid, _ := s.toolSession.Load().(string); sid != "" {
				fmt.Fprint(w, makeToolCallSSEGeneric(sid, s.toolName, s.toolArgs))
				break
			}
			if time.Now().After(deadline) {
				break
			}
			time.Sleep(time.Millisecond)
		}
		flusher.Flush()
	})
	mux.HandleFunc("GET /session/{id}/message", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"info": map[string]any{"role": "assistant", "cost": 0.01, "tokens": map[string]any{"input": 1, "output": 1}}},
		})
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

	s.server = httptest.NewServer(mux)
	t.Cleanup(s.server.Close)
	return s
}

func (s *modelFallbackServer) runner() *runner.Runner {
	return runner.New(config.OpenCodeConfig{
		Endpoint:     s.server.URL,
		Model:        "p/primary",
		StageTimeout: 10,
	}, "/tmp", nil, "test")
}

func (s *modelFallbackServer) runnerWithPrecheckTimeout(timeoutSec int) *runner.Runner {
	return runner.New(config.OpenCodeConfig{
		Endpoint:        s.server.URL,
		Model:           "p/primary",
		StageTimeout:    10,
		PrecheckTimeout: timeoutSec,
	}, "/tmp", nil, "test")
}

func (s *modelFallbackServer) seen() ([]string, []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.precheckSeen...), append([]string(nil), s.runSeen...)
}

func (s *modelFallbackServer) seenRunSessions() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.runSessions...)
}

func TestReviewStage_FallbackPrecheckPrimaryFails(t *testing.T) {
	toolArgs := map[string]any{"reviewer_name": "r", "summary": "ok", "verdict": "approve", "findings": []any{}}
	srv := newModelFallbackServer(t, "submit_review", toolArgs, func(model string, precheck bool) int {
		if precheck && model == "p/primary" {
			return http.StatusInternalServerError
		}
		if !precheck && model != "p/fallback" {
			return http.StatusBadRequest
		}
		return http.StatusOK
	})

	stage := NewReviewStage(ReviewStageConfig{
		Runner:     srv.runner(),
		Messages:   []string{"review"},
		ModelChain: []string{"p/primary", "p/fallback"},
	})
	results, stats, err := stage.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	assertStrings(t, stats[0].Models, []string{"p/fallback"})
	assertStrings(t, stats[0].FallbackModels, []string{"p/fallback"})
	prechecks, runs := srv.seen()
	assertStrings(t, prechecks, []string{"p/primary", "p/fallback"})
	assertStrings(t, runs, []string{"p/fallback"})
}

func TestReviewStage_FallbackPrecheckPrimaryTimeoutTriesNextModel(t *testing.T) {
	toolArgs := map[string]any{"reviewer_name": "r", "summary": "ok", "verdict": "approve", "findings": []any{}}
	srv := newModelFallbackServer(t, "submit_review", toolArgs, func(model string, precheck bool) int {
		if !precheck && model != "p/fallback" {
			return http.StatusBadRequest
		}
		return http.StatusOK
	})
	srv.messageDelay = func(model string, precheck bool) time.Duration {
		if precheck && model == "p/primary" {
			return 1100 * time.Millisecond
		}
		return 0
	}

	stage := NewReviewStage(ReviewStageConfig{
		Runner:     srv.runnerWithPrecheckTimeout(1),
		Messages:   []string{"review"},
		ModelChain: []string{"p/primary", "p/fallback"},
	})
	results, stats, err := stage.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	assertStrings(t, stats[0].Models, []string{"p/fallback"})
	assertStrings(t, stats[0].FallbackModels, []string{"p/fallback"})
	prechecks, runs := srv.seen()
	assertStrings(t, prechecks, []string{"p/primary", "p/fallback"})
	assertStrings(t, runs, []string{"p/fallback"})
}

func TestReviewStage_FallbackPrecheckExternalContextTimeoutStopsChain(t *testing.T) {
	toolArgs := map[string]any{"reviewer_name": "r", "summary": "ok", "verdict": "approve", "findings": []any{}}
	srv := newModelFallbackServer(t, "submit_review", toolArgs, func(_ string, _ bool) int {
		return http.StatusOK
	})
	srv.messageDelay = func(model string, precheck bool) time.Duration {
		if precheck && model == "p/primary" {
			return 200 * time.Millisecond
		}
		return 0
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	stage := NewReviewStage(ReviewStageConfig{
		Runner:     srv.runnerWithPrecheckTimeout(10),
		Messages:   []string{"review"},
		ModelChain: []string{"p/primary", "p/fallback"},
	})
	_, _, err := stage.Run(ctx)
	if err == nil {
		t.Fatal("Run error = nil, want error")
	}
	if !strings.Contains(err.Error(), "reviewer precheck model \"p/primary\"") || !strings.Contains(err.Error(), context.DeadlineExceeded.Error()) {
		t.Fatalf("error = %v, want hard context deadline error", err)
	}
	prechecks, runs := srv.seen()
	assertStrings(t, prechecks, []string{"p/primary"})
	assertStrings(t, runs, nil)
}

func TestReviewStage_FallbackReviewerFailureRetriesNextModel(t *testing.T) {
	toolArgs := map[string]any{"reviewer_name": "r", "summary": "ok", "verdict": "approve", "findings": []any{}}
	srv := newModelFallbackServer(t, "submit_review", toolArgs, func(model string, precheck bool) int {
		if !precheck && model == "p/primary" {
			return http.StatusBadRequest
		}
		return http.StatusOK
	})

	stage := NewReviewStage(ReviewStageConfig{
		Runner:     srv.runner(),
		Messages:   []string{"review"},
		ModelChain: []string{"p/primary", "p/fallback"},
	})
	results, stats, err := stage.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	assertStrings(t, stats[0].Models, []string{"p/primary", "p/fallback"})
	assertStrings(t, stats[0].FallbackModels, []string{"p/fallback"})
	prechecks, runs := srv.seen()
	assertStrings(t, prechecks, []string{"p/primary"})
	assertStrings(t, runs, []string{"p/primary", "p/primary", "p/primary", "p/fallback"})
	assertStrings(t, srv.seenRunSessions(), []string{"sess-2", "sess-2", "sess-2", "sess-2"})
}

func TestFinalizerStage_FallbackFinalizerFailureRetriesNextModelSameSession(t *testing.T) {
	toolArgs := map[string]any{"summary": "ok", "verdict": "approve", "findings": []any{}}
	srv := newModelFallbackServer(t, "submit_final_review", toolArgs, func(model string, precheck bool) int {
		if !precheck && model == "p/primary" {
			return http.StatusBadRequest
		}
		return http.StatusOK
	})

	stage := NewFinalizerStage(FinalizerStageConfig{
		Runner:           srv.runner(),
		FinalizerMessage: "finalize",
		ModelChain:       []string{"p/primary", "p/fallback"},
	})
	result, stats, err := stage.Run(context.Background(), []*models.ReviewResult{{Summary: "ok", Verdict: "approve"}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Verdict != "approve" {
		t.Fatalf("verdict = %q, want approve", result.Verdict)
	}
	assertStrings(t, stats.Models, []string{"p/primary", "p/fallback"})
	assertStrings(t, stats.FallbackModels, []string{"p/fallback"})
	prechecks, runs := srv.seen()
	assertStrings(t, prechecks, []string{"p/primary"})
	assertStrings(t, runs, []string{"p/primary", "p/primary", "p/primary", "p/fallback"})
	assertStrings(t, srv.seenRunSessions(), []string{"sess-2", "sess-2", "sess-2", "sess-2"})
}

func TestFinalizerStage_FallbackStartsFromPrimary(t *testing.T) {
	toolArgs := map[string]any{"summary": "ok", "verdict": "approve", "findings": []any{}}
	srv := newModelFallbackServer(t, "submit_final_review", toolArgs, func(_ string, _ bool) int {
		return http.StatusOK
	})

	stage := NewFinalizerStage(FinalizerStageConfig{
		Runner:           srv.runner(),
		FinalizerMessage: "finalize",
		ModelChain:       []string{"p/primary", "p/fallback"},
	})
	result, stats, err := stage.Run(context.Background(), []*models.ReviewResult{{Summary: "ok", Verdict: "approve"}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Verdict != "approve" {
		t.Fatalf("verdict = %q, want approve", result.Verdict)
	}
	assertStrings(t, stats.Models, []string{"p/primary"})
	assertStrings(t, stats.FallbackModels, nil)
	prechecks, runs := srv.seen()
	assertStrings(t, prechecks, []string{"p/primary"})
	assertStrings(t, runs, []string{"p/primary"})
}

func TestReviewStage_FallbackModelsExhausted(t *testing.T) {
	toolArgs := map[string]any{"reviewer_name": "r", "summary": "ok", "verdict": "approve", "findings": []any{}}
	srv := newModelFallbackServer(t, "submit_review", toolArgs, func(_ string, _ bool) int {
		return http.StatusInternalServerError
	})

	stage := NewReviewStage(ReviewStageConfig{
		Runner:     srv.runner(),
		Messages:   []string{"review"},
		ModelChain: []string{"p/primary", "p/fallback"},
	})
	_, _, err := stage.Run(context.Background())
	if err == nil {
		t.Fatal("Run error = nil, want error")
	}
	if !strings.Contains(err.Error(), "reviewer precheck failed for all models") || !strings.Contains(err.Error(), "p/primary") || !strings.Contains(err.Error(), "p/fallback") {
		t.Fatalf("error = %v, want exhausted model list", err)
	}
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
