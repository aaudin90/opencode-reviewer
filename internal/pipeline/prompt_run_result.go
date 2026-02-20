package pipeline

import "github.com/aaudin90/opencode-reviewer/internal/models"

type promptRunResult struct {
	path   string
	result *models.ReviewResult
	err    error
}
