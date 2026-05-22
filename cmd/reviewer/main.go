package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/alecthomas/kong"

	"github.com/aaudin90/opencode-reviewer/internal/review/pipeline"
	reviewruntime "github.com/aaudin90/opencode-reviewer/internal/review/runtime"
	"github.com/aaudin90/opencode-reviewer/internal/shared/config"
	"github.com/aaudin90/opencode-reviewer/internal/shared/git"
	"github.com/aaudin90/opencode-reviewer/internal/shared/vcs"
	gitlabvcs "github.com/aaudin90/opencode-reviewer/internal/shared/vcs/gitlab"
)

type CLI struct {
	Config                        string `kong:"optional,type='path',placeholder='FILE',help='Path to TOML config file. If omitted, config-dir defaults or environment fallbacks are used.'"`
	ConfigDir                     string `kong:"optional,name='config-dir',type='path',placeholder='DIR',help='Path to config directory. If omitted, OR_CONFIG_DIR is used, then <project_dir or cwd>/.opencodereview auto-discovery after checkout.'"`
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
File-based configuration is resolved after the target branch is checked out.
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
    model                   Optional LLM model override in provider/model format; prefer provider.json
    fallback_models         Optional fallback model chain after the primary provider/model
    binary                  Path to opencode binary (default: opencode)
    stage_timeout           Timeout per stage in seconds (default: 600)
    max_steps               Max agent steps per session (default: 50)
    min_version             Minimum required opencode version (semver)
    print_logs              Pass --print-logs to opencode serve
    log_level               Pass --log-level to opencode serve (DEBUG, INFO, WARN, ERROR)
    log_dir                 Directory for OpenCode stdout/stderr log files when print_logs is enabled
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
  OR_CONFIG_DIR               Path to config directory (used after checkout when --config-dir is not set)
  OR_DISABLE_CONFIG_DIR_AUTO_DISCOVERY  Disable only <project_dir or cwd>/.opencodereview fallback auto-discovery (true/1)
  OR_PROJECT_DIR               Path to the project repository (overrides project_dir)
  OR_BRANCH                    Branch to review (overridden by --branch flag)
  OR_GIT_REMOTE                Git remote name (overrides git.remote)
  OR_GIT_BASE_BRANCH           Base branch for diff (overrides git.base_branch)
  OR_OPENCODE_ENDPOINT         opencode API endpoint URL (overrides opencode.endpoint)
  OR_OPENCODE_PORT             opencode port (overrides opencode.port)
  OR_OPENCODE_MODEL            LLM model identifier (overrides opencode.model)
  OR_OPENCODE_FALLBACK_MODELS  Comma-separated fallback models in provider/model format
  OR_OPENCODE_BINARY           Path to opencode binary (overrides opencode.binary)
  OR_OPENCODE_STAGE_TIMEOUT    Timeout per stage in seconds (overrides opencode.stage_timeout)
  OR_OPENCODE_MAX_STEPS        Max agent steps per session (overrides opencode.max_steps)
  OR_OPENCODE_MIN_VERSION      Minimum opencode version (overrides opencode.min_version)
  OR_OPENCODE_PRINT_LOGS       Pass --print-logs to opencode serve (true/1)
  OR_OPENCODE_LOG_LEVEL        Pass --log-level to opencode serve (DEBUG, INFO, WARN, ERROR)
  OR_OPENCODE_LOG_DIR          OpenCode log directory (default: opencode-review-logs)
  OR_OPENCODE_KEEP_XDG_DIRS    Keep temporary OpenCode XDG dirs after run for debugging (true/1)
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

Priority (config directory):  --config-dir flag > OR_CONFIG_DIR env > <project_dir or cwd>/.opencodereview after checkout (if exists and auto-discovery is enabled).
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

	warnDeprecatedConfigEnv()

	if cli.Branch != "" {
		cfg.Git.Branch = cli.Branch
	}

	if cfg.Git.Branch == "" {
		slog.Error("branch is required: use --branch flag, OR_BRANCH env, or set git.branch in config")
		os.Exit(1)
	}

	initSlog()

	if err := run(cfg, cli, configBaseDir); err != nil {
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

func run(cfg *config.Config, cli CLI, configBaseDir string) error {
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
	publisher := buildPublisher(cfg)
	runtimeLoader := reviewruntime.New(reviewruntime.Config{
		AppConfig:                     cfg,
		ConfigBaseDir:                 configBaseDir,
		CLIConfigDir:                  cli.ConfigDir,
		DisableConfigDirAutoDiscovery: cli.DisableConfigDirAutoDiscovery || envBool("OR_DISABLE_CONFIG_DIR_AUTO_DISCOVERY"),
	})

	p := pipeline.New(pipeline.Config{
		GitClient:      gitClient,
		Branch:         cfg.Git.Branch,
		BaseBranch:     cfg.Git.BaseBranch,
		RuntimeLoader:  runtimeLoader,
		Publisher:      publisher,
		ReviewDumpPath: cli.ReviewDump,
		FastReviewPath: cli.FastReview,
	})

	ctx := context.Background()
	_, err = p.Run(ctx)
	printOpenCodeLogPaths(p.LogPaths())
	return err
}

func printOpenCodeLogPaths(paths []string) {
	if len(paths) == 0 {
		return
	}
	fmt.Fprintln(os.Stdout, "OpenCode log files:")
	for _, path := range paths {
		fmt.Fprintln(os.Stdout, path)
	}
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
