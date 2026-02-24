package review

import (
	"encoding/json"
	"fmt"

	"github.com/aaudin90/opencode-reviewer/internal/models"
)

type submitFinalReviewArgs struct {
	Summary  string                `json:"summary"`
	Verdict  string                `json:"verdict"`
	Findings []models.FinalFinding `json:"findings"`
}

// ParseFinalToolArgs parses the raw JSON args from the submit_final_review tool
// invocation into a FinalReview. Returns a result with ParseErr set if parsing
// fails or the verdict is invalid.
func ParseFinalToolArgs(data json.RawMessage) *models.FinalReview {
	result := &models.FinalReview{Raw: string(data)}
	var args submitFinalReviewArgs
	if err := json.Unmarshal(data, &args); err != nil {
		result.ParseErr = fmt.Errorf("parse final review tool args: %w", err)
		return result
	}
	result.Summary = args.Summary
	result.Verdict = args.Verdict
	result.Findings = args.Findings

	if !validVerdicts[args.Verdict] {
		result.ParseErr = fmt.Errorf("invalid verdict %q: must be one of approve, request_changes, comment_only", args.Verdict)
	}
	return result
}
