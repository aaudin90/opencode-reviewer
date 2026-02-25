package agentconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_InlinePrompt(t *testing.T) {
	t.Setenv("REVIEW_AGENT_PROMPT_PATH", "")

	prompt, err := Load("", "You are a code reviewer.")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if prompt != "You are a code reviewer." {
		t.Errorf("prompt = %q, want inline text", prompt)
	}
}

func TestLoad_InlineJSONReturnedAsRaw(t *testing.T) {
	raw := `{"prompt": "Review code carefully."}`
	t.Setenv("REVIEW_AGENT_PROMPT_PATH", "")

	prompt, err := Load("", raw)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if prompt != raw {
		t.Errorf("prompt = %q, want raw content %q", prompt, raw)
	}
}

func TestLoad_FromEnvFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.txt")
	if err := os.WriteFile(path, []byte("File prompt."), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("REVIEW_AGENT_PROMPT_PATH", path)

	prompt, err := Load("", "inline prompt")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if prompt != "File prompt." {
		t.Errorf("prompt = %q, want file content (env file takes priority over inline)", prompt)
	}
}

func TestLoad_NeitherSet(t *testing.T) {
	t.Setenv("REVIEW_AGENT_PROMPT_PATH", "")

	prompt, err := Load("", "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if prompt != defaultPrompt {
		t.Errorf("prompt = %q, want default prompt when neither set", prompt)
	}
}

func TestLoad_WhitespaceOnlyFallsBackToDefault(t *testing.T) {
	t.Setenv("REVIEW_AGENT_PROMPT_PATH", "")

	prompt, err := Load("", "   ")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if prompt != defaultPrompt {
		t.Errorf("prompt = %q, want default prompt for whitespace-only input", prompt)
	}
}

func TestLoad_FromConfigPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent-prompt.md")
	if err := os.WriteFile(path, []byte("TOML prompt content."), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("REVIEW_AGENT_PROMPT_PATH", "")

	prompt, err := Load(path, "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if prompt != "TOML prompt content." {
		t.Errorf("prompt = %q, want TOML fallback content", prompt)
	}
}

func TestLoad_InlineOverridesConfigPath(t *testing.T) {
	t.Setenv("REVIEW_AGENT_PROMPT_PATH", "")

	prompt, err := Load("/nonexistent/path.md", "inline prompt wins")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if prompt != "inline prompt wins" {
		t.Errorf("prompt = %q, want inline to take priority over configPath", prompt)
	}
}

func TestLoad_EnvFileOverridesAll(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, "env-agent.txt")
	if err := os.WriteFile(envPath, []byte("env file wins"), 0o600); err != nil {
		t.Fatal(err)
	}

	tomlPath := filepath.Join(dir, "toml-agent.txt")
	if err := os.WriteFile(tomlPath, []byte("toml path content"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("REVIEW_AGENT_PROMPT_PATH", envPath)

	prompt, err := Load(tomlPath, "inline content")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if prompt != "env file wins" {
		t.Errorf("prompt = %q, want env file to take priority over inline and configPath", prompt)
	}
}
