package sse

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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

	result, err := parseStream(strings.NewReader(stream), "sess-1", "submit_review", nil, nil)
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

func TestParseStream_LongDataLine(t *testing.T) {
	longSummary := strings.Repeat("x", 1<<20+1)
	args := map[string]any{
		"summary":  longSummary,
		"verdict":  "approve",
		"findings": []any{},
	}
	stream := makeToolCallEvent("sess-1", "submit_review", "completed", args)

	result, err := parseStream(strings.NewReader(stream), "sess-1", "submit_review", nil, nil)
	if err != nil {
		t.Fatalf("parseStream: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if parsed["summary"] != longSummary {
		t.Fatalf("summary length = %d, want %d", len(parsed["summary"].(string)), len(longSummary))
	}
}

func TestParseStream_DataExceedsLimit(t *testing.T) {
	stream := "data: 12345678901\ndata: 1234567890\n\n"

	_, err := parseStreamWithLimit(strings.NewReader(stream), "sess-1", "submit_review", nil, nil, 20)
	if err == nil {
		t.Fatal("expected limit error, got nil")
	}
	if IsRetryable(err) {
		t.Fatalf("expected non-retryable limit error, got %v", err)
	}
	if !strings.Contains(err.Error(), "sse event exceeds 20 bytes limit") {
		t.Fatalf("error = %v, want SSE limit error", err)
	}
}

func TestSSEEventTooLargeErrorMessage(t *testing.T) {
	err := (&sseEventTooLargeError{limitBytes: maxSSEEventBytes}).Error()
	if err != "sse event exceeds 256 MiB limit" {
		t.Fatalf("error = %q, want 256 MiB limit message", err)
	}
}

func TestParseStream_EOFWithoutFinalBlankLine(t *testing.T) {
	args := map[string]any{
		"summary":  "Done",
		"verdict":  "approve",
		"findings": []any{},
	}
	stream := strings.TrimSuffix(makeToolCallEvent("sess-1", "submit_review", "completed", args), "\n\n")

	result, err := parseStream(strings.NewReader(stream), "sess-1", "submit_review", nil, nil)
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
	_, err := parseStream(strings.NewReader(stream), "sess-1", "submit_review", nil, nil)
	if err == nil {
		t.Fatal("expected error for wrong session, got nil")
	}
}

func TestParseStream_WrongToolName(t *testing.T) {
	stream := makeToolCallEvent("sess-1", "other_tool", "completed", map[string]any{"verdict": "approve"})
	_, err := parseStream(strings.NewReader(stream), "sess-1", "submit_review", nil, nil)
	if err == nil {
		t.Fatal("expected error for wrong tool name, got nil")
	}
}

func TestParseStream_StatusNotCompleted(t *testing.T) {
	stream := makeToolCallEvent("sess-1", "submit_review", "running", map[string]any{"verdict": "approve"})
	_, err := parseStream(strings.NewReader(stream), "sess-1", "submit_review", nil, nil)
	if err == nil {
		t.Fatal("expected error for non-completed status, got nil")
	}
}

func TestParseStream_StatusErrorIsRetryable(t *testing.T) {
	stream := makeToolCallEvent("sess-1", "submit_review", "error", map[string]any{"verdict": "approve"})
	_, err := parseStream(strings.NewReader(stream), "sess-1", "submit_review", nil, nil)
	if err == nil {
		t.Fatal("expected error for error status, got nil")
	}
	if !IsRetryable(err) {
		t.Fatalf("expected retryable error, got %v", err)
	}
}

func TestWaitForToolResult_Non200CodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	client := New(srv.Client(), srv.URL)
	_, err := client.WaitForToolResult(context.Background(), "sess-1", "submit_review", nil, nil)
	if err == nil {
		t.Fatal("expected code error, got nil")
	}
	if !IsCodeError(err) {
		t.Fatalf("IsCodeError = false for %v", err)
	}
	var codeErr *CodeError
	if !errors.As(err, &codeErr) {
		t.Fatalf("error type = %T, want *CodeError", err)
	}
	if codeErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", codeErr.StatusCode, http.StatusTooManyRequests)
	}
	if codeErr.Source != "GET /event" || !strings.Contains(codeErr.Snippet, "rate limited") {
		t.Fatalf("code error = %+v, want source and body snippet", codeErr)
	}
}

func TestParseStream_SessionErrorCodeError(t *testing.T) {
	stream := `event: session.error
data: {"type":"session.error","properties":{"error":{"statusCode":500,"message":"provider exploded"}}}

`
	_, err := parseStream(strings.NewReader(stream), "sess-1", "submit_review", nil, nil)
	if err == nil {
		t.Fatal("expected code error, got nil")
	}
	if !IsCodeError(err) {
		t.Fatalf("IsCodeError = false for %v", err)
	}
	var codeErr *CodeError
	if !errors.As(err, &codeErr) {
		t.Fatalf("error type = %T, want *CodeError", err)
	}
	if codeErr.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", codeErr.StatusCode, http.StatusInternalServerError)
	}
	if codeErr.Source != "session.error" || codeErr.Snippet != "provider exploded" {
		t.Fatalf("code error = %+v, want session.error message", codeErr)
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

	got, err := parseStream(strings.NewReader(stream), "sess-1", "submit_review", nil, nil)
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
	_, err := parseStream(strings.NewReader(""), "sess-1", "submit_review", nil, nil)
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

	got, err := parseStream(strings.NewReader(stream), "sess-1", "submit_review", nil, nil)
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

	_, err := client.WaitForToolResult(ctx, "sess-1", "submit_review", nil, nil)
	if err == nil {
		t.Fatal("expected error when context is cancelled, got nil")
	}
}

func TestParseStream_ChildSessionToolCallForwarded(t *testing.T) {
	// Parent session tool call (not the target tool)
	parentEvent := makeToolCallEvent("parent-sess", "Read", "completed", map[string]any{"path": "/foo"})
	// Child session tool call
	childEvent := makeToolCallEvent("child-sess", "Grep", "completed", map[string]any{"pattern": "TODO"})
	// Target tool in parent session — terminates stream
	targetEvent := makeToolCallEvent("parent-sess", "submit_review", "completed", map[string]any{
		"summary":  "Done",
		"verdict":  "approve",
		"findings": []any{},
	})
	stream := parentEvent + childEvent + targetEvent

	// Populate childSessions with the known child
	childSessions := &sync.Map{}
	childSessions.Store("child-sess", true)

	events := make(chan ToolCall, 10)
	result, err := parseStream(strings.NewReader(stream), "parent-sess", "submit_review", childSessions, events)
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

	// Collect forwarded events (parseStream does not close the channel)
	close(events)
	var calls []ToolCall
	for tc := range events {
		calls = append(calls, tc)
	}

	if len(calls) != 3 {
		t.Fatalf("got %d tool call events, want 3", len(calls))
	}
	if calls[0].Tool != "Read" || calls[0].SessionID != "parent-sess" {
		t.Errorf("calls[0] = %+v, want Read from parent-sess", calls[0])
	}
	if calls[1].Tool != "Grep" || calls[1].SessionID != "child-sess" {
		t.Errorf("calls[1] = %+v, want Grep from child-sess", calls[1])
	}
	if calls[2].Tool != "submit_review" || calls[2].SessionID != "parent-sess" {
		t.Errorf("calls[2] = %+v, want submit_review from parent-sess", calls[2])
	}
}

func TestExtractToolArgs_ChildSessionIgnored(t *testing.T) {
	// extractToolArgs must NOT match a child session even if tool name matches
	stream := makeToolCallEvent("child-sess", "submit_review", "completed", map[string]any{
		"verdict": "approve",
	})
	// Append the real parent session target tool
	stream += makeToolCallEvent("parent-sess", "submit_review", "completed", map[string]any{
		"summary":  "Real result",
		"verdict":  "request_changes",
		"findings": []any{},
	})

	// child-sess is a known child, but extractToolArgs still only matches parent
	childSessions := &sync.Map{}
	childSessions.Store("child-sess", true)

	result, err := parseStream(strings.NewReader(stream), "parent-sess", "submit_review", childSessions, nil)
	if err != nil {
		t.Fatalf("parseStream: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	// Must return the parent session result, not the child one
	if parsed["verdict"] != "request_changes" {
		t.Errorf("verdict = %v, want request_changes (parent result)", parsed["verdict"])
	}
}

func TestParseStream_UnknownSessionIgnored(t *testing.T) {
	// An unknown session (not in childSessions map) should NOT be added to
	// knownSessions and its tool calls should NOT be forwarded.
	unknownEvent := makeToolCallEvent("unknown-sess", "Grep", "completed", map[string]any{"pattern": "TODO"})
	parentEvent := makeToolCallEvent("parent-sess", "submit_review", "completed", map[string]any{
		"summary":  "Done",
		"verdict":  "approve",
		"findings": []any{},
	})
	stream := unknownEvent + parentEvent

	// childSessions is empty — no children registered
	childSessions := &sync.Map{}

	events := make(chan ToolCall, 10)
	result, err := parseStream(strings.NewReader(stream), "parent-sess", "submit_review", childSessions, events)
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

	// Only the parent submit_review should be forwarded, NOT the unknown session's Grep
	close(events)
	var calls []ToolCall
	for tc := range events {
		calls = append(calls, tc)
	}

	if len(calls) != 1 {
		t.Fatalf("got %d tool call events, want 1 (only parent)", len(calls))
	}
	if calls[0].Tool != "submit_review" || calls[0].SessionID != "parent-sess" {
		t.Errorf("calls[0] = %+v, want submit_review from parent-sess", calls[0])
	}
}
