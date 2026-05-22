package pipeline

import (
	"github.com/aaudin90/opencode-reviewer/internal/shared/models"
	"github.com/aaudin90/opencode-reviewer/internal/shared/runner"
)

type RuntimeResources struct {
	ReviewerRunner   *runner.Runner
	FinalizerRunner  *runner.Runner
	Messages         []models.ReviewMessage
	FinalizerMessage string
	ModelChain       []string
	Cleanup          func() error
}
