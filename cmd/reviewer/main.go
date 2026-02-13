package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/alecthomas/kong"

	"github.com/aaudin90/opencode-reviewer/internal/agentsmd"
	"github.com/aaudin90/opencode-reviewer/internal/config"
	"github.com/aaudin90/opencode-reviewer/internal/git"
	"github.com/aaudin90/opencode-reviewer/internal/runner"
)

type CLI struct {
	Config string `kong:"required,type='path',help='Path to TOML config file.'"`
	Branch string `kong:"help='Branch to review.'"`
}

func main() {
	var cli CLI

	parser, err := kong.New(&cli,
		kong.Name("opencode-reviewer"),
		kong.Description("Automated code review pipeline powered by OpenCode."),
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

	if cli.Branch != "" {
		cfg.Git.Branch = cli.Branch
	}

	if cfg.Git.Branch == "" {
		slog.Error("branch is required: use --branch flag or set git.branch in config")
		os.Exit(1)
	}

	if err := run(cfg); err != nil {
		slog.Error("review failed", "error", err)
		os.Exit(1)
	}
}

func run(cfg *config.Config) error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	projectDir, err := filepath.Abs(cfg.Git.ProjectDir)
	if err != nil {
		return fmt.Errorf("resolve project dir: %w", err)
	}

	slog.Info("starting review",
		"branch", cfg.Git.Branch,
		"base", cfg.Git.BaseBranch,
		"project", projectDir,
	)

	// 1. Git fetch + diff.
	gitClient := git.NewClient(projectDir, cfg.Git.Remote)

	slog.Info("fetching remote")

	if err := gitClient.Fetch(); err != nil {
		return fmt.Errorf("git fetch: %w", err)
	}

	diff, err := gitClient.Diff(cfg.Git.BaseBranch, cfg.Git.Branch)
	if err != nil {
		return fmt.Errorf("git diff: %w", err)
	}

	if strings.TrimSpace(diff) == "" {
		slog.Info("no changes found between branches")
		return nil
	}

	files, err := gitClient.DiffFiles(cfg.Git.BaseBranch, cfg.Git.Branch)
	if err != nil {
		return fmt.Errorf("git diff files: %w", err)
	}

	slog.Info("diff collected", "files", len(files), "diff_bytes", len(diff))

	// 2. Swap AGENTS.md.
	agentsSrc := filepath.Join(projectDir, "prompts", "review-agents.md")

	if err := agentsmd.Swap(projectDir, agentsSrc); err != nil {
		return fmt.Errorf("swap AGENTS.md: %w", err)
	}

	defer func() {
		if restoreErr := agentsmd.Restore(projectDir); restoreErr != nil {
			slog.Error("failed to restore AGENTS.md", "error", restoreErr)
		}
	}()

	// 3. Build prompt.
	prompt := buildPrompt(diff, files)

	// 4. Start opencode serve + run review.
	r := runner.NewRunner(
		cfg.OpenCode.Binary,
		cfg.OpenCode.Endpoint,
		projectDir,
		cfg.OpenCode.Port,
		cfg.OpenCode.StageTimeout,
	)

	serveCmd, err := r.StartServe(ctx)
	if err != nil {
		return fmt.Errorf("start serve: %w", err)
	}

	defer func() {
		if serveCmd.Process != nil {
			_ = serveCmd.Process.Kill()
			_ = serveCmd.Wait()
			slog.Info("opencode serve stopped")
		}
	}()

	report, err := r.Run(ctx, prompt)
	if err != nil {
		return fmt.Errorf("opencode run: %w", err)
	}

	// 5. Write report.
	outputPath := cfg.Output.FilePath
	if !filepath.IsAbs(outputPath) {
		outputPath = filepath.Join(projectDir, outputPath)
	}

	if err := os.WriteFile(outputPath, []byte(report), 0o644); err != nil {
		return fmt.Errorf("write report: %w", err)
	}

	slog.Info("review complete", "report", outputPath)

	return nil
}

func buildPrompt(diff string, files []string) string {
	prompt := fmt.Sprintf(
		"Review the following code changes.\n\n"+
			"Changed files:\n%s\n\n"+
			"Diff:\n```\n%s\n```",
		strings.Join(files, "\n"),
		diff,
	)

	return prompt
}
