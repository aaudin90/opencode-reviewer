package finalizerconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadWithOptions_DisablesLegacyEnv(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, "env-finalizer.md")
	if err := os.WriteFile(envPath, []byte("env prompt"), 0o600); err != nil {
		t.Fatal(err)
	}
	tomlPath := filepath.Join(dir, "toml-finalizer.md")
	if err := os.WriteFile(tomlPath, []byte("toml prompt"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("OR_FINALIZER_PROMPT_PATH", envPath)

	prompt, err := LoadWithOptions(tomlPath, "", Options{UseLegacyEnv: false})
	if err != nil {
		t.Fatalf("LoadWithOptions: %v", err)
	}
	if prompt != "toml prompt" {
		t.Fatalf("prompt = %q, want TOML prompt when legacy env is disabled", prompt)
	}
}

func TestLoadMessageWithOptions_DisablesLegacyEnv(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, "env-message.md")
	if err := os.WriteFile(envPath, []byte("env message"), 0o600); err != nil {
		t.Fatal(err)
	}
	tomlPath := filepath.Join(dir, "toml-message.md")
	if err := os.WriteFile(tomlPath, []byte("toml message"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("OR_FINALIZER_MESSAGE_PATH", envPath)

	message, err := LoadMessageWithOptions(tomlPath, "", Options{UseLegacyEnv: false})
	if err != nil {
		t.Fatalf("LoadMessageWithOptions: %v", err)
	}
	if message != "toml message" {
		t.Fatalf("message = %q, want TOML message when legacy env is disabled", message)
	}
}
