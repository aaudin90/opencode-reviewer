package commentwarrior

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

	"github.com/aaudin90/opencode-reviewer/internal/shared/config"
	"github.com/aaudin90/opencode-reviewer/internal/shared/runner"
)

func TestValidateDecisionReplyRequiresBody(t *testing.T) {
	err := ValidateDecision(Decision{Action: ActionReply, Confidence: "high"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateDecisionResolveRequiresBody(t *testing.T) {
	err := ValidateDecision(Decision{Action: ActionResolve, Confidence: "high", Reason: "fixed"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateDecisionUnresolveRequiresBody(t *testing.T) {
	err := ValidateDecision(Decision{Action: ActionUnresolve, Confidence: "high", Reason: "still valid"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateDecisionRejectsWhitespaceBody(t *testing.T) {
	err := ValidateDecision(Decision{Action: ActionResolve, Body: " \t\n", Confidence: "high", Reason: "fixed"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateDecisionRejectsUnsafeFlags(t *testing.T) {
	err := ValidateDecision(Decision{Action: ActionNoop, Confidence: "low", WouldModifyCode: true})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateDecisionAllowsNoopWithEmptyBody(t *testing.T) {
	err := ValidateDecision(Decision{Action: ActionNoop, Confidence: "low", Reason: "no action needed"})
	if err != nil {
		t.Fatalf("ValidateDecision: %v", err)
	}
}

func TestValidateDecisionAllowsResolveAndUnresolveBody(t *testing.T) {
	tests := []Decision{
		{Action: ActionResolve, Body: "fixed", Confidence: "high", Reason: "fixed"},
		{Action: ActionUnresolve, Body: "still valid", Confidence: "high", Reason: "still valid"},
	}

	for _, tt := range tests {
		if err := ValidateDecision(tt); err != nil {
			t.Fatalf("ValidateDecision(%s): %v", tt.Action, err)
		}
	}
}

func TestRunDecisionRetriesEmptyResolveBody(t *testing.T) {
	const sessionID = "sess-comment-warrior-schema"

	invalidArgs := map[string]any{
		"action":            "resolve",
		"body":              "",
		"confidence":        "high",
		"would_modify_code": false,
		"needs_human":       false,
		"reason":            "fixed",
	}
	validArgs := map[string]any{
		"action":            "resolve",
		"body":              "Resolved after the fix was applied.",
		"confidence":        "high",
		"would_modify_code": false,
		"needs_human":       false,
		"reason":            "fixed",
	}

	invalidSSE := makeCommentWarriorToolCallSSE(sessionID, invalidArgs)
	validSSE := makeCommentWarriorToolCallSSE(sessionID, validArgs)

	var sseCallCount atomic.Int32
	var promptsMu sync.Mutex
	var prompts []string

	mux := http.NewServeMux()
	mux.HandleFunc("GET /event", func(w http.ResponseWriter, _ *http.Request) {
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
		_, _ = fmt.Fprintf(w, `{"id":%q}`, sessionID)
	})
	mux.HandleFunc("POST /session/{id}/message", func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if len(body.Parts) > 0 {
			promptsMu.Lock()
			prompts = append(prompts, body.Parts[0].Text)
			promptsMu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"info":{"id":"msg-comment-warrior-schema"},"parts":[{"type":"text","text":"ok"}]}`)
	})
	mux.HandleFunc("DELETE /session/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /session/{id}/children", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[]`)
	})
	mux.HandleFunc("GET /session/{id}/message", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"info":{"role":"assistant","cost":0.01}}]`)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	r := runner.New(config.OpenCodeConfig{
		Endpoint:     srv.URL,
		Model:        "test/model",
		StageTimeout: 30,
	}, t.TempDir(), nil)

	decision, err := runDecision(context.Background(), r, "handle this discussion", "1")
	if err != nil {
		t.Fatalf("runDecision: %v", err)
	}
	if decision.Action != ActionResolve {
		t.Fatalf("action = %q, want %q", decision.Action, ActionResolve)
	}
	if decision.Body != "Resolved after the fix was applied." {
		t.Fatalf("body = %q", decision.Body)
	}

	promptsMu.Lock()
	gotPrompts := append([]string(nil), prompts...)
	promptsMu.Unlock()

	if len(gotPrompts) < 2 {
		t.Fatalf("expected original prompt and schema retry prompt, got %d prompts", len(gotPrompts))
	}
	if !strings.Contains(gotPrompts[1], "did not match the required schema") {
		t.Fatalf("retry prompt = %q, want schema retry prompt", gotPrompts[1])
	}
	if !strings.Contains(gotPrompts[1], "required non-empty text for reply|resolve|unresolve") {
		t.Fatalf("retry prompt does not include decision schema hint: %q", gotPrompts[1])
	}
}

func makeCommentWarriorToolCallSSE(sessionID string, args any) string {
	argsJSON, _ := json.Marshal(args)
	payload := fmt.Sprintf(`{"type":"message.part.updated","properties":{"part":{"sessionID":%q,"type":"tool","tool":"submit_comment_warrior_decision","state":{"status":"completed","input":%s}}}}`,
		sessionID, argsJSON)
	return "event: message.part.updated\ndata: " + payload + "\n\n"
}
