package pipeline

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/aaudin90/opencode-reviewer/internal/shared/runner"
)

func startServeWithModelFallback(ctx context.Context, r *runner.Runner, stageName, agentName string, models []string) ([]string, error) {
	if err := r.StartServe(ctx); err != nil {
		return nil, fmt.Errorf("start %s serve: %w", stageName, err)
	}
	models, err := precheckModelChain(ctx, r, stageName, agentName, models)
	if err != nil {
		r.StopServe()
		return nil, err
	}
	return models, nil
}

func precheckModelChain(ctx context.Context, r *runner.Runner, stageName, agentName string, models []string) ([]string, error) {
	var errs []error
	for idx, model := range models {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("%s precheck model %q: %w", stageName, model, err)
		}
		if err := r.Precheck(ctx, agentName, model); err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, fmt.Errorf("%s precheck model %q: %w", stageName, model, err)
			}
			slog.Warn("precheck failed, trying next model", "stage", stageName, "model", model, "error", err)
			errs = append(errs, fmt.Errorf("%s: %w", modelLabel(model), err))
			continue
		}
		return models[idx:], nil
	}
	return nil, fmt.Errorf("%s precheck failed for all models %v: %w", stageName, models, errors.Join(errs...))
}
