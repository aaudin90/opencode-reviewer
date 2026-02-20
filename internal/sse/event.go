package sse

import "encoding/json"

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
