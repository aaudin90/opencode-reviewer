package vcs

import "github.com/aaudin90/opencode-reviewer/internal/shared/models"

// MapFindings converts FinalFindings to DiffFindings for inline VCS commenting.
// StartLine is treated as a new-file line number (LLM convention).
// Findings with empty File or StartLine <= 0 are skipped.
func MapFindings(findings []models.FinalFinding) []DiffFinding {
	out := make([]DiffFinding, 0, len(findings))
	for _, f := range findings {
		if f.File == "" || f.StartLine <= 0 {
			continue
		}
		out = append(out, DiffFinding{
			OldPath: f.File,      // default: same path (correct for non-renamed files)
			NewPath: f.File,      // normalizer will correct for renames
			NewLine: f.StartLine, // LLM always provides new-file line numbers
			Source:  f,
		})
	}
	return out
}
