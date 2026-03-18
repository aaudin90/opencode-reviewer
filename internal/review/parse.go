package review

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aaudin90/opencode-reviewer/internal/models"
)

type submitReviewArgs struct {
	ReviewerName string           `json:"reviewer_name"`
	Summary      string           `json:"summary"`
	Verdict      string           `json:"verdict"`
	Findings     []models.Finding `json:"findings"`
}

var validVerdicts = map[string]bool{
	"approve":         true,
	"request_changes": true,
	"comment_only":    true,
	"skipped":         true,
}

// ParseToolArgs parses the raw JSON args from the submit_review tool invocation
// into a ReviewResult. Returns a result with ParseErr set if parsing fails or
// the verdict is invalid. Fields are always populated so the caller can inspect them.
func ParseToolArgs(data json.RawMessage) *models.ReviewResult {
	result := &models.ReviewResult{}
	var args submitReviewArgs
	if err := json.Unmarshal(data, &args); err != nil {
		result.ParseErr = fmt.Errorf("parse tool args: %w", err)
		var raw map[string]any
		if mapErr := json.Unmarshal(data, &raw); mapErr == nil {
			result.ReviewerName, _ = raw["reviewer_name"].(string)
			result.Summary, _ = raw["summary"].(string)
			result.Verdict, _ = raw["verdict"].(string)
		}
		return result
	}
	result.ReviewerName = args.ReviewerName
	result.Summary = args.Summary
	result.Verdict = args.Verdict
	result.Findings = args.Findings

	if !validVerdicts[args.Verdict] {
		result.ParseErr = fmt.Errorf("invalid verdict %q: must be one of approve, request_changes, comment_only, skipped", args.Verdict)
	}
	return result
}

// Parse converts the raw JSON fallback response into a ReviewResult.
// It attempts to unmarshal the response as a full submitReviewArgs object
// (the same schema as the submit_review tool). If the JSON is valid and
// contains at least a verdict and findings, the structured fields are
// populated and Raw is left empty. Otherwise Raw is set as a fallback.
func Parse(raw string) *models.ReviewResult {
	cleaned := strings.TrimSpace(raw)
	cleaned = stripCodeFence(cleaned)

	var args submitReviewArgs
	if err := json.Unmarshal([]byte(cleaned), &args); err == nil {
		if args.Verdict != "" && len(args.Findings) > 0 {
			result := &models.ReviewResult{
				ReviewerName: args.ReviewerName,
				Summary:      args.Summary,
				Verdict:      args.Verdict,
				Findings:     args.Findings,
			}
			if !validVerdicts[args.Verdict] {
				result.ParseErr = fmt.Errorf("invalid verdict %q", args.Verdict)
			}
			return result
		}
	}

	return &models.ReviewResult{
		Raw:      raw,
		ParseErr: fmt.Errorf("failed to parse JSON fallback response"),
	}
}

// IsBetterResult reports whether candidate should replace current as the
// chosen ReviewResult. A candidate without ParseErr wins over one with an error.
// When both are error-free, current (ToolArgs) is kept. When both have errors,
// candidate wins if it has more findings or a non-empty verdict where current has none.
func IsBetterResult(candidate, current *models.ReviewResult) bool {
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

// stripCodeFence removes a leading ```[lang] / ``` fence and the closing ```
// from s. Returns s unchanged if no fence is detected.
func stripCodeFence(s string) string {
	if !strings.HasPrefix(s, "```") {
		return s
	}
	newline := strings.Index(s, "\n")
	if newline == -1 {
		return s
	}
	s = strings.TrimSuffix(s[newline+1:], "```")
	return strings.TrimRight(s, " \t\n\r")
}
