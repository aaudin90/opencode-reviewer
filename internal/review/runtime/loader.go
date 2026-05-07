package reviewruntime

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/aaudin90/opencode-reviewer/internal/review/agentconfig"
	"github.com/aaudin90/opencode-reviewer/internal/review/finalizerconfig"
	"github.com/aaudin90/opencode-reviewer/internal/review/pipeline"
	"github.com/aaudin90/opencode-reviewer/internal/review/promptconfig"
	"github.com/aaudin90/opencode-reviewer/internal/shared/config"
	"github.com/aaudin90/opencode-reviewer/internal/shared/configdir"
	"github.com/aaudin90/opencode-reviewer/internal/shared/providerconfig"
	"github.com/aaudin90/opencode-reviewer/internal/shared/runner"
	"github.com/aaudin90/opencode-reviewer/internal/shared/subagentconfig"
	"github.com/aaudin90/opencode-reviewer/internal/shared/workspace"
)

type Loader struct {
	cfg Config
}

func New(cfg Config) *Loader {
	return &Loader{cfg: cfg}
}

func (l *Loader) LoadRuntime(_ context.Context) (*pipeline.RuntimeResources, error) {
	if l.cfg.AppConfig == nil {
		return nil, fmt.Errorf("app config is nil")
	}

	cfg := *l.cfg.AppConfig
	effectiveConfigDir, configSource, err := configdir.Resolve(
		l.cfg.CLIConfigDir,
		os.Getenv("OR_CONFIG_DIR"),
		cfg.ProjectDir,
		l.cfg.DisableConfigDirAutoDiscovery,
	)
	if err != nil {
		return nil, fmt.Errorf("resolve config directory: %w", err)
	}

	slog.Info("resolved configuration directory after checkout", "config_dir", effectiveConfigDir, "source", configSource) // #nosec G706 -- local path and controlled source label

	if err := config.ApplyConfigDirDefaults(&cfg, effectiveConfigDir); err != nil {
		return nil, fmt.Errorf("apply config directory defaults: %w", err)
	}

	useLegacyEnvConfig := effectiveConfigDir == ""
	resolutionBaseDir := l.cfg.ConfigBaseDir

	projectDir, err := filepath.Abs(cfg.ProjectDir)
	if err != nil {
		return nil, fmt.Errorf("resolve project dir: %w", err)
	}

	providerPath := resolveRelativePath(resolutionBaseDir, cfg.OpenCode.ProviderConfigPath)
	providerJSON, err := providerconfig.LoadWithOptions(providerPath, providerconfig.Options{
		UseLegacyEnv:      useLegacyEnvConfig,
		LegacyEnvFallback: useLegacyEnvConfig,
	})
	if err != nil {
		return nil, fmt.Errorf("load provider config: %w", err)
	}

	agentPath := resolveRelativePath(resolutionBaseDir, cfg.Pipeline.ReviewAgentPromptPath)
	agentPrompt, err := agentconfig.LoadWithOptions(agentPath, cfg.Pipeline.ReviewAgentPrompt, agentconfig.Options{
		UseLegacyEnv:      useLegacyEnvConfig,
		LegacyEnvFallback: useLegacyEnvConfig,
	})
	if err != nil {
		return nil, fmt.Errorf("load agent prompt: %w", err)
	}

	finalizerPath := resolveRelativePath(resolutionBaseDir, cfg.Pipeline.FinalizerPromptPath)
	finalizerPrompt, err := finalizerconfig.LoadWithOptions(finalizerPath, cfg.Pipeline.FinalizerPrompt, finalizerconfig.Options{
		UseLegacyEnv:      useLegacyEnvConfig,
		LegacyEnvFallback: useLegacyEnvConfig,
	})
	if err != nil {
		return nil, fmt.Errorf("load finalizer prompt: %w", err)
	}

	messages, err := promptconfig.LoadReviewMessagesWithRefBaseAndOptions(resolutionBaseDir, projectDir, cfg.Pipeline.ReviewMessagePaths, cfg.Pipeline.ReviewMessages, promptconfig.Options{
		UseLegacyEnv:      useLegacyEnvConfig,
		LegacyEnvFallback: useLegacyEnvConfig,
	})
	if err != nil {
		return nil, fmt.Errorf("load review messages: %w", err)
	}
	slog.Info("loaded review messages", "count", len(messages))
	if len(messages) == 0 {
		slog.Info( // #nosec G706 -- local configuration paths are useful diagnostics
			"no review messages loaded",
			"config_dir", effectiveConfigDir,
			"reviewer_messages_dir", filepath.Join(effectiveConfigDir, "reviewer", "messages"),
			"project_dir", cfg.ProjectDir,
		)
	}

	reviewSubAgents, err := subagentconfig.LoadWithOptions(
		"OR_REVIEW_SUB_AGENT_PROMPT_PATHS", "reviewer",
		resolutionBaseDir,
		cfg.Pipeline.ReviewSubAgentPromptPaths,
		cfg.Pipeline.ReviewSubAgentPrompts,
		subagentconfig.Options{UseLegacyEnv: useLegacyEnvConfig, LegacyEnvFallback: useLegacyEnvConfig},
	)
	if err != nil {
		return nil, fmt.Errorf("load reviewer sub-agent prompts: %w", err)
	}

	finalizerMsgPath := resolveRelativePath(resolutionBaseDir, cfg.Pipeline.FinalizerMessagePath)
	finalizerMessage, err := finalizerconfig.LoadMessageWithOptions(finalizerMsgPath, cfg.Pipeline.FinalizerMessage, finalizerconfig.Options{
		UseLegacyEnv:      useLegacyEnvConfig,
		LegacyEnvFallback: useLegacyEnvConfig,
	})
	if err != nil {
		return nil, fmt.Errorf("load finalizer message: %w", err)
	}

	finalizerSubAgents, err := subagentconfig.LoadWithOptions(
		"OR_FINALIZER_SUB_AGENT_PROMPT_PATHS", "finalizer",
		resolutionBaseDir,
		cfg.Pipeline.FinalizerSubAgentPromptPaths,
		cfg.Pipeline.FinalizerSubAgentPrompts,
		subagentconfig.Options{UseLegacyEnv: useLegacyEnvConfig, LegacyEnvFallback: useLegacyEnvConfig},
	)
	if err != nil {
		return nil, fmt.Errorf("load finalizer sub-agent prompts: %w", err)
	}

	reviewerTools, err := loadConfigDirToolOverrides(effectiveConfigDir, "reviewer")
	if err != nil {
		return nil, fmt.Errorf("load reviewer tools: %w", err)
	}

	finalizerTools, err := loadConfigDirToolOverrides(effectiveConfigDir, "finalizer")
	if err != nil {
		return nil, fmt.Errorf("load finalizer tools: %w", err)
	}

	if err := runner.ValidateBinary(cfg.OpenCode); err != nil {
		return nil, fmt.Errorf("opencode is not available: %w", err)
	}

	reviewerWS, err := workspace.NewAgent(workspace.Config{
		ProviderJSON:  providerJSON,
		Model:         cfg.OpenCode.Model,
		MaxSteps:      cfg.OpenCode.MaxSteps,
		SubAgents:     reviewSubAgents,
		ToolOverrides: reviewerTools,
	}, workspace.AgentSpec{
		Name:         "reviewer",
		ToolFileName: "submit_review.ts",
		ToolContent:  submitReviewTS,
		Prompt:       agentPrompt,
	})
	if err != nil {
		return nil, fmt.Errorf("create reviewer workspace: %w", err)
	}

	finalizerWS, err := workspace.NewAgent(workspace.Config{
		ProviderJSON:  providerJSON,
		Model:         cfg.OpenCode.Model,
		MaxSteps:      cfg.OpenCode.MaxSteps,
		SubAgents:     finalizerSubAgents,
		ToolOverrides: finalizerTools,
	}, workspace.AgentSpec{
		Name:         "finalizer",
		ToolFileName: "submit_final_review.ts",
		ToolContent:  submitFinalReviewTS,
		Prompt:       finalizerPrompt,
	})
	if err != nil {
		_ = reviewerWS.Cleanup()
		return nil, fmt.Errorf("create finalizer workspace: %w", err)
	}

	return &pipeline.RuntimeResources{
		ReviewerRunner:   runner.New(cfg.OpenCode, projectDir, reviewerWS),
		FinalizerRunner:  runner.New(cfg.OpenCode, projectDir, finalizerWS),
		Messages:         messages,
		FinalizerMessage: finalizerMessage,
		Cleanup: func() error {
			reviewerErr := reviewerWS.Cleanup()
			finalizerErr := finalizerWS.Cleanup()
			if reviewerErr != nil {
				return reviewerErr
			}
			return finalizerErr
		},
	}, nil
}

func resolveRelativePath(baseDir, path string) string {
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(baseDir, path)
}
