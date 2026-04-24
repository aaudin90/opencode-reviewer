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

	provider := filepath.Join(configDir, "provider.json")
	if exists(provider) {
		cfg.OpenCode.ProviderConfigPath = provider
	}

	path := filepath.Join(configDir, "reviewer", "agent.md")
	if exists(path) {
		cfg.Pipeline.ReviewAgentPrompt = ""
		cfg.Pipeline.ReviewAgentPromptPath = path
	}

	if paths := discoverMarkdown(filepath.Join(configDir, "reviewer", "messages")); len(paths) > 0 {
		cfg.Pipeline.ReviewMessages = nil
		cfg.Pipeline.ReviewMessagePaths = paths
	}

	path = filepath.Join(configDir, "finalizer", "agent.md")
	if exists(path) {
		cfg.Pipeline.FinalizerPrompt = ""
		cfg.Pipeline.FinalizerPromptPath = path
	}

	path = filepath.Join(configDir, "finalizer", "message.md")
	if exists(path) {
		cfg.Pipeline.FinalizerMessage = ""
		cfg.Pipeline.FinalizerMessagePath = path
	}

	if paths := discoverMarkdown(filepath.Join(configDir, "reviewer", "sub-agents")); len(paths) > 0 {
		cfg.Pipeline.ReviewSubAgentPrompts = nil
		cfg.Pipeline.ReviewSubAgentPromptPaths = paths
	}

	if paths := discoverMarkdown(filepath.Join(configDir, "finalizer", "sub-agents")); len(paths) > 0 {
		cfg.Pipeline.FinalizerSubAgentPrompts = nil
		cfg.Pipeline.FinalizerSubAgentPromptPaths = paths
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
