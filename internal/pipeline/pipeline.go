package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/aaudin90/opencode-reviewer/internal/agentsmd"
	"github.com/aaudin90/opencode-reviewer/internal/diff"
	"github.com/aaudin90/opencode-reviewer/internal/git"
	"github.com/aaudin90/opencode-reviewer/internal/models"
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
	gitClient      *git.Client
	branch         string
	baseBranch     string
	reviewStage    *ReviewStage
	finalizerStage *FinalizerStage
	publisher      vcs.Publisher
	diffResult     *diff.Result // set by prepareDiff, used for normalization
	reviewDumpPath string
	fastReviewPath string
}

func New(cfg Config) *Pipeline {
	return &Pipeline{
		gitClient:  cfg.GitClient,
		branch:     cfg.Branch,
		baseBranch: cfg.BaseBranch,
		reviewStage: NewReviewStage(ReviewStageConfig{
			Runner:   cfg.Runner,
			Messages: cfg.Messages,
		}),
		finalizerStage: NewFinalizerStage(FinalizerStageConfig{
			Runner:           cfg.FinalizerRunner,
			FinalizerMessage: cfg.FinalizerMessage,
		}),
		publisher:      cfg.Publisher,
		reviewDumpPath: cfg.ReviewDumpPath,
		fastReviewPath: cfg.FastReviewPath,
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

	slog.Info("project directory ready", "size_mb", dirSizeMB(p.gitClient.Dir()))
	return nil
}

// dirSizeMB returns the total size of path in megabytes.
// Returns -1 on error.
func dirSizeMB(path string) int64 {
	var total int64
	err := filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			info, infoErr := d.Info()
			if infoErr != nil {
				return nil
			}
			total += info.Size()
		}
		return nil
	})
	if err != nil {
		return -1
	}
	return total / (1024 * 1024)
}
