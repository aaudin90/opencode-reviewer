package pipeline

import (
	"github.com/aaudin90/opencode-reviewer/internal/shared/models"
	"github.com/aaudin90/opencode-reviewer/internal/shared/runner"
)

// ReviewStageConfig holds parameters for constructing a ReviewStage.
type ReviewStageConfig struct {
	Runner         *runner.Runner
	Messages       []string // deprecated plain contents for tests/backward compatibility
	ReviewMessages []models.ReviewMessage
	ModelChain     []string
}
