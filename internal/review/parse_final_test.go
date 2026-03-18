package review

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/aaudin90/opencode-reviewer/internal/models"
)

func TestParseFinalToolArgs_FindingsWrongType(t *testing.T) {
	input := json.RawMessage(`{
		"summary": "Code looks good overall",
		"verdict": "approve",
		"findings": "nothing to report"
	}`)

	result := ParseFinalToolArgs(input)

	if result.ParseErr == nil {
		t.Fatal("ParseErr = nil, want non-nil when findings has wrong type")
	}
	if result.Summary != "Code looks good overall" {
		t.Errorf("Summary = %q, want %q", result.Summary, "Code looks good overall")
	}
	if result.Verdict != "approve" {
		t.Errorf("Verdict = %q, want %q", result.Verdict, "approve")
	}
}

func TestIsBetterFinalResult(t *testing.T) {
	tests := []struct {
		name      string
		candidate *models.FinalReview
		current   *models.FinalReview
		want      bool
	}{
		{
			name:      "candidate has no error",
			candidate: &models.FinalReview{Verdict: "approve"},
			current:   &models.FinalReview{ParseErr: fmt.Errorf("err")},
			want:      true,
		},
		{
			name:      "candidate has more findings",
			candidate: &models.FinalReview{ParseErr: fmt.Errorf("err"), Findings: make([]models.FinalFinding, 2)},
			current:   &models.FinalReview{ParseErr: fmt.Errorf("err"), Findings: make([]models.FinalFinding, 1)},
			want:      true,
		},
		{
			name:      "candidate has verdict, current does not",
			candidate: &models.FinalReview{ParseErr: fmt.Errorf("err"), Verdict: "approve"},
			current:   &models.FinalReview{ParseErr: fmt.Errorf("err")},
			want:      true,
		},
		{
			name:      "candidate is not better",
			candidate: &models.FinalReview{ParseErr: fmt.Errorf("err")},
			current:   &models.FinalReview{ParseErr: fmt.Errorf("err"), Verdict: "approve"},
			want:      false,
		},
		{
			name:      "candidate has error, current is clean",
			candidate: &models.FinalReview{ParseErr: fmt.Errorf("err"), Findings: make([]models.FinalFinding, 5)},
			current:   &models.FinalReview{Verdict: "approve"},
			want:      false,
		},
		{
			name:      "both no error keeps current",
			candidate: &models.FinalReview{Verdict: "approve"},
			current:   &models.FinalReview{Verdict: "approve"},
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsBetterFinalResult(tt.candidate, tt.current); got != tt.want {
				t.Errorf("IsBetterFinalResult() = %v, want %v", got, tt.want)
			}
		})
	}
}
