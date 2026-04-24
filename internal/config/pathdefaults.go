package config

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func ApplyConfigDirDefaults(cfg *Config, configDir string) error {
	if configDir == "" {
		return nil
	}

	if cfg.OpenCode.ProviderConfigPath == "" {
		provider := filepath.Join(configDir, "provider.json")
		if exists(provider) {
			cfg.OpenCode.ProviderConfigPath = provider
		}
	}

	if cfg.Pipeline.ReviewAgentPrompt == "" && cfg.Pipeline.ReviewAgentPromptPath == "" {
		path := filepath.Join(configDir, "reviewer", "agent.md")
		if exists(path) {
			cfg.Pipeline.ReviewAgentPromptPath = path
		}
	}

	if len(cfg.Pipeline.ReviewMessages) == 0 && len(cfg.Pipeline.ReviewMessagePaths) == 0 {
		cfg.Pipeline.ReviewMessagePaths = discoverMarkdown(filepath.Join(configDir, "reviewer", "messages"))
	}

	if cfg.Pipeline.FinalizerPrompt == "" && cfg.Pipeline.FinalizerPromptPath == "" {
		path := filepath.Join(configDir, "finalizer", "agent.md")
		if exists(path) {
			cfg.Pipeline.FinalizerPromptPath = path
		}
	}

	if cfg.Pipeline.FinalizerMessage == "" && cfg.Pipeline.FinalizerMessagePath == "" {
		path := filepath.Join(configDir, "finalizer", "message.md")
		if exists(path) {
			cfg.Pipeline.FinalizerMessagePath = path
		}
	}

	if len(cfg.Pipeline.ReviewSubAgentPrompts) == 0 && len(cfg.Pipeline.ReviewSubAgentPromptPaths) == 0 {
		cfg.Pipeline.ReviewSubAgentPromptPaths = discoverMarkdown(filepath.Join(configDir, "reviewer", "sub-agents"))
	}

	if len(cfg.Pipeline.FinalizerSubAgentPrompts) == 0 && len(cfg.Pipeline.FinalizerSubAgentPromptPaths) == 0 {
		cfg.Pipeline.FinalizerSubAgentPromptPaths = discoverMarkdown(filepath.Join(configDir, "finalizer", "sub-agents"))
	}

	return nil
}

func discoverMarkdown(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			continue
		}
		paths = append(paths, filepath.Join(dir, entry.Name()))
	}
	sort.Strings(paths)
	return paths
}

func exists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
