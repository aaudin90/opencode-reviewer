package agentconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_RawText(t *testing.T) {
	t.Setenv("REVIEW_AGENT_CONFIG", "You are a code reviewer.")
	t.Setenv("REVIEW_AGENT_CONFIG_PATH", "")

	prompt, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if prompt != "You are a code reviewer." {
		t.Errorf("prompt = %q, want raw text", prompt)
	}
}

func TestLoad_JSONReturnedAsRaw(t *testing.T) {
	raw := `{"prompt": "Review code carefully."}`
	t.Setenv("REVIEW_AGENT_CONFIG", raw)
	t.Setenv("REVIEW_AGENT_CONFIG_PATH", "")

	prompt, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if prompt != raw {
		t.Errorf("prompt = %q, want raw content %q", prompt, raw)
	}
}

func TestLoad_FromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.txt")
	if err := os.WriteFile(path, []byte("File prompt."), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("REVIEW_AGENT_CONFIG_PATH", path)
	t.Setenv("REVIEW_AGENT_CONFIG", "env prompt")

	prompt, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if prompt != "File prompt." {
		t.Errorf("prompt = %q, want file content (file takes priority)", prompt)
	}
}

func TestLoad_NeitherSet(t *testing.T) {
	t.Setenv("REVIEW_AGENT_CONFIG", "")
	t.Setenv("REVIEW_AGENT_CONFIG_PATH", "")

	prompt, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if prompt != defaultPrompt {
		t.Errorf("prompt = %q, want default prompt when neither set", prompt)
	}
}

func TestLoad_WhitespaceOnlyFallsBackToDefault(t *testing.T) {
	t.Setenv("REVIEW_AGENT_CONFIG", "   ")
	t.Setenv("REVIEW_AGENT_CONFIG_PATH", "")

	prompt, err := Load("")
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

	t.Setenv("REVIEW_AGENT_CONFIG", "")
	t.Setenv("REVIEW_AGENT_CONFIG_PATH", "")

	prompt, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if prompt != "TOML prompt content." {
		t.Errorf("prompt = %q, want TOML fallback content", prompt)
	}
}

func TestLoad_EnvOverridesConfigPath(t *testing.T) {
	t.Setenv("REVIEW_AGENT_CONFIG", "env prompt wins")
	t.Setenv("REVIEW_AGENT_CONFIG_PATH", "")

	prompt, err := Load("/nonexistent/path.md")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if prompt != "env prompt wins" {
		t.Errorf("prompt = %q, want env to take priority over configPath", prompt)
	}
}
