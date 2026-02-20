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
}

// ParseToolArgs parses the raw JSON args from the submit_review tool invocation
// into a ReviewResult. Returns a result with ParseErr set if parsing fails or
// the verdict is invalid. Fields are always populated so the caller can inspect them.
func ParseToolArgs(data json.RawMessage) *models.ReviewResult {
	result := &models.ReviewResult{Raw: string(data)}
	var args submitReviewArgs
	if err := json.Unmarshal(data, &args); err != nil {
		result.ParseErr = fmt.Errorf("parse tool args: %w", err)
		return result
	}
	result.ReviewerName = args.ReviewerName
	result.Summary = args.Summary
	result.Verdict = args.Verdict
	result.Findings = args.Findings

	if !validVerdicts[args.Verdict] {
		result.ParseErr = fmt.Errorf("invalid verdict %q: must be one of approve, request_changes, comment_only", args.Verdict)
	}
	return result
}

// Parse converts the raw agent response into a ReviewResult.
// It attempts a direct JSON unmarshal first; if that fails it tries to extract
// a JSON array by locating the first '[' and last ']' in the response, which
// handles cases where the model wraps output in markdown code fences or adds
// surrounding prose.
func Parse(raw string) *models.ReviewResult {
	result := &models.ReviewResult{Raw: raw}

	cleaned := stripCodeFence(strings.TrimSpace(raw))
	findings, err := unmarshalFindings(cleaned)
	if err != nil {
		extracted := extractJSONArray(cleaned)
		if extracted == "" {
			result.ParseErr = fmt.Errorf("parse agent response: %w", err)
			return result
		}
		findings, err = unmarshalFindings(extracted)
		if err != nil {
			result.ParseErr = fmt.Errorf("parse agent response (extracted): %w", err)
			return result
		}
	}

	result.Findings = findings
	return result
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

func unmarshalFindings(s string) ([]models.Finding, error) {
	var findings []models.Finding
	if err := json.Unmarshal([]byte(s), &findings); err != nil {
		return nil, err
	}
	return findings, nil
}

// extractJSONArray locates the outermost JSON array in s by finding the first '['
// and the last ']'. Returns empty string if not found.
func extractJSONArray(s string) string {
	start := strings.Index(s, "[")
	end := strings.LastIndex(s, "]")
	if start == -1 || end == -1 || end <= start {
		return ""
	}
	return s[start : end+1]
}
