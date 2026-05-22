package commentwarriorruntime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aaudin90/opencode-reviewer/internal/shared/config"
	"github.com/aaudin90/opencode-reviewer/internal/shared/configdir"
	"github.com/aaudin90/opencode-reviewer/internal/shared/providerconfig"
	"github.com/aaudin90/opencode-reviewer/internal/shared/runner"
	"github.com/aaudin90/opencode-reviewer/internal/shared/subagentconfig"
	"github.com/aaudin90/opencode-reviewer/internal/shared/workspace"
)

type RuntimeResources struct {
	Runner         *runner.Runner
	FindingMessage string
	MentionMessage string
	Cleanup        func() error
}

type Loader struct {
	cfg Config
}

func New(cfg Config) *Loader {
	return &Loader{cfg: cfg}
}

func (l *Loader) LoadRuntime(_ context.Context) (*RuntimeResources, error) {
	if l.cfg.AppConfig == nil {
		return nil, fmt.Errorf("app config is nil")
	}
	cfg := *l.cfg.AppConfig
	effectiveConfigDir, _, err := configdir.Resolve(
		l.cfg.CLIConfigDir,
		os.Getenv("OR_CONFIG_DIR"),
		cfg.ProjectDir,
		l.cfg.DisableConfigDirAutoDiscovery,
	)
	if err != nil {
		return nil, fmt.Errorf("resolve config directory: %w", err)
	}
	if err := config.ApplyConfigDirDefaults(&cfg, effectiveConfigDir); err != nil {
		return nil, fmt.Errorf("apply config directory defaults: %w", err)
	}
	useLegacyEnv := effectiveConfigDir == ""
	baseDir := l.cfg.ConfigBaseDir

	providerPath := resolveRelativePath(baseDir, cfg.OpenCode.ProviderConfigPath)
	providerJSON, err := providerconfig.LoadWithOptions(providerPath, providerconfig.Options{
		UseLegacyEnv:      useLegacyEnv,
		LegacyEnvFallback: useLegacyEnv,
	})
	if err != nil {
		return nil, fmt.Errorf("load provider config: %w", err)
	}
	providerModel, err := providerconfig.DefaultModel(providerJSON)
	if err != nil {
		return nil, fmt.Errorf("read provider model: %w", err)
	}
	if _, err := cfg.OpenCode.ModelChain(providerModel); err != nil {
		return nil, fmt.Errorf("resolve model chain: %w", err)
	}

	agentPrompt, err := loadText(effectiveConfigDir, "comment-warrior/agent.md", "OR_COMMENT_WARRIOR_AGENT_PROMPT_PATH", defaultAgentPrompt, useLegacyEnv)
	if err != nil {
		return nil, fmt.Errorf("load comment-warrior agent prompt: %w", err)
	}
	findingMessage, err := loadSpecificText(
		effectiveConfigDir,
		"comment-warrior/finding-message.md",
		"OR_COMMENT_WARRIOR_FINDING_MESSAGE_PATH",
		defaultFindingMessage,
		useLegacyEnv,
	)
	if err != nil {
		return nil, fmt.Errorf("load comment-warrior finding message: %w", err)
	}
	mentionMessage, err := loadSpecificText(
		effectiveConfigDir,
		"comment-warrior/mention-message.md",
		"OR_COMMENT_WARRIOR_MENTION_MESSAGE_PATH",
		defaultMentionMessage,
		useLegacyEnv,
	)
	if err != nil {
		return nil, fmt.Errorf("load comment-warrior mention message: %w", err)
	}
	var subAgents []subagentconfig.SubAgent
	if paths := discoverMarkdown(filepath.Join(effectiveConfigDir, "comment-warrior", "sub-agents")); len(paths) > 0 {
		var err error
		subAgents, err = subagentconfig.LoadWithOptions("", "comment-warrior", baseDir, paths, nil, subagentconfig.Options{})
		if err != nil {
			return nil, fmt.Errorf("load comment-warrior config-dir sub-agents: %w", err)
		}
	}
	toolsDir := ""
	if effectiveConfigDir != "" {
		toolsDir = filepath.Join(effectiveConfigDir, "comment-warrior", "tools")
	}
	tools, err := loadToolOverrides(toolsDir)
	if err != nil {
		return nil, fmt.Errorf("load comment-warrior tools: %w", err)
	}
	if err := runner.ValidateBinary(cfg.OpenCode); err != nil {
		return nil, fmt.Errorf("opencode is not available: %w", err)
	}
	projectDir, err := filepath.Abs(cfg.ProjectDir)
	if err != nil {
		return nil, fmt.Errorf("resolve project dir: %w", err)
	}
	cfg.OpenCode.LogDir = config.ResolveOpenCodeLogDir(cfg.OpenCode.LogDir, projectDir)
	ws, err := workspace.NewAgent(workspace.Config{
		ProviderJSON:  providerJSON,
		Model:         cfg.OpenCode.Model,
		MaxSteps:      cfg.OpenCode.MaxSteps,
		SubAgents:     subAgents,
		ToolOverrides: tools,
	}, workspace.AgentSpec{
		Name:         "comment-warrior",
		ToolFileName: "submit_comment_warrior_decision.ts",
		ToolContent:  submitCommentWarriorDecisionTS,
		Prompt:       agentPrompt,
	})
	if err != nil {
		return nil, fmt.Errorf("create comment-warrior workspace: %w", err)
	}
	return &RuntimeResources{
		Runner:         runner.New(cfg.OpenCode, projectDir, ws, "comment-warrior"),
		FindingMessage: findingMessage,
		MentionMessage: mentionMessage,
		Cleanup:        ws.Cleanup,
	}, nil
}

func loadSpecificText(configDir, relPath, envName, fallback string, useLegacyEnv bool) (string, error) {
	if configDir != "" {
		if content, ok, err := readOptional(filepath.Join(configDir, relPath)); ok || err != nil {
			return content, err
		}
	}
	if useLegacyEnv {
		if path := os.Getenv(envName); path != "" {
			data, err := os.ReadFile(filepath.Clean(path)) // #nosec G304 G703 -- trusted env fallback
			if err != nil {
				return "", err
			}
			return string(data), nil
		}
	}
	return fallback, nil
}

func loadText(configDir, relPath, envName, fallback string, useLegacyEnv bool) (string, error) {
	if configDir != "" {
		if content, ok, err := readOptional(filepath.Join(configDir, relPath)); ok || err != nil {
			return content, err
		}
	}
	if useLegacyEnv {
		if path := os.Getenv(envName); path != "" {
			data, err := os.ReadFile(filepath.Clean(path)) // #nosec G304 G703 -- trusted env fallback
			if err != nil {
				return "", err
			}
			return string(data), nil
		}
	}
	return fallback, nil
}

func readOptional(path string) (string, bool, error) {
	data, err := os.ReadFile(filepath.Clean(path)) // #nosec G304 G703 -- trusted config-dir path
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}
	return string(data), true, nil
}

func resolveRelativePath(baseDir, path string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(baseDir, path)
}

func discoverMarkdown(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var paths []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			paths = append(paths, filepath.Join(dir, entry.Name()))
		}
	}
	return paths
}
