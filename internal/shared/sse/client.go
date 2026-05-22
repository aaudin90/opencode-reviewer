package sse

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Client reads SSE events from an opencode serve endpoint.
type Client struct {
	httpClient *http.Client
	url        string
}

type retryableError struct {
	err error
}

func (e *retryableError) Error() string {
	return e.err.Error()
}

func (e *retryableError) Unwrap() error {
	return e.err
}

func markRetryable(err error) error {
	if err == nil {
		return nil
	}
	return &retryableError{err: err}
}

func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var retryable *retryableError
	return errors.As(err, &retryable)
}

// New creates a new SSE client that connects to baseURL/event.
func New(httpClient *http.Client, baseURL string) *Client {
	return &Client{
		httpClient: httpClient,
		url:        strings.TrimRight(baseURL, "/") + "/event",
	}
}

// WaitForToolResult connects to the SSE /event endpoint and waits for a
// status="completed" event for the given toolName within the given sessionID.
// Returns the raw JSON input of the tool call.
// events receives all completed tool call events as they arrive; it is closed
// before WaitForToolResult returns. Pass nil to disable event forwarding.
func (c *Client) WaitForToolResult(ctx context.Context, sessionID, toolName string, childSessions *sync.Map, events chan<- ToolCall) (json.RawMessage, error) {
	if events != nil {
		defer close(events)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return nil, fmt.Errorf("build sse request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.httpClient.Do(req) // #nosec G704 -- URL from trusted config
	if err != nil {
		if ctx.Err() == nil {
			return nil, markRetryable(fmt.Errorf("connect to sse: %w", err))
		}
		return nil, fmt.Errorf("connect to sse: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if isHTTPCodeError(resp.StatusCode) {
			return nil, newCodeError(resp.StatusCode, "GET /event", string(body))
		}
		return nil, fmt.Errorf("unexpected sse status %d: %s", resp.StatusCode, body)
	}

	return parseStream(resp.Body, sessionID, toolName, childSessions, events)
}

// parseStream reads SSE events from r and returns the input of the first
// tool call event with status="completed" matching sessionID and toolName.
// It does NOT close the events channel; the caller is responsible for closing it.
// This is a pure function that can be tested with strings.NewReader.
func parseStream(
	r io.Reader,
	sessionID, toolName string,
	childSessions *sync.Map,
	events chan<- ToolCall,
) (json.RawMessage, error) {
	knownSessions := map[string]bool{sessionID: true}

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 1<<20), 1<<20) // 1 MB buffer for large findings

	var dataLines []string

	startTime := time.Now()
	var eventCount int
	lastHeartbeat := time.Now()

	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "event:"):
			// ignore event: lines — we check type field in JSON instead
		case strings.HasPrefix(line, "data:"):
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		case line == "":
			if len(dataLines) > 0 {
				raw := []byte(strings.Join(dataLines, ""))
				if err := extractSessionCodeError(raw); err != nil {
					return nil, err
				}
				var event toolCallEvent
				if err := json.Unmarshal(raw, &event); err == nil {
					if sid := event.Properties.Part.SessionID; sid != "" && !knownSessions[sid] {
						if childSessions != nil {
							if _, ok := childSessions.Load(sid); ok {
								knownSessions[sid] = true
								slog.Info("discovered child session in SSE stream",
									"child_session_id", sid,
									"parent_session_id", sessionID)
							}
						}
					}
					trySendToolCall(&event, knownSessions, events)
					if err := extractToolError(&event, sessionID, toolName); err != nil {
						return nil, err
					}
					if args, ok := extractToolArgs(&event, sessionID, toolName); ok {
						return args, nil
					}
				}
				eventCount++
				if time.Since(lastHeartbeat) >= 30*time.Second {
					lastHeartbeat = time.Now()
					slog.Info("sse heartbeat", "session", sessionID, "tool", toolName, "events_processed", eventCount, "elapsed", time.Since(startTime).String())
				}
			}
			dataLines = dataLines[:0]
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, markRetryable(fmt.Errorf("sse stream read error: %w", err))
	}

	return nil, fmt.Errorf("sse stream ended without %q tool result for session %q", toolName, sessionID)
}

// trySendToolCall sends a completed tool call event to the channel (non-blocking).
// Only events matching a known session ID are forwarded.
func trySendToolCall(event *toolCallEvent, knownSessions map[string]bool, events chan<- ToolCall) {
	if events == nil {
		return
	}
	part := event.Properties.Part
	if part.Type == "tool" && part.State.Status == "completed" && knownSessions[part.SessionID] {
		select {
		case events <- ToolCall{Tool: part.Tool, SessionID: part.SessionID, Input: part.State.Input}:
		default:
		}
	}
}

// extractToolArgs returns the tool input if the event matches the expected
// sessionID, toolName, and status="completed".
func extractToolArgs(event *toolCallEvent, sessionID, toolName string) (json.RawMessage, bool) {
	part := event.Properties.Part
	if part.Type != "tool" {
		return nil, false
	}

	if part.Tool != toolName {
		return nil, false
	}

	if part.SessionID != sessionID {
		return nil, false
	}

	if part.State.Status != "completed" {
		return nil, false
	}

	return part.State.Input, true
}

func extractToolError(event *toolCallEvent, sessionID, toolName string) error {
	part := event.Properties.Part
	if part.Type != "tool" || part.Tool != toolName || part.SessionID != sessionID {
		return nil
	}
	if part.State.Status != "error" {
		return nil
	}
	return markRetryable(fmt.Errorf("tool %q entered error state in session %q", toolName, sessionID))
}
