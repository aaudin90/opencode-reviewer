package runner

import "encoding/json"

// RunResult contains the result of a review run.
// If the agent called submit_review, ToolArgs is non-nil.
// If the agent did not call the tool after all retries, FallbackText is non-empty.
type RunResult struct {
	ToolArgs     json.RawMessage // non-nil: agent used submit_review tool
	FallbackText string          // non-empty: fallback to text response
}
