package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"

	"github.com/alecthomas/kong"

	"github.com/aaudin90/opencode-reviewer/internal/commentwarrior"
	commentwarriorruntime "github.com/aaudin90/opencode-reviewer/internal/commentwarrior/runtime"
	"github.com/aaudin90/opencode-reviewer/internal/shared/config"
	"github.com/aaudin90/opencode-reviewer/internal/shared/git"
	gitlabvcs "github.com/aaudin90/opencode-reviewer/internal/shared/vcs/gitlab"
)

type CLI struct {
	Config                        string `kong:"optional,type='path',placeholder='FILE',help='Path to TOML config file.'"`
	ConfigDir                     string `kong:"optional,name='config-dir',type='path',placeholder='DIR',help='Path to config directory.'"`
	Branch                        string `kong:"placeholder='BRANCH',help='Branch to checkout.'"`
	MRIID                         int    `kong:"optional,name='mr-iid',help='GitLab merge request IID.'"`
	DryRun                        bool   `kong:"optional,name='dry-run',help='Run agent but do not mutate GitLab.'"`
	MaxDiscussions                int    `kong:"optional,name='max-discussions',default='0',help='Maximum discussions to process; 0 means no limit.'"`
	DiscussionID                  string `kong:"optional,name='discussion-id',help='Process a single discussion ID.'"`
	DisableConfigDirAutoDiscovery bool   `kong:"optional,name='disable-config-dir-auto-discovery',help='Disable .opencodereview auto-discovery.'"`
}

func main() {
	var cli CLI
	parser, err := kong.New(&cli,
		kong.Name("opencode-reviewer-comment-warrior"),
		kong.Description(`GitLab discussion follow-up agent for opencode-reviewer.

Flow:
  load scalar TOML/env config
  prepare the target branch
  lazily load .opencodereview/comment-warrior files
  list GitLab discussions
  run one OpenCode session per selected discussion
  optionally reply, resolve, or unresolve

Config directory files:
  provider.json
  comment-warrior/agent.md
  comment-warrior/finding-message.md
  comment-warrior/mention-message.md
  comment-warrior/sub-agents/*.md
  comment-warrior/tools/*.ts

Environment variables:
  OR_CONFIG_DIR                         Config directory used after checkout
  OR_DISABLE_CONFIG_DIR_AUTO_DISCOVERY  Disable .opencodereview auto-discovery (true/1)
  OR_BRANCH                             Branch; overridden by --branch
  CI_MERGE_REQUEST_SOURCE_BRANCH_NAME   CI fallback branch
  OR_COMMENT_WARRIOR_MR_IID             MR IID; overridden by --mr-iid
  CI_MERGE_REQUEST_IID                  CI fallback MR IID
  OR_COMMENT_WARRIOR_AGENT_PROMPT_PATH  Deprecated fallback when config-dir is inactive
  OR_COMMENT_WARRIOR_FINDING_MESSAGE_PATH  Deprecated fallback finding message path
  OR_COMMENT_WARRIOR_MENTION_MESSAGE_PATH  Deprecated fallback mention message path

Priority (branch): --branch > OR_BRANCH > CI_MERGE_REQUEST_SOURCE_BRANCH_NAME > git.branch TOML.
Priority (MR IID): --mr-iid > OR_COMMENT_WARRIOR_MR_IID > CI_MERGE_REQUEST_IID.`),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	_, err = parser.Parse(os.Args[1:])
	if err != nil {
		parser.FatalIfErrorf(err)
	}
	initSlog()

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
	applyBranchPriority(cfg, cli.Branch)
	mrIID := resolveMRIID(cli.MRIID)

	if cfg.Git.Branch == "" {
		slog.Error("branch is required")
		os.Exit(1)
	}
	if mrIID == 0 {
		slog.Error("MR IID is required")
		os.Exit(1)
	}
	if cfg.GitLab.URL == "" || cfg.GitLab.Token == "" || cfg.GitLab.ProjectID == 0 {
		slog.Error("GitLab url/token/project_id are required")
		os.Exit(1)
	}
	if err := run(context.Background(), cfg, cli, configBaseDir, mrIID); err != nil {
		slog.Error("comment-warrior failed", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg *config.Config, cli CLI, configBaseDir string, mrIID int) error {
	projectDir, err := filepath.Abs(cfg.ProjectDir)
	if err != nil {
		return fmt.Errorf("resolve project dir: %w", err)
	}
	gitClient := git.NewClient(projectDir, cfg.Git.Remote)
	client := gitlabvcs.NewClient(gitlabvcs.Config{URL: cfg.GitLab.URL, Token: cfg.GitLab.Token, ProjectID: cfg.GitLab.ProjectID})
	loader := commentwarriorruntime.New(commentwarriorruntime.Config{
		AppConfig:                     cfg,
		ConfigBaseDir:                 configBaseDir,
		CLIConfigDir:                  cli.ConfigDir,
		DisableConfigDirAutoDiscovery: cli.DisableConfigDirAutoDiscovery || envBool("OR_DISABLE_CONFIG_DIR_AUTO_DISCOVERY"),
	})
	pipeline := commentwarrior.NewPipeline(commentwarrior.PipelineConfig{
		ProjectDir:     projectDir,
		Branch:         cfg.Git.Branch,
		BaseBranch:     cfg.Git.BaseBranch,
		MRIID:          mrIID,
		DryRun:         cli.DryRun,
		MaxDiscussions: cli.MaxDiscussions,
		DiscussionID:   cli.DiscussionID,
	}, gitClient, client, loader)
	return pipeline.Run(ctx)
}

func applyBranchPriority(cfg *config.Config, flag string) {
	switch {
	case flag != "":
		cfg.Git.Branch = flag
	case os.Getenv("OR_BRANCH") != "":
		cfg.Git.Branch = os.Getenv("OR_BRANCH")
	case os.Getenv("CI_MERGE_REQUEST_SOURCE_BRANCH_NAME") != "":
		cfg.Git.Branch = os.Getenv("CI_MERGE_REQUEST_SOURCE_BRANCH_NAME")
	}
}

func resolveMRIID(flag int) int {
	if flag != 0 {
		return flag
	}
	for _, key := range []string{"OR_COMMENT_WARRIOR_MR_IID", "CI_MERGE_REQUEST_IID"} {
		if raw := os.Getenv(key); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil {
				return n
			}
		}
	}
	return 0
}

func applyEnv(env map[string]string) {
	for k, v := range env {
		if os.Getenv(k) == "" {
			_ = os.Setenv(k, v)
		}
	}
}

func envBool(key string) bool {
	v := os.Getenv(key)
	return v == "true" || v == "1"
}

func initSlog() {
	level := slog.LevelInfo
	if val := os.Getenv("OR_SLOG_LEVEL"); val != "" {
		_ = level.UnmarshalText([]byte(val))
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))
}
