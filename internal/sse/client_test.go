package sse

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// makeToolCallEvent generates a single SSE event in the real opencode format.
func makeToolCallEvent(sessionID, toolName, status string, input any) string {
	inputJSON, _ := json.Marshal(input)
	event := map[string]any{
		"type": "message.part.updated",
		"properties": map[string]any{
			"part": map[string]any{
				"sessionID": sessionID,
				"type":      "tool",
				"tool":      toolName,
				"state": map[string]any{
					"status": status,
					"input":  json.RawMessage(inputJSON),
				},
			},
		},
	}
	payload, _ := json.Marshal(event)
	return fmt.Sprintf("event: message.part.updated\ndata: %s\n\n", payload)
}

func TestParseStream_MatchingEvent(t *testing.T) {
	args := map[string]any{
		"summary":  "Looks good",
		"verdict":  "approve",
		"findings": []any{},
	}
	stream := makeToolCallEvent("sess-1", "submit_review", "completed", args)

	result, err := parseStream(strings.NewReader(stream), "sess-1", "submit_review", nil)
	if err != nil {
		t.Fatalf("parseStream: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if parsed["verdict"] != "approve" {
		t.Errorf("verdict = %v, want approve", parsed["verdict"])
	}
}

func TestParseStream_WrongSession(t *testing.T) {
	stream := makeToolCallEvent("other-session", "submit_review", "completed", map[string]any{"verdict": "approve"})
	_, err := parseStream(strings.NewReader(stream), "sess-1", "submit_review", nil)
	if err == nil {
		t.Fatal("expected error for wrong session, got nil")
	}
}

func TestParseStream_WrongToolName(t *testing.T) {
	stream := makeToolCallEvent("sess-1", "other_tool", "completed", map[string]any{"verdict": "approve"})
	_, err := parseStream(strings.NewReader(stream), "sess-1", "submit_review", nil)
	if err == nil {
		t.Fatal("expected error for wrong tool name, got nil")
	}
}

func TestParseStream_StatusNotCompleted(t *testing.T) {
	stream := makeToolCallEvent("sess-1", "submit_review", "running", map[string]any{"verdict": "approve"})
	_, err := parseStream(strings.NewReader(stream), "sess-1", "submit_review", nil)
	if err == nil {
		t.Fatal("expected error for non-completed status, got nil")
	}
}

func TestParseStream_PartialThenResult(t *testing.T) {
	// pending event should be ignored, completed event should be returned
	pending := makeToolCallEvent("sess-1", "submit_review", "pending", map[string]any{"summary": "incomplete"})
	completed := makeToolCallEvent("sess-1", "submit_review", "completed", map[string]any{
		"summary":  "Done",
		"verdict":  "request_changes",
		"findings": []any{},
	})
	stream := pending + completed

	got, err := parseStream(strings.NewReader(stream), "sess-1", "submit_review", nil)
	if err != nil {
		t.Fatalf("parseStream: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(got, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed["verdict"] != "request_changes" {
		t.Errorf("verdict = %v, want request_changes", parsed["verdict"])
	}
}

func TestParseStream_EmptyStream(t *testing.T) {
	_, err := parseStream(strings.NewReader(""), "sess-1", "submit_review", nil)
	if err == nil {
		t.Fatal("expected error for empty stream, got nil")
	}
}

func TestParseStream_NoEventLine(t *testing.T) {
	// Real opencode may not include "event:" lines — only "data:" lines.
	// parseStream should still work because we check type in the JSON payload.
	args := map[string]any{"verdict": "approve", "summary": "ok", "findings": []any{}}
	inputJSON, _ := json.Marshal(args)
	event := map[string]any{
		"type": "message.part.updated",
		"properties": map[string]any{
			"part": map[string]any{
				"sessionID": "sess-1",
				"type":      "tool",
				"tool":      "submit_review",
				"state": map[string]any{
					"status": "completed",
					"input":  json.RawMessage(inputJSON),
				},
			},
		},
	}
	payload, _ := json.Marshal(event)
	// No "event:" line — only "data:" and blank line
	stream := fmt.Sprintf("data: %s\n\n", payload)

	got, err := parseStream(strings.NewReader(stream), "sess-1", "submit_review", nil)
	if err != nil {
		t.Fatalf("parseStream without event line: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(got, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed["verdict"] != "approve" {
		t.Errorf("verdict = %v, want approve", parsed["verdict"])
	}
}

func TestWaitForToolResult_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// hang forever — context cancellation should unblock the client
		<-req.Context().Done()
	}))
	defer srv.Close()

	client := New(srv.Client(), srv.URL)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := client.WaitForToolResult(ctx, "sess-1", "submit_review", nil)
	if err == nil {
		t.Fatal("expected error when context is cancelled, got nil")
	}
}
