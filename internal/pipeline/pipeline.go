package pipeline

import (
	"context"
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

const reviewPrompt = "Прочитай diff из файла .opencode-review/diff.md и выполни код-ревью."

type Pipeline struct {
	gitClient  *git.Client
	branch     string
	baseBranch string
	runner     *runner.Runner
	outputPath string
}

func New(
	gitClient *git.Client,
	branch, baseBranch string,
	r *runner.Runner,
	outputPath string,
) *Pipeline {
	return &Pipeline{
		gitClient:  gitClient,
		branch:     branch,
		baseBranch: baseBranch,
		runner:     r,
		outputPath: outputPath,
	}
}

func (p *Pipeline) Run(ctx context.Context) (*models.ReviewResult, error) {
	if err := p.prepareBranch(); err != nil {
		return nil, fmt.Errorf("prepare branch: %w", err)
	}

	slog.Info("repository prepared for review")

	// defer clean right after prepareBranch to ensure cleanup
	// even if prepareDiff or Swap fails
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

	swapper := agentsmd.NewSwapper(p.gitClient.Dir())
	swapped, err := swapper.Swap()
	if err != nil {
		return nil, fmt.Errorf("swap agents.md: %w", err)
	}

	slog.Info("AGENTS.md and CLAUDE.md swapped for review", "overwritten", swapped)

	if err := p.runner.StartServe(ctx); err != nil {
		return nil, fmt.Errorf("start serve: %w", err)
	}
	defer p.runner.StopServe()

	raw, err := p.runner.Run(ctx, runner.RunRequest{
		Prompt: reviewPrompt,
	})
	if err != nil {
		return nil, fmt.Errorf("run review: %w", err)
	}

	reviewResult := review.Parse(raw)
	if reviewResult.ParseErr != nil {
		slog.Warn("failed to parse agent response as JSON", "error", reviewResult.ParseErr)
	} else {
		slog.Info("review parsed", "findings", len(reviewResult.Findings))
	}

	fmt.Println()
	fmt.Println("════════════════════════════════════════")
	fmt.Println("  Review Result")
	fmt.Println("════════════════════════════════════════")
	fmt.Println()
	fmt.Println(raw)
	fmt.Println()
	fmt.Println("════════════════════════════════════════")

	if err := os.WriteFile(p.outputPath, []byte(raw), 0o600); err != nil {
		return nil, fmt.Errorf("write output: %w", err)
	}

	slog.Info("review output written", "path", p.outputPath)

	return reviewResult, nil
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
