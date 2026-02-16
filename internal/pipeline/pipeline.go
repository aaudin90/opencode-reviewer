package pipeline

import (
	"fmt"
	"log/slog"

	"github.com/aaudin90/opencode-reviewer/internal/diff"
	"github.com/aaudin90/opencode-reviewer/internal/git"
)

type Pipeline struct {
	gitClient  *git.Client
	branch     string
	baseBranch string
}

func New(gitClient *git.Client, branch, baseBranch string) *Pipeline {
	return &Pipeline{
		gitClient:  gitClient,
		branch:     branch,
		baseBranch: baseBranch,
	}
}

func (p *Pipeline) Run() error {
	if err := p.prepareBranch(); err != nil {
		return fmt.Errorf("prepare branch: %w", err)
	}

	slog.Info("repository prepared for review")

	diffPath, err := p.prepareDiff()
	if err != nil {
		return fmt.Errorf("prepare diff: %w", err)
	}

	slog.Info("diff context ready", "path", diffPath)

	return nil
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
