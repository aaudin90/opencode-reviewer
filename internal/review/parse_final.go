package review

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aaudin90/opencode-reviewer/internal/models"
)

type submitFinalReviewArgs struct {
	Summary  string                `json:"summary"`
	Verdict  string                `json:"verdict"`
	Findings []models.FinalFinding `json:"findings"`
}

// ParseFinal converts the raw JSON fallback response into a FinalReview.
// It attempts to unmarshal as a full submitFinalReviewArgs object. If the JSON
// is valid and contains at least a verdict and findings, the structured fields
// are populated and Raw is left empty. Otherwise Raw is set as a fallback.
func ParseFinal(raw string) *models.FinalReview {
	cleaned := strings.TrimSpace(raw)
	cleaned = stripCodeFence(cleaned)

	var args submitFinalReviewArgs
	if err := json.Unmarshal([]byte(cleaned), &args); err == nil {
		if args.Verdict != "" && len(args.Findings) > 0 {
			result := &models.FinalReview{
				Summary:  args.Summary,
				Verdict:  args.Verdict,
				Findings: args.Findings,
			}
			if !validVerdicts[args.Verdict] {
				result.ParseErr = fmt.Errorf("invalid verdict %q", args.Verdict)
			}
			return result
		}
	}

	return &models.FinalReview{
		Raw:      raw,
		ParseErr: fmt.Errorf("failed to parse JSON fallback response"),
	}
}

// ParseFinalToolArgs parses the raw JSON args from the submit_final_review tool
// invocation into a FinalReview. Returns a result with ParseErr set if parsing
// fails or the verdict is invalid.
func ParseFinalToolArgs(data json.RawMessage) *models.FinalReview {
	result := &models.FinalReview{}
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
