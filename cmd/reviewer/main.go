package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/alecthomas/kong"

	"github.com/aaudin90/opencode-reviewer/internal/agentconfig"
	"github.com/aaudin90/opencode-reviewer/internal/config"
	"github.com/aaudin90/opencode-reviewer/internal/configdir"
	"github.com/aaudin90/opencode-reviewer/internal/finalizerconfig"
	"github.com/aaudin90/opencode-reviewer/internal/git"
	"github.com/aaudin90/opencode-reviewer/internal/pipeline"
	"github.com/aaudin90/opencode-reviewer/internal/promptconfig"
	"github.com/aaudin90/opencode-reviewer/internal/providerconfig"
	"github.com/aaudin90/opencode-reviewer/internal/runner"
	"github.com/aaudin90/opencode-reviewer/internal/subagentconfig"
	"github.com/aaudin90/opencode-reviewer/internal/vcs"
	gitlabvcs "github.com/aaudin90/opencode-reviewer/internal/vcs/gitlab"
	"github.com/aaudin90/opencode-reviewer/internal/workspace"
)

type CLI struct {
	Config                        string `kong:"optional,type='path',placeholder='FILE',help='Path to TOML config file. If omitted, config-dir defaults or environment fallbacks are used.'"`
	ConfigDir                     string `kong:"optional,name='config-dir',type='path',placeholder='DIR',help='Path to config directory. If omitted, OR_CONFIG_DIR is used, then <project_dir or cwd>/.opencodereview auto-discovery.'"`
	DisableConfigDirAutoDiscovery bool   `kong:"optional,name='disable-config-dir-auto-discovery',help='Disable only the fallback auto-discovery of <project_dir or cwd>/.opencodereview. --config-dir and OR_CONFIG_DIR still work.'"`
	Branch                        string `kong:"placeholder='BRANCH',help='Branch to review (overrides OR_BRANCH env).'"`
	ReviewDump                    string `kong:"optional,name='review-dump',type='path',placeholder='FILE',help='Save final review JSON to FILE (for fast-path debugging).'"`
	FastReview                    string `kong:"optional,name='fast-review',type='path',placeholder='FILE',help='Skip LLM pipeline and load review from FILE (fast-path for debugging VCS).'"`
}

func main() {
	var cli CLI

	parser, err := kong.New(&cli,
		kong.Name("opencode-reviewer"),
		kong.Description(`Automated code review pipeline powered by OpenCode.

Use --config-dir/OR_CONFIG_DIR for file-based configuration, or --config for a TOML file.
Environment variables still override scalar TOML settings; provider and prompt env vars are deprecated fallbacks.

Config file (TOML) sections:

  project_dir               Path to the project repository (overridable via OR_PROJECT_DIR)

  [env]
    KEY = "VALUE"           Environment variables applied after TOML fields but before system env
                            takes effect. Values are set only when not already defined in system env.
                            Priority: system env > [env] > TOML fields.

  [opencode]
    endpoint                API endpoint URL (optional, connects to running instance)
    port                    Port for opencode subprocess (dynamic OS port if not set)
    model                   Optional LLM model override; prefer provider.json
    binary                  Path to opencode binary (default: opencode)
    stage_timeout           Timeout per stage in seconds (default: 600)
    max_steps               Max agent steps per session (default: 50)
    min_version             Minimum required opencode version (semver)
    provider_config_path    Path to provider JSON config (relative to config base dir or absolute)

  [git]
    remote                  Git remote name (default: origin)
    branch                  Branch to review
    base_branch             Base branch for diff (default: main)

  [pipeline]
    review_agent_prompt_path  Path to reviewer agent prompt file (relative to config base dir or absolute)
    review_agent_prompt       Inline reviewer agent prompt (alternative to path)
    review_message_paths      List of reviewer message files; each triggers a parallel session
    review_messages           Inline reviewer messages (alternative to paths)
    finalizer_prompt_path     Path to finalizer agent prompt file (relative to config base dir or absolute)
    finalizer_prompt          Inline finalizer agent prompt (alternative to path)
    finalizer_message_path    Path to finalizer user message file (relative to config base dir or absolute)
    finalizer_message         Inline finalizer user message (alternative to path)
    review_sub_agent_prompt_paths  Paths to reviewer sub-agent prompt files (relative to config base dir or absolute)
    review_sub_agent_prompts       Inline reviewer sub-agent prompts (alternative to paths)
    finalizer_sub_agent_prompt_paths  Paths to finalizer sub-agent prompt files (relative to config base dir or absolute)
    finalizer_sub_agent_prompts    Inline finalizer sub-agent prompts (alternative to paths)

  [gitlab]
    url                       GitLab instance URL (e.g. https://gitlab.example.com)
    token                     GitLab private access token
    project_id                Numeric GitLab project ID
    clear_comments            Delete open unanswered discussions before posting (default: false)

Environment variables:
  OR_CONFIG_DIR               Path to config directory (used by --config-dir flow and auto-default files)
  OR_DISABLE_CONFIG_DIR_AUTO_DISCOVERY  Disable only <project_dir or cwd>/.opencodereview fallback auto-discovery (true/1)
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
  OR_PROVIDER_CONFIG_PATH      Deprecated: path to provider JSON file (ignored when config-dir is active)
  OR_PROVIDER_CONFIG           Deprecated: inline provider JSON config (ignored when config-dir is active)
  OR_AGENT_PROMPT_PATH         Deprecated: path to reviewer agent prompt file (ignored when config-dir is active)
  OR_MESSAGE_PATHS             Deprecated: comma-separated paths to reviewer message files (ignored when config-dir is active)
  OR_FINALIZER_PROMPT_PATH     Deprecated: path to finalizer agent prompt file (ignored when config-dir is active)
  OR_FINALIZER_MESSAGE_PATH    Deprecated: path to finalizer user message file (ignored when config-dir is active)
  OR_REVIEW_SUB_AGENT_PROMPT_PATHS     Deprecated: comma-separated reviewer sub-agent prompt files (ignored when config-dir is active)
  OR_FINALIZER_SUB_AGENT_PROMPT_PATHS  Deprecated: comma-separated finalizer sub-agent prompt files (ignored when config-dir is active)
  OR_GITLAB_URL                GitLab instance URL (overrides gitlab.url)
  OR_GITLAB_TOKEN              GitLab private access token (overrides gitlab.token)
  OR_GITLAB_PROJECT_ID         Numeric GitLab project ID (overrides gitlab.project_id)
  OR_GITLAB_CLEAR_COMMENTS     Clear open MR discussions before posting (true/1 to enable)
  OR_SLOG_LEVEL                Log level: debug, info, warn, error (default: info)

Config directory files:
  provider.json
  reviewer/agent.md
  reviewer/messages/*.md
  reviewer/sub-agents/*.md
  reviewer/tools/*.ts          Optional reviewer tool overrides or custom tools
  finalizer/agent.md
  finalizer/message.md
  finalizer/sub-agents/*.md
  finalizer/tools/*.ts         Optional finalizer tool overrides or custom tools

Built-in tools submit_review.ts and submit_final_review.ts are available without files.

Debug flags:
  --review-dump FILE    Save final review JSON to FILE after LLM pipeline (for replay with --fast-review).
  --fast-review FILE    Skip LLM pipeline and load review from FILE (fast-path for iterating on VCS publishing).
  --disable-config-dir-auto-discovery
                        Disable only <project_dir or cwd>/.opencodereview fallback auto-discovery.

Priority (config directory):  --config-dir flag > OR_CONFIG_DIR env > <project_dir or cwd>/.opencodereview (if exists and auto-discovery is enabled).
Priority (branch):            --branch flag > OR_BRANCH env > git.branch TOML.
Priority (config files/prompts): config-dir files > TOML inline/path > deprecated OR_* env fallback (only when config-dir is inactive) > built-in default.`),
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
	configBaseDir := "."
	if cli.Config != "" {
		configBaseDir = filepath.Dir(cli.Config)
	}

	applyEnv(cfg.Env)
	config.ApplyEnvOverrides(cfg)

	disableAutoDiscovery := cli.DisableConfigDirAutoDiscovery || envBool("OR_DISABLE_CONFIG_DIR_AUTO_DISCOVERY")
	effectiveConfigDir, configSource, err := configdir.Resolve(cli.ConfigDir, os.Getenv("OR_CONFIG_DIR"), cfg.ProjectDir, disableAutoDiscovery)
	if err != nil {
		slog.Error("failed to resolve config directory", "error", err)
		os.Exit(1)
	}

	warnDeprecatedConfigEnv()

	if cli.Branch != "" {
		cfg.Git.Branch = cli.Branch
	}

	if cfg.Git.Branch == "" {
		slog.Error("branch is required: use --branch flag, OR_BRANCH env, or set git.branch in config")
		os.Exit(1)
	}

	initSlog()

	slog.Info("resolved configuration directory", "config_dir", effectiveConfigDir, "source", configSource) // #nosec G706 -- local path and controlled source label

	if err := config.ApplyConfigDirDefaults(cfg, effectiveConfigDir); err != nil {
		slog.Error("failed to apply config directory defaults", "error", err)
		os.Exit(1)
	}

	useLegacyEnvConfig := effectiveConfigDir == ""
	resolutionBaseDir := configBaseDir

	providerPath := resolveRelativePath(resolutionBaseDir, cfg.OpenCode.ProviderConfigPath)
	providerJSON, err := providerconfig.LoadWithOptions(providerPath, providerconfig.Options{
		UseLegacyEnv:      useLegacyEnvConfig,
		LegacyEnvFallback: useLegacyEnvConfig,
	})
	if err != nil {
		slog.Error("failed to load provider config", "error", err)
		os.Exit(1)
	}

	agentPath := resolveRelativePath(resolutionBaseDir, cfg.Pipeline.ReviewAgentPromptPath)
	agentPrompt, err := agentconfig.LoadWithOptions(agentPath, cfg.Pipeline.ReviewAgentPrompt, agentconfig.Options{
		UseLegacyEnv:      useLegacyEnvConfig,
		LegacyEnvFallback: useLegacyEnvConfig,
	})
	if err != nil {
		slog.Error("failed to load agent prompt", "error", err)
		os.Exit(1)
	}

	finalizerPath := resolveRelativePath(resolutionBaseDir, cfg.Pipeline.FinalizerPromptPath)
	finalizerPrompt, err := finalizerconfig.LoadWithOptions(finalizerPath, cfg.Pipeline.FinalizerPrompt, finalizerconfig.Options{
		UseLegacyEnv:      useLegacyEnvConfig,
		LegacyEnvFallback: useLegacyEnvConfig,
	})
	if err != nil {
		slog.Error("failed to load finalizer prompt", "error", err)
		os.Exit(1)
	}

	messages, err := promptconfig.LoadWithOptions(resolutionBaseDir, cfg.Pipeline.ReviewMessagePaths, cfg.Pipeline.ReviewMessages, promptconfig.Options{
		UseLegacyEnv:      useLegacyEnvConfig,
		LegacyEnvFallback: useLegacyEnvConfig,
	})
	if err != nil {
		slog.Error("failed to load review messages", "error", err)
		os.Exit(1)
	}

	reviewSubAgents, err := subagentconfig.LoadWithOptions(
		"OR_REVIEW_SUB_AGENT_PROMPT_PATHS", "reviewer",
		resolutionBaseDir,
		cfg.Pipeline.ReviewSubAgentPromptPaths,
		cfg.Pipeline.ReviewSubAgentPrompts,
		subagentconfig.Options{UseLegacyEnv: useLegacyEnvConfig, LegacyEnvFallback: useLegacyEnvConfig},
	)
	if err != nil {
		slog.Error("failed to load reviewer sub-agent prompts", "error", err)
		os.Exit(1)
	}

	finalizerMsgPath := resolveRelativePath(resolutionBaseDir, cfg.Pipeline.FinalizerMessagePath)
	finalizerMessage, err := finalizerconfig.LoadMessageWithOptions(finalizerMsgPath, cfg.Pipeline.FinalizerMessage, finalizerconfig.Options{
		UseLegacyEnv:      useLegacyEnvConfig,
		LegacyEnvFallback: useLegacyEnvConfig,
	})
	if err != nil {
		slog.Error("failed to load finalizer message", "error", err)
		os.Exit(1)
	}

	finalizerSubAgents, err := subagentconfig.LoadWithOptions(
		"OR_FINALIZER_SUB_AGENT_PROMPT_PATHS", "finalizer",
		resolutionBaseDir,
		cfg.Pipeline.FinalizerSubAgentPromptPaths,
		cfg.Pipeline.FinalizerSubAgentPrompts,
		subagentconfig.Options{UseLegacyEnv: useLegacyEnvConfig, LegacyEnvFallback: useLegacyEnvConfig},
	)
	if err != nil {
		slog.Error("failed to load finalizer sub-agent prompts", "error", err)
		os.Exit(1)
	}

	reviewerTools, err := loadConfigDirToolOverrides(effectiveConfigDir, "reviewer")
	if err != nil {
		slog.Error("failed to load reviewer tools", "error", err)
		os.Exit(1)
	}

	finalizerTools, err := loadConfigDirToolOverrides(effectiveConfigDir, "finalizer")
	if err != nil {
		slog.Error("failed to load finalizer tools", "error", err)
		os.Exit(1)
	}

	reviewerWS, err := workspace.NewReviewer(workspace.Config{
		ProviderJSON:  providerJSON,
		Model:         cfg.OpenCode.Model,
		MaxSteps:      cfg.OpenCode.MaxSteps,
		SubAgents:     reviewSubAgents,
		ToolOverrides: reviewerTools,
	}, agentPrompt)
	if err != nil {
		slog.Error("failed to create reviewer workspace", "error", err)
		os.Exit(1)
	}
	defer func() { _ = reviewerWS.Cleanup() }()

	finalizerWS, err := workspace.NewFinalizer(workspace.Config{
		ProviderJSON:  providerJSON,
		Model:         cfg.OpenCode.Model,
		MaxSteps:      cfg.OpenCode.MaxSteps,
		SubAgents:     finalizerSubAgents,
		ToolOverrides: finalizerTools,
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

var deprecatedConfigEnvKeys = []string{
	"OR_PROVIDER_CONFIG_PATH",
	"OR_PROVIDER_CONFIG",
	"OR_AGENT_PROMPT_PATH",
	"OR_MESSAGE_PATHS",
	"OR_FINALIZER_PROMPT_PATH",
	"OR_FINALIZER_MESSAGE_PATH",
	"OR_REVIEW_SUB_AGENT_PROMPT_PATHS",
	"OR_FINALIZER_SUB_AGENT_PROMPT_PATHS",
}

func warnDeprecatedConfigEnv() {
	keys := activeDeprecatedConfigEnv()
	if len(keys) == 0 {
		return
	}
	fmt.Fprintf(
		os.Stderr,
		"\033[31mwarning: deprecated configuration env vars are set and will be removed soon: %s. Use --config-dir or OR_CONFIG_DIR instead.\033[0m\n",
		strings.Join(keys, ", "),
	)
}

func activeDeprecatedConfigEnv() []string {
	keys := make([]string, 0, len(deprecatedConfigEnvKeys))
	for _, key := range deprecatedConfigEnvKeys {
		if os.Getenv(key) != "" {
			keys = append(keys, key)
		}
	}
	return keys
}

func envBool(key string) bool {
	val := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	return val == "true" || val == "1"
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
