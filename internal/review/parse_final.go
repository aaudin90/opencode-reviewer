package review

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aaudin90/opencode-reviewer/internal/shared/models"
)

type submitFinalReviewArgs struct {
	Summary  string                `json:"summary"`
	Verdict  string                `json:"verdict"`
	Findings []models.FinalFinding `json:"findings"`
}

var validFinalVerdicts = map[string]bool{
	"approve":         true,
	"request_changes": true,
	"comment_only":    true,
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
		if args.Verdict != "" && args.Findings != nil {
			result := &models.FinalReview{
				Summary:  args.Summary,
				Verdict:  args.Verdict,
				Findings: args.Findings,
			}
			if !validFinalVerdicts[args.Verdict] {
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

// IsBetterFinalResult reports whether candidate should replace current as the
// chosen FinalReview. A candidate without ParseErr wins over one with an error.
// When both are error-free, current (ToolArgs) is kept. When both have errors,
// candidate wins if it has more findings or a non-empty verdict where current has none.
func IsBetterFinalResult(candidate, current *models.FinalReview) bool {
	if candidate.ParseErr == nil && current.ParseErr != nil {
		return true
	}
	if candidate.ParseErr == nil {
		return false
	}
	if current.ParseErr == nil {
		return false
	}
	if len(candidate.Findings) > len(current.Findings) {
		return true
	}
	return candidate.Verdict != "" && current.Verdict == ""
}

// ParseFinalToolArgs parses the raw JSON args from the submit_final_review tool
// invocation into a FinalReview. Returns a result with ParseErr set if parsing
// fails or the verdict is invalid.
func ParseFinalToolArgs(data json.RawMessage) *models.FinalReview {
	result := &models.FinalReview{}
	var args submitFinalReviewArgs
	if err := json.Unmarshal(data, &args); err != nil {
		result.ParseErr = fmt.Errorf("parse final review tool args: %w", err)
		var raw map[string]any
		if mapErr := json.Unmarshal(data, &raw); mapErr == nil {
			result.Summary, _ = raw["summary"].(string)
			result.Verdict, _ = raw["verdict"].(string)
		}
		return result
	}
	result.Summary = args.Summary
	result.Verdict = args.Verdict
	result.Findings = args.Findings

	if !validFinalVerdicts[args.Verdict] {
		result.ParseErr = fmt.Errorf("invalid verdict %q: must be one of approve, request_changes, comment_only", args.Verdict)
	}
	return result
}
