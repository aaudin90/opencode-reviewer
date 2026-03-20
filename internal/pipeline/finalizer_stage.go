package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/aaudin90/opencode-reviewer/internal/models"
	"github.com/aaudin90/opencode-reviewer/internal/review"
	"github.com/aaudin90/opencode-reviewer/internal/runner"
)

// FinalizerStage encapsulates Phase 2: finalizer consolidation.
type FinalizerStage struct {
	runner           *runner.Runner
	finalizerMessage string
}

// NewFinalizerStage creates a FinalizerStage from the given config.
func NewFinalizerStage(cfg FinalizerStageConfig) *FinalizerStage {
	return &FinalizerStage{
		runner:           cfg.Runner,
		finalizerMessage: cfg.FinalizerMessage,
	}
}

// Run starts the runner, executes the finalizer session and stops the runner.
func (s *FinalizerStage) Run(ctx context.Context, phase1Results []*models.ReviewResult) (*models.FinalReview, runner.SessionStats, error) {
	if err := s.runner.StartServe(ctx); err != nil {
		return nil, runner.SessionStats{}, fmt.Errorf("start finalizer serve: %w", err)
	}
	defer s.runner.StopServe()
	if err := s.runner.Precheck(ctx); err != nil {
		return nil, runner.SessionStats{}, fmt.Errorf("finalizer precheck: %w", err)
	}
	return s.runFinalizerReview(ctx, phase1Results)
}

func (s *FinalizerStage) runFinalizerReview(ctx context.Context, phase1Results []*models.ReviewResult) (*models.FinalReview, runner.SessionStats, error) {
	serialized, err := json.MarshalIndent(phase1Results, "", "  ")
	if err != nil {
		return nil, runner.SessionStats{}, fmt.Errorf("marshal phase1 results: %w", err)
	}
	userMessage := s.finalizerMessage + "\n\n## Phase 1 Results\n\n```json\n" + string(serialized) + "\n```"

	runResult, err := collectRunResult(ctx, s.runner, runner.RunRequest{
		Prompt:     userMessage,
		ToolName:   "submit_final_review",
		PromptPath: "finalizer",
		AgentName:  "finalizer",
		ValidateFunc: func(data json.RawMessage) error {
			return review.ParseFinalToolArgs(data).ParseErr
		},
		SchemaHint: finalizerSchemaHint,
	})
	if err != nil {
		return nil, runner.SessionStats{}, fmt.Errorf("finalizer session: %w", err)
	}

	if runResult.ToolArgs != nil {
		result := review.ParseFinalToolArgs(runResult.ToolArgs)
		if result.ParseErr != nil {
			slog.Warn("failed to parse written review", "error", result.ParseErr)
			if runResult.FallbackText != "" {
				fallback := review.ParseFinal(runResult.FallbackText)
				if review.IsBetterFinalResult(fallback, result) {
					result = fallback
				}
			}
		}
		slog.Info("finalizer review completed",
			"verdict", result.Verdict,
			"findings", len(result.Findings),
		)
		if prettyBytes, jsonErr := json.MarshalIndent(runResult.ToolArgs, "", "  "); jsonErr == nil {
			slog.Debug("finalizer review result", "result", string(prettyBytes))
		}
		return result, runResult.Stats, nil
	}

	slog.Warn("finalizer: using text fallback")
	return review.ParseFinal(runResult.FallbackText), runResult.Stats, nil
}
