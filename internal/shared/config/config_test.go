package config

import (
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
