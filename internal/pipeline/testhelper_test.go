package pipeline

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// makeToolCallSSEGeneric produces a valid SSE payload for a tool call.
func makeToolCallSSEGeneric(sessionID, toolName string, args any) string {
	argsJSON, _ := json.Marshal(args)
	payload := fmt.Sprintf(
		`{"type":"message.part.updated","properties":{"part":{"sessionID":%q,"type":"tool","tool":%q,"state":{"status":"completed","input":%s}}}}`,
		sessionID, toolName, argsJSON,
	)
	return "event: message.part.updated\ndata: " + payload + "\n\n"
}

// newPipelineTestServer creates an httptest.Server that fakes the opencode API.
// It handles health, session creation (with atomic counter), messages, SSE events, children, and deletion.
// The SSE emits a tool call only for the review session (not the precheck session).
// sessionCounter tracks how many sessions were created. The tool call is emitted for sessions created after precheck.
func newPipelineTestServer(t *testing.T, toolName string, toolArgs any) *httptest.Server {
	t.Helper()

	var sessionCounter atomic.Int32
	var latestReviewSessionID atomic.Value

	mux := http.NewServeMux()

	mux.HandleFunc("GET /global/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("POST /session", func(w http.ResponseWriter, _ *http.Request) {
		n := sessionCounter.Add(1)
		id := fmt.Sprintf("sess-%d", n)
		// Sessions after the first one (precheck) are review sessions
		if n > 1 {
			latestReviewSessionID.Store(id)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"id": id})
	})

	mux.HandleFunc("POST /session/{id}/message", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"info":  map[string]any{"id": "msg-1", "tokens": map[string]any{"input": 0, "output": 0}},
			"parts": []map[string]any{{"type": "text", "text": "ok"}},
		})
	})

	mux.HandleFunc("GET /event", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "no flusher", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)

		// Wait briefly for the review session to be created
		// The SSE client will reconnect if needed
		sid, _ := latestReviewSessionID.Load().(string)
		if sid != "" {
			fmt.Fprint(w, makeToolCallSSEGeneric(sid, toolName, toolArgs))
		}
		flusher.Flush()
	})

	mux.HandleFunc("GET /session/{id}/message", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"info": map[string]any{"role": "assistant", "cost": 0.05, "tokens": map[string]any{"input": 100, "output": 50, "reasoning": 10, "cache": map[string]any{"read": 5, "write": 3}}}},
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
	t.Cleanup(srv.Close)
	return srv
}
