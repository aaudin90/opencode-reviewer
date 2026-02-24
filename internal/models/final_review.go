package models

// FinalReview holds the result produced by the finalizer agent (Phase 2).
// It contains the deduplicated and merged findings from all Phase 1 reviewers.
type FinalReview struct {
	Raw      string
	Summary  string
	Verdict  string
	Findings []FinalFinding
	ParseErr error
}
