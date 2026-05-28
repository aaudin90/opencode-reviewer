package sse

import (
	"bufio"
	"bytes"
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

const maxSSEEventBytes = 256 << 20

type sseEventTooLargeError struct {
	limitBytes int
}

func (e *sseEventTooLargeError) Error() string {
	return fmt.Sprintf("sse event exceeds %s limit", formatByteLimit(e.limitBytes))
}

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
	return parseStreamWithLimit(r, sessionID, toolName, childSessions, events, maxSSEEventBytes)
}

func parseStreamWithLimit(
	r io.Reader,
	sessionID, toolName string,
	childSessions *sync.Map,
	events chan<- ToolCall,
	maxEventBytes int,
) (json.RawMessage, error) {
	knownSessions := map[string]bool{sessionID: true}

	reader := bufio.NewReader(r)

	var data bytes.Buffer
	var hasData bool

	startTime := time.Now()
	var eventCount int
	lastHeartbeat := time.Now()

	for {
		line, readErr := readSSELine(reader, maxEventBytes)
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			var tooLarge *sseEventTooLargeError
			if errors.As(readErr, &tooLarge) {
				return nil, readErr
			}
			return nil, markRetryable(fmt.Errorf("sse stream read error: %w", readErr))
		}
		line = bytes.TrimSuffix(line, []byte("\n"))
		line = bytes.TrimSuffix(line, []byte("\r"))

		switch {
		case bytes.HasPrefix(line, []byte("event:")):
			// ignore event: lines — we check type field in JSON instead
		case bytes.HasPrefix(line, []byte("data:")):
			dataLine := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
			if data.Len()+len(dataLine) > maxEventBytes {
				return nil, &sseEventTooLargeError{limitBytes: maxEventBytes}
			}
			if _, err := data.Write(dataLine); err != nil {
				return nil, err
			}
			hasData = true
		case len(line) == 0:
			args, handled, err := handleSSEEvent(hasData, data.Bytes(), knownSessions, sessionID, toolName, childSessions, events)
			if err != nil {
				return nil, err
			}
			if args != nil {
				return args, nil
			}
			if handled {
				eventCount++
				lastHeartbeat = maybeLogHeartbeat(lastHeartbeat, startTime, sessionID, toolName, eventCount)
			}
			data.Reset()
			hasData = false
		}

		if errors.Is(readErr, io.EOF) {
			args, handled, err := handleSSEEvent(hasData, data.Bytes(), knownSessions, sessionID, toolName, childSessions, events)
			if err != nil {
				return nil, err
			}
			if args != nil {
				return args, nil
			}
			if handled {
				eventCount++
				maybeLogHeartbeat(lastHeartbeat, startTime, sessionID, toolName, eventCount)
			}
			break
		}
	}

	return nil, fmt.Errorf("sse stream ended without %q tool result for session %q", toolName, sessionID)
}

func readSSELine(reader *bufio.Reader, maxBytes int) ([]byte, error) {
	var line bytes.Buffer
	for {
		fragment, err := reader.ReadSlice('\n')
		if line.Len()+len(fragment) > maxBytes {
			return nil, &sseEventTooLargeError{limitBytes: maxBytes}
		}
		if _, writeErr := line.Write(fragment); writeErr != nil {
			return nil, writeErr
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		return line.Bytes(), err
	}
}

func formatByteLimit(limitBytes int) string {
	if limitBytes%(1<<20) == 0 {
		return fmt.Sprintf("%d MiB", limitBytes>>20)
	}
	return fmt.Sprintf("%d bytes", limitBytes)
}

func handleSSEEvent(
	hasData bool,
	raw []byte,
	knownSessions map[string]bool,
	sessionID, toolName string,
	childSessions *sync.Map,
	events chan<- ToolCall,
) (json.RawMessage, bool, error) {
	if !hasData {
		return nil, false, nil
	}

	if err := extractSessionCodeError(raw); err != nil {
		return nil, true, err
	}
	var event toolCallEvent
	if err := json.Unmarshal(raw, &event); err != nil {
		return nil, true, nil
	}
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
		return nil, true, err
	}
	if args, ok := extractToolArgs(&event, sessionID, toolName); ok {
		return args, true, nil
	}
	return nil, true, nil
}

func maybeLogHeartbeat(lastHeartbeat, startTime time.Time, sessionID, toolName string, eventCount int) time.Time {
	if time.Since(lastHeartbeat) < 30*time.Second {
		return lastHeartbeat
	}
	lastHeartbeat = time.Now()
	slog.Info("sse heartbeat", "session", sessionID, "tool", toolName, "events_processed", eventCount, "elapsed", time.Since(startTime).String())
	return lastHeartbeat
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
