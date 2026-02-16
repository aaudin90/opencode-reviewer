package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/alecthomas/kong"

	"github.com/aaudin90/opencode-reviewer/internal/config"
	"github.com/aaudin90/opencode-reviewer/internal/git"
	"github.com/aaudin90/opencode-reviewer/internal/pipeline"
)

type CLI struct {
	Config string `kong:"required,type='path',placeholder='FILE',help='Path to TOML config file.'"`
	Branch string `kong:"placeholder='BRANCH',help='Branch to review (overrides REVIEW_BRANCH env).'"`
}

func main() {
	var cli CLI

	parser, err := kong.New(&cli,
		kong.Name("opencode-reviewer"),
		kong.Description(`Automated code review pipeline powered by OpenCode.

Config file (TOML) sections:

  [opencode]
    endpoint        API endpoint URL (default: http://localhost:4096)
    port            API port (default: 4096)
    model           Model name (e.g. anthropic/claude-sonnet-4-20250514)
    binary          Path to opencode binary (default: opencode)
    stage_timeout   Timeout per stage in seconds (default: 600)

  [git]
    project_dir     Path to the project repository
    remote          Git remote name (default: origin)
    branch          Branch to review
    base_branch     Base branch for diff (default: main)

  [output]
    file_path         Report output path (default: review-report.md)
    format_project_dir  Project dir for path formatting

Environment variables:
  REVIEW_BRANCH   Branch to review (overridden by --branch flag)`),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	_, err = parser.Parse(os.Args[1:])
	if err != nil {
		parser.FatalIfErrorf(err)
	}

	cfg, err := config.Load(cli.Config)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	if envBranch := os.Getenv("REVIEW_BRANCH"); envBranch != "" && cfg.Git.Branch == "" {
		cfg.Git.Branch = envBranch
	}

	if cli.Branch != "" {
		cfg.Git.Branch = cli.Branch
	}

	if cfg.Git.Branch == "" {
		slog.Error("branch is required: use --branch flag, REVIEW_BRANCH env, or set git.branch in config")
		os.Exit(1)
	}

	if err := run(cfg); err != nil {
		slog.Error("review failed", "error", err)
		os.Exit(1)
	}
}

func run(cfg *config.Config) error {
	projectDir, err := filepath.Abs(cfg.Git.ProjectDir)
	if err != nil {
		return fmt.Errorf("resolve project dir: %w", err)
	}

	slog.Info("starting review",
		"branch", cfg.Git.Branch,
		"base", cfg.Git.BaseBranch,
		"project", projectDir,
	)

	gitClient := git.NewClient(projectDir, cfg.Git.Remote)
	p := pipeline.New(gitClient, cfg.Git.Branch, cfg.Git.BaseBranch)

	return p.Run()
}
