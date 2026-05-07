package pipeline

import (
	"github.com/aaudin90/opencode-reviewer/internal/shared/models"
	"github.com/aaudin90/opencode-reviewer/internal/shared/runner"
)

type promptRunResult struct {
	index  int
	result *models.ReviewResult
	stats  runner.SessionStats
	err    error
}
