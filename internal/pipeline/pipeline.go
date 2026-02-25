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
	"github.com/aaudin90/opencode-reviewer/internal/vcs"
)

// finalReviewDump is used to persist and restore FinalReview across runs.
type finalReviewDump struct {
	Summary  string                `json:"summary"`
	Verdict  string                `json:"verdict"`
	Findings []models.FinalFinding `json:"findings"`
}

// Config holds all parameters needed to construct a Pipeline.
type Config struct {
	GitClient        *git.Client
	Branch           string
	BaseBranch       string
	Runner           *runner.Runner
	FinalizerRunner  *runner.Runner
	Messages         []string      // reviewer message contents
	FinalizerMessage string        // finalizer user message
	Publisher        vcs.Publisher // optional; nil = skip publishing
	ReviewDumpPath   string        // if set, save writtenReview as JSON after LLM pipeline
	FastReviewPath   string        // if set, skip LLM stages and load review from this file
}

type Pipeline struct {
	gitClient        *git.Client
	branch           string
	baseBranch       string
	runner           *runner.Runner
	finalizerRunner  *runner.Runner
	messages         []string // reviewer message contents; must be non-empty
	finalizerMessage string
	publisher        vcs.Publisher
	diffResult       *diff.Result // set by prepareDiff, used for normalization
	reviewDumpPath   string
	fastReviewPath   string
}

func New(cfg Config) *Pipeline {
	return &Pipeline{
		gitClient:        cfg.GitClient,
		branch:           cfg.Branch,
		baseBranch:       cfg.BaseBranch,
		runner:           cfg.Runner,
		finalizerRunner:  cfg.FinalizerRunner,
		messages:         cfg.Messages,
		finalizerMessage: cfg.FinalizerMessage,
		publisher:        cfg.Publisher,
		reviewDumpPath:   cfg.ReviewDumpPath,
		fastReviewPath:   cfg.FastReviewPath,
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

	if p.fastReviewPath != "" {
		writtenReview, err := p.loadReview(p.fastReviewPath)
		if err != nil {
			return nil, fmt.Errorf("load review dump: %w", err)
		}
		slog.Info("fast path: loaded review from file", "path", p.fastReviewPath)
		p.publishReview(ctx, writtenReview)
		return writtenReview, nil
	}

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

	if p.reviewDumpPath != "" {
		if dumpErr := p.saveReview(writtenReview, p.reviewDumpPath); dumpErr != nil {
			slog.Warn("failed to save review dump", "path", p.reviewDumpPath, "error", dumpErr)
		} else {
			slog.Info("review dump saved", "path", p.reviewDumpPath)
		}
	}

	p.publishReview(ctx, writtenReview)

	return writtenReview, nil
}

func (p *Pipeline) publishReview(ctx context.Context, review *models.FinalReview) {
	if p.publisher == nil || review == nil || p.diffResult == nil {
		return
	}
	normalizer := vcs.NewNormalizer(p.diffResult)

	// Correct StartLine/EndLine in findings for display in summary note.
	normalizedReview := *review
	normalizedReview.Findings = normalizer.Normalize(review.Findings)

	// Map findings to inline positions and normalize old/new line numbers.
	inline := normalizer.NormalizeDiff(vcs.MapFindings(review.Findings))

	if err := p.publisher.Publish(ctx, &normalizedReview, inline, p.branch, p.baseBranch); err != nil {
		slog.Warn("failed to publish review to VCS", "error", err)
	}
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
	if len(p.messages) == 0 {
		slog.Error("no messages configured: set pipeline.review_message_paths in config or REVIEW_MESSAGE_PATHS env")
		return nil
	}

	resultsCh := make(chan promptRunResult, len(p.messages))

	for idx, msg := range p.messages {
		go func(message string, i int) {
			result, runErr := p.runSingleReview(ctx, message, i)
			resultsCh <- promptRunResult{index: i, result: result, err: runErr}
		}(msg, idx)
	}

	results := make([]*models.ReviewResult, 0, len(p.messages))
	for range p.messages {
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
	}

	return results
}

func (p *Pipeline) runSingleReview(ctx context.Context, message string, idx int) (*models.ReviewResult, error) {
	var runResult *runner.RunResult
	for event := range p.runner.Run(ctx, runner.RunRequest{
		Prompt:     message,
		ToolName:   "submit_review",
		PromptPath: fmt.Sprintf("message-%d", idx),
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
	userMessage := p.finalizerMessage + "\n\n## Phase 1 Results\n\n```json\n" + string(serialized) + "\n```"

	var runResult *runner.RunResult
	for event := range p.finalizerRunner.Run(ctx, runner.RunRequest{
		Prompt:     userMessage,
		ToolName:   "submit_final_review",
		PromptPath: "finalizer",
		AgentName:  "finalizer",
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
	return review.ParseFinal(runResult.FallbackText), nil
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
	p.diffResult = result

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

func (p *Pipeline) saveReview(review *models.FinalReview, path string) error {
	dump := finalReviewDump{
		Summary:  review.Summary,
		Verdict:  review.Verdict,
		Findings: review.Findings,
	}
	data, err := json.MarshalIndent(dump, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func (p *Pipeline) loadReview(path string) (*models.FinalReview, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is a CLI flag provided by the operator
	if err != nil {
		return nil, err
	}
	var dump finalReviewDump
	if err := json.Unmarshal(data, &dump); err != nil {
		return nil, err
	}
	return &models.FinalReview{
		Summary:  dump.Summary,
		Verdict:  dump.Verdict,
		Findings: dump.Findings,
	}, nil
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
