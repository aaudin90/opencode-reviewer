package vcs

import "github.com/aaudin90/opencode-reviewer/internal/shared/models"

// DiffFinding is a finding mapped to an inline VCS comment position.
// OldLine > 0: deleted line (sent as old_line in GitLab API).
// NewLine > 0: added or context line (sent as new_line in GitLab API).
// OldPath is the file path in the old commit (old_path in GitLab API).
// NewPath is the file path in the new commit (new_path in GitLab API).
// For non-renamed files OldPath == NewPath.
// InDiff is true when the line is present in a diff hunk and can be posted inline.
type DiffFinding struct {
	OldPath string
	NewPath string
	OldLine int
	NewLine int
	InDiff  bool                // true — line is in a diff hunk, inline posting is possible
	Source  models.FinalFinding // original finding, used for body formatting
}
