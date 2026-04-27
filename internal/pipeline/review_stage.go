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

// ReviewStage encapsulates Phase 1: parallel reviewer sessions.
type ReviewStage struct {
	runner   *runner.Runner
	messages []string
}

// NewReviewStage creates a ReviewStage from the given config.
func NewReviewStage(cfg ReviewStageConfig) *ReviewStage {
	return &ReviewStage{
		runner:   cfg.Runner,
		messages: cfg.Messages,
	}
}

// Run starts the runner, executes all review sessions and stops the runner.
func (s *ReviewStage) Run(ctx context.Context) ([]*models.ReviewResult, []runner.SessionStats, error) {
	if len(s.messages) == 0 {
		return nil, nil, fmt.Errorf("no review messages configured")
	}
	if err := s.runner.StartServe(ctx); err != nil {
		return nil, nil, fmt.Errorf("start reviewer serve: %w", err)
	}
	defer s.runner.StopServe()
	if err := s.runner.Precheck(ctx, "reviewer"); err != nil {
		return nil, nil, fmt.Errorf("reviewer precheck: %w", err)
	}
	results, stats := s.runAllReviews(ctx)
	if failed := len(s.messages) - len(results); failed > 0 {
		return nil, nil, fmt.Errorf("%d of %d review sessions failed", failed, len(s.messages))
	}
	return results, stats, nil
}

// MessageCount returns the number of configured review messages.
func (s *ReviewStage) MessageCount() int { return len(s.messages) }

// runAllReviews runs all reviewer sessions in parallel.
func (s *ReviewStage) runAllReviews(ctx context.Context) ([]*models.ReviewResult, []runner.SessionStats) {
	resultsCh := make(chan promptRunResult, len(s.messages))

	for idx, msg := range s.messages {
		go func(message string, i int) {
			result, stats, runErr := s.runSingleReview(ctx, message, i)
			resultsCh <- promptRunResult{index: i, result: result, stats: stats, err: runErr}
		}(msg, idx)
	}

	results := make([]*models.ReviewResult, 0, len(s.messages))
	var allStats []runner.SessionStats
	for range s.messages {
		r := <-resultsCh
		if r.err != nil {
			slog.Error("review session failed", "prompt_index", r.index, "error", r.err)
			continue
		}
		slog.Info("review result",
			"prompt_index", r.index,
			"reviewer", r.result.ReviewerName,
			"verdict", r.result.Verdict,
			"findings", len(r.result.Findings),
		)
		results = append(results, r.result)
		allStats = append(allStats, r.stats)
	}

	return results, allStats
}

func (s *ReviewStage) runSingleReview(ctx context.Context, message string, idx int) (*models.ReviewResult, runner.SessionStats, error) {
	slog.Info("starting review session", "prompt_index", idx)
	runResult, err := collectRunResult(ctx, s.runner, runner.RunRequest{
		Prompt:     message,
		ToolName:   "submit_review",
		PromptPath: fmt.Sprintf("message-%d", idx),
		AgentName:  "reviewer",
		ValidateFunc: func(data json.RawMessage) error {
			return review.ParseToolArgs(data).ParseErr
		},
		SchemaHint: reviewSchemaHint,
	})
	if err != nil {
		return nil, runner.SessionStats{}, fmt.Errorf("run review: %w", err)
	}
	return s.parseResult(runResult), runResult.Stats, nil
}

func (s *ReviewStage) parseResult(runResult *runner.RunResult) *models.ReviewResult {
	var reviewResult *models.ReviewResult
	var outputBytes []byte

	if runResult.ToolArgs != nil {
		reviewResult = review.ParseToolArgs(runResult.ToolArgs)
		outputBytes = runResult.ToolArgs
		if reviewResult.ParseErr != nil && runResult.FallbackText != "" {
			fallback := review.Parse(runResult.FallbackText)
			if review.IsBetterResult(fallback, reviewResult) {
				reviewResult = fallback
				outputBytes = []byte(runResult.FallbackText)
			}
		}
	} else {
		slog.Warn("using text fallback for review result")
		reviewResult = review.Parse(runResult.FallbackText)
		outputBytes = []byte(runResult.FallbackText)
	}

	if reviewResult.ParseErr != nil {
		slog.Warn("failed to parse review result", "error", reviewResult.ParseErr)
	}

	prettyBytes := outputBytes
	if runResult.ToolArgs != nil {
		if b, jsonErr := json.MarshalIndent(json.RawMessage(outputBytes), "", "  "); jsonErr == nil {
			prettyBytes = b
		}
	}
	slog.Debug("intermediate review result", "result", string(prettyBytes))

	return reviewResult
}
