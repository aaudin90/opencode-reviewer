package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApplyConfigDirDefaults_PopulatesPaths(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "provider.json"), "{}")
	mustWrite(t, filepath.Join(dir, "reviewer", "agent.md"), "reviewer")
	mustWrite(t, filepath.Join(dir, "reviewer", "messages", "02.md"), "two")
	mustWrite(t, filepath.Join(dir, "reviewer", "messages", "01.md"), "one")
	mustWrite(t, filepath.Join(dir, "reviewer", "sub-agents", "b.md"), "subb")
	mustWrite(t, filepath.Join(dir, "reviewer", "sub-agents", "a.md"), "suba")
	mustWrite(t, filepath.Join(dir, "finalizer", "agent.md"), "finalizer")
	mustWrite(t, filepath.Join(dir, "finalizer", "message.md"), "message")
	mustWrite(t, filepath.Join(dir, "finalizer", "sub-agents", "z.md"), "subz")

	cfg := &Config{}
	if err := ApplyConfigDirDefaults(cfg, dir); err != nil {
		t.Fatalf("ApplyConfigDirDefaults: %v", err)
	}

	if cfg.OpenCode.ProviderConfigPath != filepath.Join(dir, "provider.json") {
		t.Fatalf("provider path = %q", cfg.OpenCode.ProviderConfigPath)
	}
	if len(cfg.Pipeline.ReviewMessagePaths) != 2 {
		t.Fatalf("review message paths = %v", cfg.Pipeline.ReviewMessagePaths)
	}
	if cfg.Pipeline.ReviewMessagePaths[0] != filepath.Join(dir, "reviewer", "messages", "01.md") {
		t.Fatalf("review message sort order broken: %v", cfg.Pipeline.ReviewMessagePaths)
	}
	if len(cfg.Pipeline.ReviewSubAgentPromptPaths) != 2 {
		t.Fatalf("review subagent paths = %v", cfg.Pipeline.ReviewSubAgentPromptPaths)
	}
	if cfg.Pipeline.ReviewSubAgentPromptPaths[0] != filepath.Join(dir, "reviewer", "sub-agents", "a.md") {
		t.Fatalf("review subagent sort order broken: %v", cfg.Pipeline.ReviewSubAgentPromptPaths)
	}
	if cfg.Pipeline.FinalizerMessagePath != filepath.Join(dir, "finalizer", "message.md") {
		t.Fatalf("finalizer message path = %q", cfg.Pipeline.FinalizerMessagePath)
	}
	if len(cfg.Pipeline.FinalizerSubAgentPromptPaths) != 1 {
		t.Fatalf("finalizer subagent paths = %v", cfg.Pipeline.FinalizerSubAgentPromptPaths)
	}
}

func TestApplyConfigDirDefaults_ConfigDirOverridesInlineOrExplicit(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "reviewer", "agent.md"), "reviewer")
	mustWrite(t, filepath.Join(dir, "reviewer", "messages", "01.md"), "one")

	cfg := &Config{}
	cfg.Pipeline.ReviewAgentPrompt = "inline"
	cfg.Pipeline.ReviewMessagePaths = []string{"existing"}

	if err := ApplyConfigDirDefaults(cfg, dir); err != nil {
		t.Fatalf("ApplyConfigDirDefaults: %v", err)
	}

	if cfg.Pipeline.ReviewAgentPrompt != "" {
		t.Fatalf("review agent inline should be cleared, got %q", cfg.Pipeline.ReviewAgentPrompt)
	}
	if cfg.Pipeline.ReviewAgentPromptPath != filepath.Join(dir, "reviewer", "agent.md") {
		t.Fatalf("review agent path = %q", cfg.Pipeline.ReviewAgentPromptPath)
	}
	if len(cfg.Pipeline.ReviewMessagePaths) != 1 || cfg.Pipeline.ReviewMessagePaths[0] != filepath.Join(dir, "reviewer", "messages", "01.md") {
		t.Fatalf("review messages were not overridden by config dir: %v", cfg.Pipeline.ReviewMessagePaths)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
