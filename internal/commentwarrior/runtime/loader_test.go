package commentwarriorruntime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/aaudin90/opencode-reviewer/internal/shared/config"
)

func TestLoaderAppliesProviderHeaders(t *testing.T) {
	projectDir := t.TempDir()
	configDir := t.TempDir()
	writeRuntimeTestFile(t, filepath.Join(configDir, "provider.json"), `{
		"provider": {
			"proxy": {
				"models": {"model": {}}
			}
		},
		"model": "proxy/model"
	}`)

	workspaceRoot := t.TempDir()
	cacheHome := t.TempDir()
	t.Setenv("TMPDIR", workspaceRoot)
	t.Setenv("HOME", cacheHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(cacheHome, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(cacheHome, ".cache"))
	t.Setenv("BUN_INSTALL_CACHE_DIR", filepath.Join(cacheHome, ".bun", "install", "cache"))

	loader := New(Config{
		AppConfig: &config.Config{
			ProjectDir: projectDir,
			OpenCode: config.OpenCodeConfig{
				Endpoint: "http://127.0.0.1:1",
			},
		},
		ConfigBaseDir:   ".",
		CLIConfigDir:    configDir,
		ProviderHeaders: map[string]string{"X-Request-Source": "review-workflow"},
	})

	resources, err := loader.LoadRuntime(context.Background())
	if err != nil {
		t.Fatalf("LoadRuntime: %v", err)
	}
	t.Cleanup(func() { _ = resources.Cleanup() })

	configPaths, err := filepath.Glob(filepath.Join(workspaceRoot, "opencode-review-*", "opencode", "opencode.json"))
	if err != nil {
		t.Fatalf("glob opencode configs: %v", err)
	}
	if len(configPaths) != 1 {
		t.Fatalf("generated config paths = %v, want comment-warrior config", configPaths)
	}

	data, err := os.ReadFile(configPaths[0])
	if err != nil {
		t.Fatalf("read %s: %v", configPaths[0], err)
	}
	var generated map[string]any
	if err := json.Unmarshal(data, &generated); err != nil {
		t.Fatalf("parse %s: %v", configPaths[0], err)
	}
	provider := generated["provider"].(map[string]any)["proxy"].(map[string]any)
	headers := provider["options"].(map[string]any)["headers"].(map[string]any)
	if headers["X-Request-Source"] != "review-workflow" {
		t.Errorf("X-Request-Source = %v, want review-workflow", headers["X-Request-Source"])
	}
}

func TestLoadSpecificTextPrefersSpecificConfigDirFile(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()
	writeRuntimeTestFile(t, filepath.Join(configDir, "comment-warrior", "finding-message.md"), "specific")

	got, err := loadSpecificText(
		configDir,
		"comment-warrior/finding-message.md",
		"OR_COMMENT_WARRIOR_FINDING_MESSAGE_PATH",
		"default",
		false,
	)
	if err != nil {
		t.Fatalf("loadSpecificText: %v", err)
	}
	if got != "specific" {
		t.Fatalf("loadSpecificText() = %q, want specific", got)
	}
}

func TestLoadSpecificTextUsesDefaultWhenSpecificConfigDirFileMissing(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()
	writeRuntimeTestFile(t, filepath.Join(configDir, "comment-warrior", "message.md"), "common")

	got, err := loadSpecificText(
		configDir,
		"comment-warrior/finding-message.md",
		"OR_COMMENT_WARRIOR_FINDING_MESSAGE_PATH",
		"default",
		false,
	)
	if err != nil {
		t.Fatalf("loadSpecificText: %v", err)
	}
	if got != "default" {
		t.Fatalf("loadSpecificText() = %q, want default", got)
	}
}

func TestLoadSpecificTextUsesSpecificLegacyEnv(t *testing.T) {
	specific := filepath.Join(t.TempDir(), "specific.md")
	writeRuntimeTestFile(t, specific, "specific env")
	t.Setenv("OR_COMMENT_WARRIOR_FINDING_MESSAGE_PATH", specific)

	got, err := loadSpecificText(
		"",
		"comment-warrior/finding-message.md",
		"OR_COMMENT_WARRIOR_FINDING_MESSAGE_PATH",
		"default",
		true,
	)
	if err != nil {
		t.Fatalf("loadSpecificText: %v", err)
	}
	if got != "specific env" {
		t.Fatalf("loadSpecificText() = %q, want specific env", got)
	}
}

func writeRuntimeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
