package review

import (
	"encoding/json"
	"testing"
)

func TestParse_ValidFullJSON(t *testing.T) {
	raw := `{
		"reviewer_name": "security",
		"summary": "Found issues",
		"verdict": "request_changes",
		"findings": [
			{
				"file": "main.go",
				"start_line": 10,
				"end_line": 12,
				"existing_code": "x := 1",
				"confidence": "high",
				"issue_content": "unused variable",
				"recommendation": "remove it"
			}
		]
	}`

	result := Parse(raw)

	if result.ParseErr != nil {
		t.Fatalf("ParseErr = %v, want nil", result.ParseErr)
	}
	if result.Raw != "" {
		t.Errorf("Raw = %q, want empty for successful parse", result.Raw)
	}
	if result.ReviewerName != "security" {
		t.Errorf("ReviewerName = %q, want %q", result.ReviewerName, "security")
	}
	if result.Summary != "Found issues" {
		t.Errorf("Summary = %q, want %q", result.Summary, "Found issues")
	}
	if result.Verdict != "request_changes" {
		t.Errorf("Verdict = %q, want %q", result.Verdict, "request_changes")
	}
	if len(result.Findings) != 1 {
		t.Fatalf("len(Findings) = %d, want 1", len(result.Findings))
	}
	if result.Findings[0].File != "main.go" {
		t.Errorf("Findings[0].File = %q, want %q", result.Findings[0].File, "main.go")
	}
}

func TestParse_ValidJSONInCodeFence(t *testing.T) {
	raw := "```json\n" + `{"reviewer_name":"r","summary":"s","verdict":"approve","findings":[{"file":"a.go","start_line":1,"end_line":1,"existing_code":"x","confidence":"high","issue_content":"bad","recommendation":"fix"}]}` + "\n```"

	result := Parse(raw)

	if result.ParseErr != nil {
		t.Fatalf("ParseErr = %v, want nil", result.ParseErr)
	}
	if result.Raw != "" {
		t.Errorf("Raw = %q, want empty", result.Raw)
	}
	if result.Verdict != "approve" {
		t.Errorf("Verdict = %q, want %q", result.Verdict, "approve")
	}
	if len(result.Findings) != 1 {
		t.Errorf("len(Findings) = %d, want 1", len(result.Findings))
	}
}

func TestParse_InvalidVerdict(t *testing.T) {
	raw := `{"summary":"test","verdict":"reject","findings":[{"file":"a.go","start_line":1,"end_line":1,"existing_code":"x","confidence":"high","issue_content":"bad","recommendation":"fix"}]}`

	result := Parse(raw)

	if result.ParseErr == nil {
		t.Fatal("ParseErr = nil, want non-nil for invalid verdict")
	}
	if result.Raw != "" {
		t.Errorf("Raw = %q, want empty (fields are still populated)", result.Raw)
	}
	if result.Verdict != "reject" {
		t.Errorf("Verdict = %q, want %q", result.Verdict, "reject")
	}
	if len(result.Findings) != 1 {
		t.Errorf("len(Findings) = %d, want 1", len(result.Findings))
	}
}

func TestParse_InvalidJSON(t *testing.T) {
	raw := "not json at all"

	result := Parse(raw)

	if result.ParseErr == nil {
		t.Error("ParseErr = nil, want non-nil for invalid input")
	}
	if result.Raw != raw {
		t.Error("Raw not preserved on parse failure")
	}
	if result.Findings != nil {
		t.Errorf("Findings = %v, want nil on parse failure", result.Findings)
	}
}

func TestParse_EmptyString(t *testing.T) {
	result := Parse("")

	if result.ParseErr == nil {
		t.Error("ParseErr = nil, want non-nil for empty input")
	}
	if result.Raw != "" {
		t.Errorf("Raw = %q, want empty", result.Raw)
	}
}

func TestParse_JSONWithoutVerdictAndFindings(t *testing.T) {
	raw := `{"summary": "some summary"}`

	result := Parse(raw)

	if result.ParseErr == nil {
		t.Error("ParseErr = nil, want non-nil when verdict and findings are missing")
	}
	if result.Raw != raw {
		t.Errorf("Raw = %q, want %q", result.Raw, raw)
	}
}

func TestParse_JSONWithEmptyFindings(t *testing.T) {
	raw := `{"summary":"s","verdict":"approve","findings":[]}`

	result := Parse(raw)

	if result.ParseErr == nil {
		t.Error("ParseErr = nil, want non-nil when findings are empty")
	}
	if result.Raw != raw {
		t.Errorf("Raw = %q, want %q", result.Raw, raw)
	}
}

func TestStripCodeFence_WithLanguage(t *testing.T) {
	input := "```json\n[{\"key\":\"val\"}]\n```"
	want := "[{\"key\":\"val\"}]"

	got := stripCodeFence(input)
	if got != want {
		t.Errorf("stripCodeFence = %q, want %q", got, want)
	}
}

func TestStripCodeFence_WithoutLanguage(t *testing.T) {
	input := "```\n[{\"key\":\"val\"}]\n```"
	want := "[{\"key\":\"val\"}]"

	got := stripCodeFence(input)
	if got != want {
		t.Errorf("stripCodeFence = %q, want %q", got, want)
	}
}

func TestStripCodeFence_NoFence(t *testing.T) {
	input := "[{\"key\":\"val\"}]"

	got := stripCodeFence(input)
	if got != input {
		t.Errorf("stripCodeFence = %q, want %q (unchanged)", got, input)
	}
}

func TestParseToolArgs_Valid(t *testing.T) {
	input := json.RawMessage(`{
		"summary": "Overall looks good",
		"verdict": "request_changes",
		"findings": [
			{
				"file": "main.go",
				"start_line": 10,
				"end_line": 12,
				"existing_code": "x := 1",
				"confidence": "high",
				"issue_content": "unused variable",
				"recommendation": "remove it"
			}
		]
	}`)

	result := ParseToolArgs(input)

	if result.ParseErr != nil {
		t.Fatalf("ParseErr = %v, want nil", result.ParseErr)
	}
	if result.Summary != "Overall looks good" {
		t.Errorf("Summary = %q, want %q", result.Summary, "Overall looks good")
	}
	if result.Verdict != "request_changes" {
		t.Errorf("Verdict = %q, want request_changes", result.Verdict)
	}
	if len(result.Findings) != 1 {
		t.Fatalf("len(Findings) = %d, want 1", len(result.Findings))
	}
	f := result.Findings[0]
	if f.File != "main.go" {
		t.Errorf("File = %q, want main.go", f.File)
	}
	if f.StartLine != 10 {
		t.Errorf("StartLine = %d, want 10", f.StartLine)
	}
}

func TestParseToolArgs_EmptyFindings(t *testing.T) {
	input := json.RawMessage(`{"summary": "All good", "verdict": "approve", "findings": []}`)

	result := ParseToolArgs(input)

	if result.ParseErr != nil {
		t.Fatalf("ParseErr = %v, want nil", result.ParseErr)
	}
	if len(result.Findings) != 0 {
		t.Errorf("len(Findings) = %d, want 0", len(result.Findings))
	}
	if result.Verdict != "approve" {
		t.Errorf("Verdict = %q, want approve", result.Verdict)
	}
}

func TestParseToolArgs_InvalidJSON(t *testing.T) {
	input := json.RawMessage(`not valid json`)

	result := ParseToolArgs(input)

	if result.ParseErr == nil {
		t.Error("ParseErr = nil, want non-nil for invalid JSON")
	}
}

func TestParseToolArgs_InvalidVerdict(t *testing.T) {
	input := json.RawMessage(`{"summary": "test", "verdict": "reject", "findings": []}`)

	result := ParseToolArgs(input)

	if result.ParseErr == nil {
		t.Fatal("ParseErr = nil, want non-nil for invalid verdict")
	}
	if result.Verdict != "reject" {
		t.Errorf("Verdict = %q, want %q (should still be populated)", result.Verdict, "reject")
	}
	if result.Summary != "test" {
		t.Errorf("Summary = %q, want %q (should still be populated)", result.Summary, "test")
	}
}

func TestParseToolArgs_EmptyVerdict(t *testing.T) {
	input := json.RawMessage(`{"summary": "test", "verdict": "", "findings": []}`)

	result := ParseToolArgs(input)

	if result.ParseErr == nil {
		t.Fatal("ParseErr = nil, want non-nil for empty verdict")
	}
	if result.Verdict != "" {
		t.Errorf("Verdict = %q, want empty", result.Verdict)
	}
}

func TestParseToolArgs_RawEmpty(t *testing.T) {
	input := json.RawMessage(`{"summary": "test", "verdict": "approve", "findings": []}`)

	result := ParseToolArgs(input)

	if result.Raw != "" {
		t.Errorf("Raw = %q, want empty (Raw is only set for text fallback)", result.Raw)
	}
}
