package pipeline

import "github.com/aaudin90/opencode-reviewer/internal/runner"

// ReviewStageConfig holds parameters for constructing a ReviewStage.
type ReviewStageConfig struct {
	Runner   *runner.Runner
	Messages []string
}
