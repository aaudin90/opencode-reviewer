package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/aaudin90/opencode-reviewer/internal/agentsmd"
	"github.com/aaudin90/opencode-reviewer/internal/diff"
	"github.com/aaudin90/opencode-reviewer/internal/git"
	"github.com/aaudin90/opencode-reviewer/internal/models"
	"github.com/aaudin90/opencode-reviewer/internal/review"
	"github.com/aaudin90/opencode-reviewer/internal/runner"
)

// Config holds all parameters needed to construct a Pipeline.
type Config struct {
	GitClient       *git.Client
	Branch          string
	BaseBranch      string
	Runner          *runner.Runner
	FinalizerRunner *runner.Runner
	PromptPaths     []string
	FinalizerPrompt string
}

type Pipeline struct {
	gitClient       *git.Client
	branch          string
	baseBranch      string
	runner          *runner.Runner
	finalizerRunner *runner.Runner
	promptPaths     []string // resolved absolute paths; must be non-empty
	finalizerPrompt string
}

func New(cfg Config) *Pipeline {
	return &Pipeline{
		gitClient:       cfg.GitClient,
		branch:          cfg.Branch,
		baseBranch:      cfg.BaseBranch,
		runner:          cfg.Runner,
		finalizerRunner: cfg.FinalizerRunner,
		promptPaths:     cfg.PromptPaths,
		finalizerPrompt: cfg.FinalizerPrompt,
	}
}

func (p *Pipeline) Run(ctx context.Context) (*models.FinalReview, error) {
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

	// Phase 1: parallel reviewer sessions.
	if err := p.runner.StartServe(ctx); err != nil {
		return nil, fmt.Errorf("start reviewer serve: %w", err)
	}
	phase1Results := p.runAllReviews(ctx)
	p.runner.StopServe()

	// Phase 2: finalizer consolidation.
	if err := p.finalizerRunner.StartServe(ctx); err != nil {
		return nil, fmt.Errorf("start finalizer serve: %w", err)
	}
	writtenReview, err := p.runFinalizerReview(ctx, phase1Results)
	p.finalizerRunner.StopServe()
	if err != nil {
		return nil, fmt.Errorf("finalizer review: %w", err)
	}

	return writtenReview, nil
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

// runAllReviews runs all Phase 1 reviewer sessions in parallel.
// Failed sessions are logged but do not stop the pipeline.
func (p *Pipeline) runAllReviews(ctx context.Context) []*models.ReviewResult {
	if len(p.promptPaths) == 0 {
		slog.Error("no prompt paths configured: set pipeline.prompt_paths in config or REVIEW_PROMPT_PATHS env")
		return nil
	}

	resultsCh := make(chan promptRunResult, len(p.promptPaths))

	for _, path := range p.promptPaths {
		go func(promptPath string) {
			result, runErr := p.runSingleReview(ctx, promptPath)
			resultsCh <- promptRunResult{path: promptPath, result: result, err: runErr}
		}(path)
	}

	results := make([]*models.ReviewResult, 0, len(p.promptPaths))
	for range p.promptPaths {
		r := <-resultsCh
		if r.err != nil {
			slog.Error("review session failed", "prompt", r.path, "error", r.err)
			continue
		}
		slog.Info("review result",
			"prompt", r.path,
			"reviewer", r.result.ReviewerName,
			"verdict", r.result.Verdict,
			"findings", len(r.result.Findings),
		)
		results = append(results, r.result)
	}

	return results
}

func (p *Pipeline) runSingleReview(ctx context.Context, promptPath string) (*models.ReviewResult, error) {
	prompt, err := readPromptFile(promptPath)
	if err != nil {
		return nil, err
	}
	var runResult *runner.RunResult
	for event := range p.runner.Run(ctx, runner.RunRequest{
		Prompt:     prompt,
		ToolName:   "submit_review",
		PromptPath: promptPath,
		AgentName:  "reviewer",
	}) {
		switch {
		case event.Err != nil:
			return nil, fmt.Errorf("run review: %w", event.Err)
		case event.ToolCall != nil:
		case event.Final != nil:
			runResult = event.Final
		}
	}
	if runResult == nil {
		return nil, fmt.Errorf("no result received")
	}
	return p.parseResult(runResult), nil
}

func (p *Pipeline) runFinalizerReview(ctx context.Context, phase1Results []*models.ReviewResult) (*models.FinalReview, error) {
	serialized, err := json.MarshalIndent(phase1Results, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal phase1 results: %w", err)
	}
	userMessage := p.finalizerPrompt + "\n\n## Phase 1 Results\n\n```json\n" + string(serialized) + "\n```"

	var runResult *runner.RunResult
	for event := range p.finalizerRunner.Run(ctx, runner.RunRequest{
		Prompt:     userMessage,
		ToolName:   "submit_final_review",
		PromptPath: "finalizer",
		AgentName:  "reviewer", // TODO 79400
	}) {
		switch {
		case event.Err != nil:
			return nil, fmt.Errorf("finalizer session: %w", event.Err)
		case event.ToolCall != nil:
		case event.Final != nil:
			runResult = event.Final
		}
	}

	if runResult == nil {
		return nil, fmt.Errorf("finalizer: no result received")
	}

	if runResult.ToolArgs != nil {
		result := review.ParseFinalToolArgs(runResult.ToolArgs)
		if result.ParseErr != nil {
			slog.Warn("failed to parse written review", "error", result.ParseErr)
		}
		slog.Info("finalizer review completed",
			"verdict", result.Verdict,
			"findings", len(result.Findings),
		)
		if prettyBytes, jsonErr := json.MarshalIndent(runResult.ToolArgs, "", "  "); jsonErr == nil {
			slog.Debug("finalizer review result", "result", string(prettyBytes))
		}
		return result, nil
	}

	slog.Warn("finalizer: using text fallback")
	return &models.FinalReview{Raw: runResult.FallbackText}, nil
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

func readPromptFile(path string) (string, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path from trusted config
	if err != nil {
		return "", fmt.Errorf("read prompt file %q: %w", path, err)
	}
	return string(data), nil
}
