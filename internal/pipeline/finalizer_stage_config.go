package pipeline

import "github.com/aaudin90/opencode-reviewer/internal/runner"

// FinalizerStageConfig holds parameters for constructing a FinalizerStage.
type FinalizerStageConfig struct {
	Runner           *runner.Runner
	FinalizerMessage string
}
