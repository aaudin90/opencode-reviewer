package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/aaudin90/opencode-reviewer/internal/review"
	"github.com/aaudin90/opencode-reviewer/internal/shared/models"
	"github.com/aaudin90/opencode-reviewer/internal/shared/runner"
)

// ReviewStage encapsulates Phase 1: parallel reviewer sessions.
type ReviewStage struct {
	runner   *runner.Runner
	messages []models.ReviewMessage
	models   []string
}

// NewReviewStage creates a ReviewStage from the given config.
func NewReviewStage(cfg ReviewStageConfig) *ReviewStage {
	messages := cfg.ReviewMessages
	if len(messages) == 0 && len(cfg.Messages) > 0 {
		messages = make([]models.ReviewMessage, 0, len(cfg.Messages))
		for i, content := range cfg.Messages {
			messages = append(messages, models.ReviewMessage{
				Ref:     models.ReviewMessageRef{ID: fmt.Sprintf("message-%d", i)},
				Content: content,
			})
		}
	}
	return &ReviewStage{
		runner:   cfg.Runner,
		messages: messages,
		models:   normalizeModelChain(cfg.ModelChain),
	}
}

// Run starts the runner, executes all review sessions and stops the runner.
func (s *ReviewStage) Run(ctx context.Context) ([]*models.ReviewResult, []runner.SessionStats, error) {
	if len(s.messages) == 0 {
		return nil, nil, fmt.Errorf("no review messages configured")
	}
	models, err := startServeWithModelFallback(ctx, s.runner, "reviewer", "reviewer", s.models)
	if err != nil {
		return nil, nil, err
	}
	defer s.runner.StopServe()
	results, stats := s.runAllReviews(ctx, models)
	if failed := len(s.messages) - len(results); failed > 0 {
		return nil, nil, fmt.Errorf("%d of %d review sessions failed; models exhausted: %v", failed, len(s.messages), models)
	}
	return results, stats, nil
}

// MessageCount returns the number of configured review messages.
func (s *ReviewStage) MessageCount() int { return len(s.messages) }

// runAllReviews runs all reviewer sessions in parallel.
func (s *ReviewStage) runAllReviews(ctx context.Context, modelChain []string) ([]*models.ReviewResult, []runner.SessionStats) {
	resultsCh := make(chan promptRunResult, len(s.messages))

	for idx, msg := range s.messages {
		go func(message models.ReviewMessage, i int) {
			result, stats, runErr := s.runSingleReview(ctx, message, i, modelChain)
			resultsCh <- promptRunResult{index: i, result: result, stats: stats, err: runErr}
		}(msg, idx)
	}

	resultsByIndex := make([]*models.ReviewResult, len(s.messages))
	statsByIndex := make([]runner.SessionStats, len(s.messages))
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
			"models", r.stats.Models,
			"fallback_models", r.stats.FallbackModels,
			"model_costs", r.stats.ModelCosts,
		)
		resultsByIndex[r.index] = r.result
		statsByIndex[r.index] = r.stats
	}

	results := make([]*models.ReviewResult, 0, len(s.messages))
	allStats := make([]runner.SessionStats, 0, len(s.messages))
	for idx := range s.messages {
		if resultsByIndex[idx] == nil {
			continue
		}
		results = append(results, resultsByIndex[idx])
		allStats = append(allStats, statsByIndex[idx])
	}

	return results, allStats
}

func (s *ReviewStage) runSingleReview(ctx context.Context, message models.ReviewMessage, idx int, modelChain []string) (*models.ReviewResult, runner.SessionStats, error) {
	slog.Info("starting review session", "prompt_index", idx, "model_chain", modelChain)
	runResult, err := collectRunResult(ctx, s.runner, runner.RunRequest{
		Prompt:               message.Content,
		ToolName:             "submit_review",
		PromptPath:           fmt.Sprintf("message-%d", idx),
		AgentName:            "reviewer",
		ModelChain:           modelChain,
		ConfiguredModelChain: s.models,
		ValidateFunc: func(data json.RawMessage) error {
			return review.ParseToolArgs(data).ParseErr
		},
		SchemaHint: reviewSchemaHint,
	})
	if err != nil {
		return nil, runner.SessionStats{}, fmt.Errorf("run review: %w", err)
	}
	result := s.parseResult(runResult)
	result.MessageRef = message.Ref
	stats := runResult.Stats.WithFallbackModels(s.models)
	return result, stats, nil
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
