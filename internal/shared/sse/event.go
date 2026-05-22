package sse

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// ToolCall contains data about a completed tool invocation from the SSE stream.
type ToolCall struct {
	Tool      string
	SessionID string
	Input     json.RawMessage
}

type toolCallEvent struct {
	Type       string        `json:"type"`
	Properties toolCallProps `json:"properties"`
}

type toolCallProps struct {
	Part toolCallPart `json:"part"`
}

type toolCallPart struct {
	SessionID string        `json:"sessionID"`
	Type      string        `json:"type"` // "tool"
	Tool      string        `json:"tool"` // "submit_review"
	State     toolCallState `json:"state"`
}

type toolCallState struct {
	Status string          `json:"status"` // "pending"|"running"|"completed"|"error"
	Input  json.RawMessage `json:"input"`
}

func extractSessionCodeError(raw []byte) error {
	var event struct {
		Type       string         `json:"type"`
		Properties map[string]any `json:"properties"`
	}
	if err := json.Unmarshal(raw, &event); err != nil {
		return nil
	}
	if event.Type != "session.error" {
		return nil
	}

	status, ok := sessionErrorStatus(event.Properties)
	if !ok || !isHTTPCodeError(status) {
		return nil
	}
	return newCodeError(status, "session.error", sessionErrorSnippet(event.Properties))
}

func sessionErrorStatus(properties map[string]any) (int, bool) {
	if status, ok := numberValue(properties["statusCode"]); ok {
		return status, true
	}
	if nested, ok := properties["error"].(map[string]any); ok {
		return numberValue(nested["statusCode"])
	}
	return 0, false
}

func sessionErrorSnippet(properties map[string]any) string {
	if nested, ok := properties["error"].(map[string]any); ok {
		for _, key := range []string{"message", "body", "details"} {
			if value, ok := nested[key].(string); ok && value != "" {
				return value
			}
		}
	}
	for _, key := range []string{"message", "body", "details"} {
		if value, ok := properties[key].(string); ok && value != "" {
			return value
		}
	}
	data, err := json.Marshal(properties)
	if err != nil {
		return fmt.Sprint(properties)
	}
	return string(data)
}

func numberValue(value any) (int, bool) {
	switch v := value.(type) {
	case float64:
		return int(v), true
	case int:
		return v, true
	case json.Number:
		i, err := v.Int64()
		return int(i), err == nil
	case string:
		i, err := strconv.Atoi(v)
		return i, err == nil
	default:
		return 0, false
	}
}
