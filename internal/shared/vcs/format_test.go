package vcs

import (
	"strings"
	"testing"

	"github.com/aaudin90/opencode-reviewer/internal/shared/models"
)

func TestFormatReviewNote_Nil(t *testing.T) {
	got := FormatReviewNote(nil)
	if !strings.Contains(got, "no structured output") {
		t.Errorf("expected fallback message, got:\n%s", got)
	}
}

func TestFormatReviewNote_ApproveNoFindings(t *testing.T) {
	rev := &models.FinalReview{
		Verdict: "approve",
		Summary: "Everything looks great.",
	}
	got := FormatReviewNote(rev)
	if !strings.Contains(got, "\u2705") {
		t.Error("expected checkmark emoji for approve")
	}
	if !strings.Contains(got, "Approved") {
		t.Error("expected 'Approved' label")
	}
	if !strings.Contains(got, "No issues found") {
		t.Error("expected 'No issues found' for empty findings")
	}
}

func TestFormatReviewNote_RequestChangesWithFindings(t *testing.T) {
	rev := &models.FinalReview{
		Verdict: "request_changes",
		Summary: "Several issues.",
		Findings: []models.FinalFinding{
			{File: "main.go", StartLine: 10, EndLine: 10, Confidence: "high", IssueContent: "Bug found"},
			{File: "util.go", StartLine: 5, EndLine: 5, Confidence: "low", IssueContent: "Minor issue"},
		},
	}
	got := FormatReviewNote(rev)
	if !strings.Contains(got, "\U0001f534") {
		t.Error("expected red circle for request_changes")
	}
	if !strings.Contains(got, "Changes Requested") {
		t.Error("expected 'Changes Requested' label")
	}
	if !strings.Contains(got, "### Findings") {
		t.Error("expected Findings section")
	}
	if !strings.Contains(got, "1.") || !strings.Contains(got, "2.") {
		t.Error("expected numbered findings")
	}
}

func TestFormatReviewNote_RawFallback(t *testing.T) {
	rev := &models.FinalReview{
		Verdict: "",
		Raw:     "Some raw markdown review text.",
	}
	got := FormatReviewNote(rev)
	if !strings.Contains(got, "Some raw markdown review text.") {
		t.Error("expected raw text in output")
	}
	if !strings.Contains(got, "## Automated Code Review") {
		t.Error("expected header")
	}
}

func TestFormatReviewNote_EndLineRange(t *testing.T) {
	rev := &models.FinalReview{
		Verdict: "comment_only",
		Summary: "Minor notes.",
		Findings: []models.FinalFinding{
			{File: "main.go", StartLine: 42, EndLine: 48, Confidence: "medium", IssueContent: "Complex block"},
		},
	}
	got := FormatReviewNote(rev)
	if !strings.Contains(got, "main.go:42\u201348") {
		t.Errorf("expected file:42\u201348 range, got:\n%s", got)
	}
}

func TestFormatFindingNote_Full(t *testing.T) {
	f := models.FinalFinding{
		Confidence:     "high",
		Sources:        []string{"reviewer-1", "reviewer-2"},
		IssueContent:   "Potential nil dereference.",
		Recommendation: "Add a nil check before use.",
	}
	got := FormatFindingNote(f)
	if !strings.Contains(got, "\U0001f534 High") {
		t.Error("expected high confidence badge")
	}
	if !strings.Contains(got, "(reviewer-1, reviewer-2)") {
		t.Error("expected sources in parentheses")
	}
	if !strings.Contains(got, "Potential nil dereference.") {
		t.Error("expected issue content")
	}
	if !strings.Contains(got, "**Recommendation:**") {
		t.Error("expected recommendation section")
	}
}

func TestFormatFindingNote_EmptyRecommendation(t *testing.T) {
	f := models.FinalFinding{
		Confidence:     "low",
		IssueContent:   "Style issue.",
		Recommendation: "",
	}
	got := FormatFindingNote(f)
	if strings.Contains(got, "Recommendation") {
		t.Error("should not contain recommendation section when empty")
	}
}

func TestFormatReviewNote_Skipped(t *testing.T) {
	rev := &models.FinalReview{
		Verdict: "skipped",
		Summary: "Outside scope.",
	}
	got := FormatReviewNote(rev)
	if !strings.Contains(got, "\u23ed\ufe0f") {
		t.Error("expected ⏭️ emoji for skipped")
	}
	if !strings.Contains(got, "Skipped") {
		t.Error("expected 'Skipped' label")
	}
}
