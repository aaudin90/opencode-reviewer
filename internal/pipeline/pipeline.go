package pipeline

import (
	"fmt"
	"log/slog"

	"github.com/aaudin90/opencode-reviewer/internal/git"
)

type Pipeline struct {
	gitClient *git.Client
	branch    string
}

func New(gitClient *git.Client, branch string) *Pipeline {
	return &Pipeline{
		gitClient: gitClient,
		branch:    branch,
	}
}

func (p *Pipeline) Run() error {
	if err := p.prepareBranch(); err != nil {
		return fmt.Errorf("prepare branch: %w", err)
	}

	slog.Info("repository prepared for review")
	return nil
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
