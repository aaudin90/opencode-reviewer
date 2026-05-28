package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_DefaultOpenCodeLogDir(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if cfg.OpenCode.LogDir != "opencode-review-logs" {
		t.Fatalf("LogDir = %q, want opencode-review-logs", cfg.OpenCode.LogDir)
	}
}

func TestLoad_DefaultOpenCodePrecheckTimeout(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if cfg.OpenCode.PrecheckTimeout != 300 {
		t.Fatalf("PrecheckTimeout = %d, want 300", cfg.OpenCode.PrecheckTimeout)
	}
}

func TestApplyEnvOverrides_OpenCodePrecheckTimeout(t *testing.T) {
	t.Setenv("OR_OPENCODE_PRECHECK_TIMEOUT", "42")

	cfg := &Config{}
	ApplyEnvOverrides(cfg)

	if cfg.OpenCode.PrecheckTimeout != 42 {
		t.Fatalf("PrecheckTimeout = %d, want 42", cfg.OpenCode.PrecheckTimeout)
	}
}

func TestApplyEnvOverrides_OpenCodeLogs(t *testing.T) {
	t.Setenv("OR_OPENCODE_PRINT_LOGS", "true")
	t.Setenv("OR_OPENCODE_LOG_LEVEL", "DEBUG")
	t.Setenv("OR_OPENCODE_LOG_DIR", "custom-logs")

	cfg := &Config{}
	ApplyEnvOverrides(cfg)

	if !cfg.OpenCode.PrintLogs {
		t.Fatal("PrintLogs = false, want true")
	}
	if cfg.OpenCode.LogLevel != "DEBUG" {
		t.Fatalf("LogLevel = %q, want DEBUG", cfg.OpenCode.LogLevel)
	}
	if cfg.OpenCode.LogDir != "custom-logs" {
		t.Fatalf("LogDir = %q, want custom-logs", cfg.OpenCode.LogDir)
	}
}

func TestApplyEnvOverrides_OpenCodePrintLogsFalse(t *testing.T) {
	tests := []string{"false", "0"}

	for _, value := range tests {
		t.Run(value, func(t *testing.T) {
			t.Setenv("OR_OPENCODE_PRINT_LOGS", value)

			cfg := &Config{OpenCode: OpenCodeConfig{PrintLogs: true}}
			ApplyEnvOverrides(cfg)

			if cfg.OpenCode.PrintLogs {
				t.Fatalf("PrintLogs = true for %q, want false", value)
			}
		})
	}
}

func TestApplyEnvOverrides_OpenCodeFallbackModels(t *testing.T) {
	t.Setenv("OR_OPENCODE_FALLBACK_MODELS", " openai/gpt-5 , , anthropic/claude-sonnet-4-5 ,, ")

	cfg := &Config{}
	ApplyEnvOverrides(cfg)

	want := []string{"openai/gpt-5", "anthropic/claude-sonnet-4-5"}
	if len(cfg.OpenCode.FallbackModels) != len(want) {
		t.Fatalf("FallbackModels len = %d, want %d (%v)", len(cfg.OpenCode.FallbackModels), len(want), cfg.OpenCode.FallbackModels)
	}
	for i := range want {
		if cfg.OpenCode.FallbackModels[i] != want[i] {
			t.Fatalf("FallbackModels[%d] = %q, want %q", i, cfg.OpenCode.FallbackModels[i], want[i])
		}
	}
}

func TestLoad_OpenCodeFallbackModelsFromTOML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	data := []byte(`
[opencode]
fallback_models = ["openai/gpt-5", "anthropic/claude-sonnet-4-5"]
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	assertModelChain(t, cfg.OpenCode.FallbackModels, []string{"openai/gpt-5", "anthropic/claude-sonnet-4-5"})
}

func TestOpenCodeConfigModelChain(t *testing.T) {
	t.Run("nil when no fallbacks", func(t *testing.T) {
		chain, err := (OpenCodeConfig{Model: "openai/gpt-5"}).ModelChain("provider/default")
		if err != nil {
			t.Fatalf("ModelChain() error = %v", err)
		}
		if chain != nil {
			t.Fatalf("ModelChain() = %v, want nil", chain)
		}
	})

	t.Run("primary from opencode model", func(t *testing.T) {
		chain, err := (OpenCodeConfig{
			Model:          "openai/gpt-5",
			FallbackModels: []string{"anthropic/claude-sonnet-4-5", "openai/gpt-5", " google/gemini-2.5-pro "},
		}).ModelChain("provider/ignored")
		if err != nil {
			t.Fatalf("ModelChain() error = %v", err)
		}
		want := []string{"openai/gpt-5", "anthropic/claude-sonnet-4-5", "google/gemini-2.5-pro"}
		assertModelChain(t, chain, want)
	})

	t.Run("primary from provider model", func(t *testing.T) {
		chain, err := (OpenCodeConfig{
			FallbackModels: []string{"anthropic/claude-sonnet-4-5"},
		}).ModelChain("openai/gpt-5")
		if err != nil {
			t.Fatalf("ModelChain() error = %v", err)
		}
		assertModelChain(t, chain, []string{"openai/gpt-5", "anthropic/claude-sonnet-4-5"})
	})

	t.Run("invalid primary", func(t *testing.T) {
		_, err := (OpenCodeConfig{
			Model:          "gpt-5",
			FallbackModels: []string{"anthropic/claude-sonnet-4-5"},
		}).ModelChain("")
		if err == nil {
			t.Fatal("ModelChain() error = nil, want error")
		}
	})

	t.Run("invalid fallback", func(t *testing.T) {
		_, err := (OpenCodeConfig{
			Model:          "openai/gpt-5",
			FallbackModels: []string{"claude-sonnet-4-5"},
		}).ModelChain("")
		if err == nil {
			t.Fatal("ModelChain() error = nil, want error")
		}
	})

	t.Run("invalid provider primary", func(t *testing.T) {
		_, err := (OpenCodeConfig{
			FallbackModels: []string{"anthropic/claude-sonnet-4-5"},
		}).ModelChain("gpt-5")
		if err == nil {
			t.Fatal("ModelChain() error = nil, want error")
		}
	})
}

func TestResolveOpenCodeLogDir_Default(t *testing.T) {
	projectDir := t.TempDir()
	want := filepath.Join(projectDir, "opencode-review-logs")

	if got := ResolveOpenCodeLogDir("", projectDir); got != want {
		t.Fatalf("ResolveOpenCodeLogDir() = %q, want %q", got, want)
	}
}

func TestResolveOpenCodeLogDir_Relative(t *testing.T) {
	projectDir := t.TempDir()
	want := filepath.Join(projectDir, "logs", "opencode")

	if got := ResolveOpenCodeLogDir(filepath.Join("logs", "opencode"), projectDir); got != want {
		t.Fatalf("ResolveOpenCodeLogDir() = %q, want %q", got, want)
	}
}

func TestResolveOpenCodeLogDir_Absolute(t *testing.T) {
	logDir := filepath.Join(t.TempDir(), "logs", "..", "opencode")
	want := filepath.Clean(logDir)

	if got := ResolveOpenCodeLogDir(logDir, t.TempDir()); got != want {
		t.Fatalf("ResolveOpenCodeLogDir() = %q, want %q", got, want)
	}
}

func assertModelChain(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("ModelChain len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ModelChain[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
