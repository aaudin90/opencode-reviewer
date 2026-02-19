package models

// ReviewResult holds the raw agent response and the parsed list of findings.
// ParseErr is non-nil when the response could not be parsed as JSON.
type ReviewResult struct {
	Raw      string
	Findings []Finding
	ParseErr error
}
