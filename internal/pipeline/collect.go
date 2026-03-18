package pipeline

import (
	"context"
	"fmt"

	"github.com/aaudin90/opencode-reviewer/internal/runner"
)

func collectRunResult(ctx context.Context, r *runner.Runner, req runner.RunRequest) (*runner.RunResult, error) {
	var result *runner.RunResult
	for event := range r.Run(ctx, req) {
		switch {
		case event.Err != nil:
			return nil, event.Err
		case event.Final != nil:
			result = event.Final
		}
	}
	if result == nil {
		return nil, fmt.Errorf("no result received")
	}
	return result, nil
}
