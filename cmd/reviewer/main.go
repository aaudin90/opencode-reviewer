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
	"github.com/aaudin90/opencode-reviewer/internal/promptconfig"
	"github.com/aaudin90/opencode-reviewer/internal/providerconfig"
	"github.com/aaudin90/opencode-reviewer/internal/runner"
	"github.com/aaudin90/opencode-reviewer/internal/workspace"
)

type CLI struct {
	Config string `kong:"optional,type='path',placeholder='FILE',help='Path to TOML config file. If omitted, all settings must be provided via environment variables.'"`
	Branch string `kong:"placeholder='BRANCH',help='Branch to review (overrides REVIEW_BRANCH env).'"`
}

func main() {
	var cli CLI

	parser, err := kong.New(&cli,
		kong.Name("opencode-reviewer"),
		kong.Description(`Automated code review pipeline powered by OpenCode.

Config file is optional: all settings can be provided via environment variables.

Config file (TOML) sections:

  project_dir               Path to the project repository (overridable via REVIEW_PROJECT_DIR)

  [env]
    KEY = "VALUE"           Environment variables applied after TOML fields but before system env
                            takes effect. Values are set only when not already defined in system env.
                            Priority: system env > [env] > TOML fields.

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
    prompt_paths            List of prompt files (relative to config file or absolute).
                            Each file triggers a separate parallel review session.
                            If not set, the built-in default review prompt is used.

Environment variables (override TOML values):
  REVIEW_PROJECT_DIR               Path to the project repository (overrides project_dir)
  REVIEW_BRANCH                    Branch to review (overridden by --branch flag)
  REVIEW_GIT_REMOTE                Git remote name (overrides git.remote)
  REVIEW_GIT_BASE_BRANCH           Base branch for diff (overrides git.base_branch)
  REVIEW_OPENCODE_ENDPOINT         opencode API endpoint URL (overrides opencode.endpoint)
  REVIEW_OPENCODE_PORT             opencode port (overrides opencode.port)
  REVIEW_OPENCODE_MODEL            LLM model identifier (overrides opencode.model)
  REVIEW_OPENCODE_BINARY           Path to opencode binary (overrides opencode.binary)
  REVIEW_OPENCODE_STAGE_TIMEOUT    Timeout per stage in seconds (overrides opencode.stage_timeout)
  REVIEW_OPENCODE_MAX_STEPS        Max agent steps per session (overrides opencode.max_steps)
  REVIEW_OPENCODE_MIN_VERSION      Minimum opencode version (overrides opencode.min_version)
  REVIEW_PROVIDER_CONFIG_PATH      Path to provider JSON file (overrides opencode.provider_config_path)
  REVIEW_PROVIDER_CONFIG           Inline provider JSON config
  REVIEW_AGENT_CONFIG_PATH         Path to agent prompt file (overrides pipeline.agent_config_path)
  REVIEW_AGENT_CONFIG              Inline agent prompt or JSON with "prompt" field
  REVIEW_PROMPT_PATHS              Comma-separated paths to prompt files (overrides pipeline.prompt_paths)

Priority: --branch flag > REVIEW_BRANCH env > git.branch TOML.
Priority (provider): REVIEW_PROVIDER_CONFIG_PATH > REVIEW_PROVIDER_CONFIG > TOML path.
Priority (agent):    REVIEW_AGENT_CONFIG_PATH > REVIEW_AGENT_CONFIG > TOML path > built-in default.
Priority (prompts):  REVIEW_PROMPT_PATHS > pipeline.prompt_paths TOML > built-in default.`),
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
		slog.Error("branch is required: use --branch flag, REVIEW_BRANCH env, or set git.branch in config")
		os.Exit(1)
	}

	applyEnv(cfg.Env)
	config.ApplyEnvOverrides(cfg)
	initSlog()

	configDir := "."
	if cli.Config != "" {
		configDir = filepath.Dir(cli.Config)
	}

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

	promptPaths, err := promptconfig.Load(configDir, cfg.Pipeline.PromptPaths)
	if err != nil {
		slog.Error("failed to load prompt paths", "error", err)
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

	if err := run(cfg, ws, promptPaths); err != nil {
		slog.Error("review failed", "error", err)
		os.Exit(1)
	}
}

func run(cfg *config.Config, ws *workspace.Workspace, promptPaths []string) error {
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

	p := pipeline.New(gitClient, cfg.Git.Branch, cfg.Git.BaseBranch, ocRunner, promptPaths)

	ctx := context.Background()
	_, err = p.Run(ctx)
	return err
}

// initSlog configures the global slog logger.
// Level is read from SLOG_LEVEL env var (e.g. "debug", "info", "warn", "error").
// Defaults to info if unset or invalid.
func initSlog() {
	level := slog.LevelInfo
	if val := os.Getenv("SLOG_LEVEL"); val != "" {
		if err := level.UnmarshalText([]byte(val)); err != nil {
			slog.Warn("invalid SLOG_LEVEL, defaulting to info", "value", val) // #nosec G706 -- env var, not user input
		}
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))
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
