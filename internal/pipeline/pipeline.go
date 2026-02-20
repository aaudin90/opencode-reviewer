package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/aaudin90/opencode-reviewer/internal/agentsmd"
	"github.com/aaudin90/opencode-reviewer/internal/diff"
	"github.com/aaudin90/opencode-reviewer/internal/git"
	"github.com/aaudin90/opencode-reviewer/internal/models"
	"github.com/aaudin90/opencode-reviewer/internal/review"
	"github.com/aaudin90/opencode-reviewer/internal/runner"
)

const reviewPrompt = "Прочитай diff из файла .opencode-review/diff.md и выполни код-ревью."

type Pipeline struct {
	gitClient  *git.Client
	branch     string
	baseBranch string
	runner     *runner.Runner
}

func New(
	gitClient *git.Client,
	branch, baseBranch string,
	r *runner.Runner,
) *Pipeline {
	return &Pipeline{
		gitClient:  gitClient,
		branch:     branch,
		baseBranch: baseBranch,
		runner:     r,
	}
}

func (p *Pipeline) Run(ctx context.Context) (*models.ReviewResult, error) {
	if err := p.prepareBranch(); err != nil {
		return nil, fmt.Errorf("prepare branch: %w", err)
	}
	slog.Info("repository prepared for review")
	defer func() {
		if cleanErr := p.gitClient.Clean(); cleanErr != nil {
			slog.Error("failed to clean working tree", "error", cleanErr)
		}
	}()

	diffPath, err := p.prepareDiff()
	if err != nil {
		return nil, fmt.Errorf("prepare diff: %w", err)
	}
	slog.Info("diff context ready", "path", diffPath)

	if err := p.swapAgentsMD(); err != nil {
		return nil, fmt.Errorf("swap agents.md: %w", err)
	}

	runResult, err := p.runReview(ctx)
	if err != nil {
		return nil, fmt.Errorf("run review: %w", err)
	}

	return p.parseResult(runResult), nil
}

func (p *Pipeline) swapAgentsMD() error {
	swapper := agentsmd.NewSwapper(p.gitClient.Dir())
	swapped, err := swapper.Swap()
	if err != nil {
		return err
	}
	slog.Info("AGENTS.md and CLAUDE.md swapped for review", "overwritten", swapped)
	return nil
}

func (p *Pipeline) runReview(ctx context.Context) (*runner.RunResult, error) {
	if err := p.runner.StartServe(ctx); err != nil {
		return nil, fmt.Errorf("start serve: %w", err)
	}
	defer p.runner.StopServe()

	var runResult *runner.RunResult
	for event := range p.runner.Run(ctx, runner.RunRequest{Prompt: reviewPrompt, ToolName: "submit_review"}) {
		switch {
		case event.Err != nil:
			return nil, fmt.Errorf("run review: %w", event.Err)
		case event.ToolCall != nil:
			tc := event.ToolCall
			slog.Debug("tool call", "tool", tc.Tool, "session", tc.SessionID, "args", string(tc.Input))
		case event.Final != nil:
			runResult = event.Final
		}
	}
	if runResult == nil {
		return nil, fmt.Errorf("no result received")
	}
	return runResult, nil
}

func (p *Pipeline) parseResult(runResult *runner.RunResult) *models.ReviewResult {
	var reviewResult *models.ReviewResult
	var outputBytes []byte

	if runResult.ToolArgs != nil {
		reviewResult = review.ParseToolArgs(runResult.ToolArgs)
		outputBytes = runResult.ToolArgs
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

	slog.Info("review completed", "verdict", reviewResult.Verdict, "findings", len(reviewResult.Findings))

	return reviewResult
}

func (p *Pipeline) prepareDiff() (string, error) {
	slog.Info("preparing diff", "branch", p.branch, "base", p.baseBranch)

	result, err := diff.Prepare(p.gitClient, p.branch, p.baseBranch)
	if err != nil {
		return "", fmt.Errorf("prepare: %w", err)
	}

	slog.Info("diff parsed",
		"files", len(result.Files),
		"filtered", len(result.FilteredFiles),
		"added", result.TotalAdded,
		"deleted", result.TotalDeleted,
		"tokens_estimate", diff.EstimateTokens(result),
	)

	path, err := diff.WriteContextFile(result, p.gitClient.Dir())
	if err != nil {
		return "", fmt.Errorf("write context file: %w", err)
	}

	return path, nil
}

func (p *Pipeline) prepareBranch() error {
	slog.Info("fetching remote")
	if err := p.gitClient.Fetch(); err != nil {
		return fmt.Errorf("fetch: %w", err)
	}

	slog.Info("cleaning working tree")
	if err := p.gitClient.Clean(); err != nil {
		return fmt.Errorf("clean: %w", err)
	}

	slog.Info("checking out branch", "branch", p.branch)
	if err := p.gitClient.Checkout(p.branch); err != nil {
		return fmt.Errorf("checkout: %w", err)
	}

	return nil
}
