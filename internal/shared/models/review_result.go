package models

// ReviewResult holds the raw agent response and the parsed list of findings.
// ParseErr is non-nil when the response could not be parsed as JSON.
type ReviewResult struct {
	Raw          string
	Findings     []Finding
	ParseErr     error
	Summary      string // from submit_review tool args
	Verdict      string // "approve" | "request_changes" | "comment_only" | "skipped"
	ReviewerName string // short label inferred by agent from user prompt
	MessageRef   ReviewMessageRef
}
