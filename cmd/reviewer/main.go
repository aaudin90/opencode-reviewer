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
	"github.com/aaudin90/opencode-reviewer/internal/finalizerconfig"
	"github.com/aaudin90/opencode-reviewer/internal/git"
	"github.com/aaudin90/opencode-reviewer/internal/pipeline"
	"github.com/aaudin90/opencode-reviewer/internal/promptconfig"
	"github.com/aaudin90/opencode-reviewer/internal/providerconfig"
	"github.com/aaudin90/opencode-reviewer/internal/runner"
	"github.com/aaudin90/opencode-reviewer/internal/vcs"
	gitlabvcs "github.com/aaudin90/opencode-reviewer/internal/vcs/gitlab"
	"github.com/aaudin90/opencode-reviewer/internal/workspace"
)

type CLI struct {
	Config     string `kong:"optional,type='path',placeholder='FILE',help='Path to TOML config file. If omitted, all settings must be provided via environment variables.'"`
	Branch     string `kong:"placeholder='BRANCH',help='Branch to review (overrides OR_BRANCH env).'"`
	ReviewDump string `kong:"optional,name='review-dump',type='path',placeholder='FILE',help='Save final review JSON to FILE (for fast-path debugging).'"`
	FastReview string `kong:"optional,name='fast-review',type='path',placeholder='FILE',help='Skip LLM pipeline and load review from FILE (fast-path for debugging VCS).'"`
}

func main() {
	var cli CLI

	parser, err := kong.New(&cli,
		kong.Name("opencode-reviewer"),
		kong.Description(`Automated code review pipeline powered by OpenCode.

Config file is optional: all settings can be provided via environment variables.

Config file (TOML) sections:

  project_dir               Path to the project repository (overridable via OR_PROJECT_DIR)

  [env]
    KEY = "VALUE"           Environment variables applied after TOML fields but before system env
                            takes effect. Values are set only when not already defined in system env.
                            Priority: system env > [env] > TOML fields.

  [opencode]
    endpoint                API endpoint URL (optional, connects to running instance)
    port                    Port for opencode subprocess (dynamic OS port if not set)
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
    review_agent_prompt_path  Path to reviewer agent prompt file (relative to config file or absolute)
    review_agent_prompt       Inline reviewer agent prompt (alternative to path)
    review_message_paths      List of reviewer message files; each triggers a parallel session
    review_messages           Inline reviewer messages (alternative to paths)
    finalizer_prompt_path     Path to finalizer agent prompt file (relative to config file or absolute)
    finalizer_prompt          Inline finalizer agent prompt (alternative to path)
    finalizer_message_path    Path to finalizer user message file (relative to config file or absolute)
    finalizer_message         Inline finalizer user message (alternative to path)

  [gitlab]
    url                       GitLab instance URL (e.g. https://gitlab.example.com)
    token                     GitLab private access token
    project_id                Numeric GitLab project ID
    clear_comments            Delete open unanswered discussions before posting (default: false)

Environment variables (override TOML values):
  OR_PROJECT_DIR               Path to the project repository (overrides project_dir)
  OR_BRANCH                    Branch to review (overridden by --branch flag)
  OR_GIT_REMOTE                Git remote name (overrides git.remote)
  OR_GIT_BASE_BRANCH           Base branch for diff (overrides git.base_branch)
  OR_OPENCODE_ENDPOINT         opencode API endpoint URL (overrides opencode.endpoint)
  OR_OPENCODE_PORT             opencode port (overrides opencode.port)
  OR_OPENCODE_MODEL            LLM model identifier (overrides opencode.model)
  OR_OPENCODE_BINARY           Path to opencode binary (overrides opencode.binary)
  OR_OPENCODE_STAGE_TIMEOUT    Timeout per stage in seconds (overrides opencode.stage_timeout)
  OR_OPENCODE_MAX_STEPS        Max agent steps per session (overrides opencode.max_steps)
  OR_OPENCODE_MIN_VERSION      Minimum opencode version (overrides opencode.min_version)
  OR_PROVIDER_CONFIG_PATH      Path to provider JSON file (overrides opencode.provider_config_path)
  OR_PROVIDER_CONFIG           Inline provider JSON config
  OR_AGENT_PROMPT_PATH         Path to reviewer agent prompt file (overrides pipeline.review_agent_prompt_path)
  OR_MESSAGE_PATHS             Comma-separated paths to reviewer message files (overrides pipeline.review_message_paths)
  OR_FINALIZER_PROMPT_PATH     Path to finalizer agent prompt file (overrides pipeline.finalizer_prompt_path)
  OR_FINALIZER_MESSAGE_PATH    Path to finalizer user message file (overrides pipeline.finalizer_message_path)
  OR_GITLAB_URL                GitLab instance URL (overrides gitlab.url)
  OR_GITLAB_TOKEN              GitLab private access token (overrides gitlab.token)
  OR_GITLAB_PROJECT_ID         Numeric GitLab project ID (overrides gitlab.project_id)
  OR_GITLAB_CLEAR_COMMENTS     Clear open MR discussions before posting (true/1 to enable)

Debug flags:
  --review-dump FILE    Save final review JSON to FILE after LLM pipeline (for replay with --fast-review).
  --fast-review FILE    Skip LLM pipeline and load review from FILE (fast-path for iterating on VCS publishing).

Priority (branch):            --branch flag > OR_BRANCH env > git.branch TOML.
Priority (provider):          OR_PROVIDER_CONFIG_PATH > OR_PROVIDER_CONFIG > TOML path.
Priority (agent prompt):      OR_AGENT_PROMPT_PATH > review_agent_prompt TOML > review_agent_prompt_path TOML > built-in default.
Priority (messages):          OR_MESSAGE_PATHS > review_messages TOML > review_message_paths TOML > (none).
Priority (finalizer prompt):  OR_FINALIZER_PROMPT_PATH > finalizer_prompt TOML > finalizer_prompt_path TOML > built-in default.
Priority (finalizer message): OR_FINALIZER_MESSAGE_PATH > finalizer_message TOML > finalizer_message_path TOML > built-in default.`),
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
		slog.Error("branch is required: use --branch flag, OR_BRANCH env, or set git.branch in config")
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

	agentPath := resolveRelativePath(configDir, cfg.Pipeline.ReviewAgentPromptPath)
	agentPrompt, err := agentconfig.Load(agentPath, cfg.Pipeline.ReviewAgentPrompt)
	if err != nil {
		slog.Error("failed to load agent prompt", "error", err)
		os.Exit(1)
	}

	finalizerPath := resolveRelativePath(configDir, cfg.Pipeline.FinalizerPromptPath)
	finalizerPrompt, err := finalizerconfig.Load(finalizerPath, cfg.Pipeline.FinalizerPrompt)
	if err != nil {
		slog.Error("failed to load finalizer prompt", "error", err)
		os.Exit(1)
	}

	messages, err := promptconfig.Load(configDir, cfg.Pipeline.ReviewMessagePaths, cfg.Pipeline.ReviewMessages)
	if err != nil {
		slog.Error("failed to load review messages", "error", err)
		os.Exit(1)
	}

	finalizerMsgPath := resolveRelativePath(configDir, cfg.Pipeline.FinalizerMessagePath)
	finalizerMessage, err := finalizerconfig.LoadMessage(finalizerMsgPath, cfg.Pipeline.FinalizerMessage)
	if err != nil {
		slog.Error("failed to load finalizer message", "error", err)
		os.Exit(1)
	}

	reviewerWS, err := workspace.NewReviewer(workspace.Config{
		ProviderJSON: providerJSON,
		Model:        cfg.OpenCode.Model,
		MaxSteps:     cfg.OpenCode.MaxSteps,
	}, agentPrompt)
	if err != nil {
		slog.Error("failed to create reviewer workspace", "error", err)
		os.Exit(1)
	}
	defer func() { _ = reviewerWS.Cleanup() }()

	finalizerWS, err := workspace.NewFinalizer(workspace.Config{
		ProviderJSON: providerJSON,
		Model:        cfg.OpenCode.Model,
		MaxSteps:     cfg.OpenCode.MaxSteps,
	}, finalizerPrompt)
	if err != nil {
		slog.Error("failed to create finalizer workspace", "error", err)
		os.Exit(1)
	}
	defer func() { _ = finalizerWS.Cleanup() }()

	if err := runner.ValidateBinary(cfg.OpenCode); err != nil {
		slog.Error("opencode is not available", "error", err)
		os.Exit(1)
	}

	if err := run(cfg, reviewerWS, finalizerWS, messages, finalizerMessage, cli.ReviewDump, cli.FastReview); err != nil {
		slog.Error("review failed", "error", err)
		os.Exit(1)
	}
}

// buildPublisher constructs a GitLab publisher when all required
// gitlab config fields are set. Returns nil otherwise (publishing skipped).
func buildPublisher(cfg *config.Config) vcs.Publisher {
	gl := cfg.GitLab
	if gl.URL == "" || gl.Token == "" || gl.ProjectID == 0 {
		return nil
	}
	client := gitlabvcs.NewClient(gitlabvcs.Config{
		URL:       gl.URL,
		Token:     gl.Token,
		ProjectID: gl.ProjectID,
	})
	return gitlabvcs.NewPublisher(client, gl.ClearComments)
}

func run(cfg *config.Config, reviewerWS, finalizerWS *workspace.Workspace, messages []string, finalizerMessage string, reviewDump, fastReview string) error {
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
	reviewerRunner := runner.New(cfg.OpenCode, projectDir, reviewerWS)
	finalizerRunner := runner.New(cfg.OpenCode, projectDir, finalizerWS)

	publisher := buildPublisher(cfg)

	p := pipeline.New(pipeline.Config{
		GitClient:        gitClient,
		Branch:           cfg.Git.Branch,
		BaseBranch:       cfg.Git.BaseBranch,
		Runner:           reviewerRunner,
		FinalizerRunner:  finalizerRunner,
		Messages:         messages,
		FinalizerMessage: finalizerMessage,
		Publisher:        publisher,
		ReviewDumpPath:   reviewDump,
		FastReviewPath:   fastReview,
	})

	ctx := context.Background()
	_, err = p.Run(ctx)
	return err
}

// initSlog configures the global slog logger.
// Level is read from OR_SLOG_LEVEL env var (e.g. "debug", "info", "warn", "error").
// Defaults to info if unset or invalid.
func initSlog() {
	level := slog.LevelInfo
	if val := os.Getenv("OR_SLOG_LEVEL"); val != "" {
		if err := level.UnmarshalText([]byte(val)); err != nil {
			slog.Warn("invalid OR_SLOG_LEVEL, defaulting to info", "value", val) // #nosec G706 -- env var, not user input
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
