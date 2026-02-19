package review

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aaudin90/opencode-reviewer/internal/models"
)

// Parse converts the raw agent response into a ReviewResult.
// It attempts a direct JSON unmarshal first; if that fails it tries to extract
// a JSON array by locating the first '[' and last ']' in the response, which
// handles cases where the model wraps output in markdown code fences or adds
// surrounding prose.
func Parse(raw string) *models.ReviewResult {
	result := &models.ReviewResult{Raw: raw}

	findings, err := unmarshalFindings(strings.TrimSpace(raw))
	if err != nil {
		extracted := extractJSONArray(raw)
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
