package pipeline

import "github.com/aaudin90/opencode-reviewer/internal/runner"

type RuntimeResources struct {
	ReviewerRunner   *runner.Runner
	FinalizerRunner  *runner.Runner
	Messages         []string
	FinalizerMessage string
	Cleanup          func() error
}
