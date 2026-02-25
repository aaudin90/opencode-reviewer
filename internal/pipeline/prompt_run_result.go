package pipeline

import "github.com/aaudin90/opencode-reviewer/internal/models"

type promptRunResult struct {
	index  int
	result *models.ReviewResult
	err    error
}
