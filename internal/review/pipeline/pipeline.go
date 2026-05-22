package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/aaudin90/opencode-reviewer/internal/review/agentsmd"
	"github.com/aaudin90/opencode-reviewer/internal/shared/diff"
	"github.com/aaudin90/opencode-reviewer/internal/shared/git"
	"github.com/aaudin90/opencode-reviewer/internal/shared/models"
	"github.com/aaudin90/opencode-reviewer/internal/shared/runner"
	"github.com/aaudin90/opencode-reviewer/internal/shared/vcs"
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
	Messages         []string // deprecated plain contents
	ReviewMessages   []models.ReviewMessage
	FinalizerMessage string        // finalizer user message
	RuntimeLoader    RuntimeLoader // optional; lazily creates LLM runtime after checkout
	Publisher        vcs.Publisher // optional; nil = skip publishing
	ReviewDumpPath   string        // if set, save writtenReview as JSON after LLM pipeline
	FastReviewPath   string        // if set, skip LLM stages and load review from this file
	ModelChain       []string      // primary model followed by fallback models
}

type Pipeline struct {
	gitClient      *git.Client
	branch         string
	baseBranch     string
	reviewStage    *ReviewStage
	finalizerStage *FinalizerStage
	runtimeLoader  RuntimeLoader
	runtimeCleanup func() error
	publisher      vcs.Publisher
	diffResult     *diff.Result // set by prepareDiff, used for normalization
	reviewDumpPath string
	fastReviewPath string
}

func New(cfg Config) *Pipeline {
	p := &Pipeline{
		gitClient:      cfg.GitClient,
		branch:         cfg.Branch,
		baseBranch:     cfg.BaseBranch,
		runtimeLoader:  cfg.RuntimeLoader,
		publisher:      cfg.Publisher,
		reviewDumpPath: cfg.ReviewDumpPath,
		fastReviewPath: cfg.FastReviewPath,
	}
	if cfg.Runner != nil || cfg.FinalizerRunner != nil || cfg.Messages != nil || cfg.FinalizerMessage != "" {
		p.reviewStage = NewReviewStage(ReviewStageConfig{
			Runner:         cfg.Runner,
			Messages:       cfg.Messages,
			ReviewMessages: cfg.ReviewMessages,
			ModelChain:     cfg.ModelChain,
		})
		p.finalizerStage = NewFinalizerStage(FinalizerStageConfig{
			Runner:           cfg.FinalizerRunner,
			FinalizerMessage: cfg.FinalizerMessage,
			ModelChain:       cfg.ModelChain,
		})
	}
	return p
}

func (p *Pipeline) Run(ctx context.Context) (*models.FinalReview, error) {
	if err := p.PrepareRepository(); err != nil {
		return nil, fmt.Errorf("prepare branch: %w", err)
	}
	slog.Info("repository prepared for review")
	defer func() {
		if cleanErr := p.gitClient.Clean(p.logDirs()...); cleanErr != nil {
			slog.Error("failed to clean working tree", "error", cleanErr)
		}
	}()

	return p.RunPrepared(ctx)
}

func (p *Pipeline) LogPaths() []string {
	var paths []string
	if p.reviewStage != nil && p.reviewStage.runner != nil && p.reviewStage.runner.LogPath() != "" {
		paths = append(paths, p.reviewStage.runner.LogPath())
	}
	if p.finalizerStage != nil && p.finalizerStage.runner != nil && p.finalizerStage.runner.LogPath() != "" {
		paths = append(paths, p.finalizerStage.runner.LogPath())
	}
	return paths
}

func (p *Pipeline) logDirs() []string {
	seen := make(map[string]struct{})
	var dirs []string
	for _, path := range p.LogPaths() {
		dir := filepath.Dir(path)
		if _, ok := seen[dir]; ok {
			continue
		}
		seen[dir] = struct{}{}
		dirs = append(dirs, dir)
	}
	return dirs
}

func (p *Pipeline) RunPrepared(ctx context.Context) (*models.FinalReview, error) {
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

	if err := p.ensureRuntime(ctx); err != nil {
		return nil, fmt.Errorf("load runtime: %w", err)
	}
	defer p.cleanupRuntime()

	if err := p.swapAgentsMD(); err != nil {
		return nil, fmt.Errorf("swap agents.md: %w", err)
	}

	phase1Start := time.Now()
	slog.Info("starting Phase 1: reviewer sessions", "sessions_count", p.reviewStage.MessageCount())
	phase1Results, phase1Stats, err := p.reviewStage.Run(ctx)
	if err != nil {
		return nil, fmt.Errorf("review stage: %w", err)
	}
	slog.Info("Phase 1 completed", "results", len(phase1Results), "elapsed", time.Since(phase1Start).String())

	phase2Start := time.Now()
	slog.Info("starting Phase 2: finalizer")
	writtenReview, finalizerStats, err := p.finalizerStage.Run(ctx, phase1Results)
	if err != nil {
		slog.Error("Phase 2 failed", "elapsed", time.Since(phase2Start).String(), "error", err)
		return nil, fmt.Errorf("finalizer review: %w", err)
	}
	slog.Info("Phase 2 completed", "elapsed", time.Since(phase2Start).String())
	applyFindingProvenance(writtenReview, phase1Results)

	if p.reviewDumpPath != "" {
		if dumpErr := p.saveReview(writtenReview, p.reviewDumpPath); dumpErr != nil {
			slog.Warn("failed to save review dump", "path", p.reviewDumpPath, "error", dumpErr)
		} else {
			slog.Info("review dump saved", "path", p.reviewDumpPath)
		}
	}

	p.publishReview(ctx, writtenReview)
	p.logTotalStats(phase1Stats, finalizerStats, len(phase1Stats)+1) // +1 for finalizer session

	return writtenReview, nil
}

func (p *Pipeline) ensureRuntime(ctx context.Context) error {
	if p.reviewStage != nil && p.finalizerStage != nil {
		return nil
	}
	if p.runtimeLoader == nil {
		return fmt.Errorf("runtime loader is not configured")
	}

	resources, err := p.runtimeLoader.LoadRuntime(ctx)
	if err != nil {
		return err
	}
	if resources == nil {
		return fmt.Errorf("runtime loader returned nil resources")
	}

	p.reviewStage = NewReviewStage(ReviewStageConfig{
		Runner:         resources.ReviewerRunner,
		ReviewMessages: resources.Messages,
		ModelChain:     resources.ModelChain,
	})
	p.finalizerStage = NewFinalizerStage(FinalizerStageConfig{
		Runner:           resources.FinalizerRunner,
		FinalizerMessage: resources.FinalizerMessage,
		ModelChain:       resources.ModelChain,
	})
	p.runtimeCleanup = resources.Cleanup

	return nil
}

func (p *Pipeline) cleanupRuntime() {
	if p.runtimeCleanup == nil {
		return
	}
	if err := p.runtimeCleanup(); err != nil {
		slog.Error("failed to cleanup runtime", "error", err)
	}
	p.runtimeCleanup = nil
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

func (p *Pipeline) logTotalStats(reviewerStats []runner.SessionStats, finalizerStats runner.SessionStats, totalSessions int) {
	var total runner.SessionStats
	for _, s := range reviewerStats {
		total = total.Add(s)
	}
	total = total.Add(finalizerStats)

	slog.Info("total review stats",
		"sessions", totalSessions,
		"cost", total.Cost,
		"tokens_input", total.Tokens.Input,
		"tokens_output", total.Tokens.Output,
		"tokens_reasoning", total.Tokens.Reasoning,
		"tokens_cache_read", total.Tokens.Cache.Read,
		"tokens_cache_write", total.Tokens.Cache.Write,
	)
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

func (p *Pipeline) PrepareRepository() error {
	return git.PrepareRepository(p.gitClient, p.branch)
}
