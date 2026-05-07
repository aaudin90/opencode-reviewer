package commentwarrior

import (
	"encoding/json"
	"fmt"
	"strings"
)

type Action string

const (
	ActionReply     Action = "reply"
	ActionResolve   Action = "resolve"
	ActionUnresolve Action = "unresolve"
	ActionNoop      Action = "noop"
)

type Decision struct {
	Action          Action `json:"action"`
	Body            string `json:"body"`
	Confidence      string `json:"confidence"`
	WouldModifyCode bool   `json:"would_modify_code"`
	NeedsHuman      bool   `json:"needs_human"`
	Reason          string `json:"reason"`
}

func ParseDecision(data json.RawMessage) (*Decision, error) {
	var d Decision
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("parse decision: %w", err)
	}
	if err := ValidateDecision(d); err != nil {
		return nil, err
	}
	return &d, nil
}

func ValidateDecision(d Decision) error {
	switch d.Action {
	case ActionReply, ActionResolve, ActionUnresolve, ActionNoop:
	default:
		return fmt.Errorf("unknown action %q", d.Action)
	}
	switch d.Confidence {
	case "high", "medium", "low":
	default:
		return fmt.Errorf("unknown confidence %q", d.Confidence)
	}
	if d.WouldModifyCode {
		return fmt.Errorf("decision would modify code")
	}
	if d.NeedsHuman && d.Action != ActionNoop {
		return fmt.Errorf("needs_human decisions must use noop")
	}
	if d.Action != ActionNoop && strings.TrimSpace(d.Body) == "" {
		return fmt.Errorf("%s action requires body", d.Action)
	}
	return nil
}

const DecisionSchemaHint = `{
  "action": "reply|resolve|unresolve|noop",
  "body": "required non-empty text for reply|resolve|unresolve; empty only for noop",
  "confidence": "high|medium|low",
  "would_modify_code": false,
  "needs_human": false,
  "reason": "short reason"
}`
