package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/alecthomas/kong"

	"github.com/aaudin90/opencode-reviewer/internal/agentconfig"
	"github.com/aaudin90/opencode-reviewer/internal/config"
	"github.com/aaudin90/opencode-reviewer/internal/git"
	"github.com/aaudin90/opencode-reviewer/internal/pipeline"
	"github.com/aaudin90/opencode-reviewer/internal/providerconfig"
	"github.com/aaudin90/opencode-reviewer/internal/runner"
	"github.com/aaudin90/opencode-reviewer/internal/workspace"
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

  project_dir               Path to the project repository (required)

  [env]
    KEY = "VALUE"           Environment variables (set if not already defined)

  [opencode]
    endpoint                API endpoint URL (optional, connects to running instance)
    port                    API port (default: 4096)
    model                   LLM model identifier (e.g. llm-proxy/kimi-k2.5)
    binary                  Path to opencode binary (default: opencode)
    stage_timeout           Timeout per stage in seconds (default: 600)
    max_steps               Max agent steps per session (default: 30)
    provider_config_path    Path to provider JSON config (relative to config file or absolute)

  [git]
    remote                  Git remote name (default: origin)
    branch                  Branch to review
    base_branch             Base branch for diff (default: main)

  [pipeline]
    agent_config_path       Path to agent prompt file (relative to config file or absolute)
                            If not set, the built-in default prompt is used.

  [output]
    file_path               Report output path (default: review-report.md)
    format_project_dir      Project dir for path formatting in report

Environment variables (override TOML values):
  REVIEW_BRANCH                Branch to review (overridden by --branch flag)
  REVIEW_PROVIDER_CONFIG_PATH  Path to provider JSON file (overrides provider_config_path)
  REVIEW_PROVIDER_CONFIG       Inline provider JSON config
  REVIEW_AGENT_CONFIG_PATH     Path to agent prompt file (overrides agent_config_path)
  REVIEW_AGENT_CONFIG          Inline agent prompt (overrides agent_config_path)

Priority: env file path > env inline > TOML path > built-in default (for agent config).
Priority: env file path > env inline > TOML path (for provider config).`),
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

	applyEnv(cfg.Env)

	configDir := filepath.Dir(cli.Config)

	providerPath := resolveRelativePath(configDir, cfg.OpenCode.ProviderConfigPath)
	providerJSON, err := providerconfig.Load(providerPath)
	if err != nil {
		slog.Error("failed to load provider config", "error", err)
		os.Exit(1)
	}

	agentPath := resolveRelativePath(configDir, cfg.Pipeline.AgentConfigPath)
	agentPrompt, err := agentconfig.Load(agentPath)
	if err != nil {
		slog.Error("failed to load agent config", "error", err)
		os.Exit(1)
	}

	ws, err := workspace.New(workspace.Config{
		ProviderJSON: providerJSON,
		Model:        cfg.OpenCode.Model,
		MaxSteps:     cfg.OpenCode.MaxSteps,
		AgentPrompt:  agentPrompt,
	})
	if err != nil {
		slog.Error("failed to create workspace", "error", err)
		os.Exit(1)
	}
	defer func() { _ = ws.Cleanup() }()

	if err := runner.ValidateBinary(cfg.OpenCode); err != nil {
		slog.Error("opencode is not available", "error", err)
		os.Exit(1)
	}

	if err := run(cfg, ws); err != nil {
		slog.Error("review failed", "error", err)
		os.Exit(1)
	}
}

func run(cfg *config.Config, ws *workspace.Workspace) error {
	projectDir, err := filepath.Abs(cfg.ProjectDir)
	if err != nil {
		return fmt.Errorf("resolve project dir: %w", err)
	}

	slog.Info("starting review",
		"branch", cfg.Git.Branch,
		"base", cfg.Git.BaseBranch,
		"project", projectDir,
	)

	gitClient := git.NewClient(projectDir, cfg.Git.Remote)
	ocRunner := runner.New(cfg.OpenCode, projectDir, ws)

	outputPath := cfg.Output.FilePath
	if !filepath.IsAbs(outputPath) {
		outputPath = filepath.Join(projectDir, outputPath)
	}

	p := pipeline.New(gitClient, cfg.Git.Branch, cfg.Git.BaseBranch,
		ocRunner, outputPath)

	ctx := context.Background()
	_, err = p.Run(ctx)
	return err
}

// applyEnv sets environment variables from the [env] TOML section.
// Existing env vars are not overwritten.
func applyEnv(env map[string]string) {
	for key, val := range env {
		if os.Getenv(key) == "" {
			if err := os.Setenv(key, val); err != nil {
				slog.Warn("failed to set env var", "key", key, "error", err)
			}
		}
	}
}

// resolveRelativePath resolves a path relative to baseDir if it is not absolute.
// Returns empty string if path is empty.
func resolveRelativePath(baseDir, path string) string {
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(baseDir, path)
}
