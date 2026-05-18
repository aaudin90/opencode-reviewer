package subagentconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validFrontmatter = `---
description: "Test sub-agent"
mode: subagent
tools:
  submit_review: false
  submit_final_review: false
permission:
  edit: deny
  bash:
    "*": deny
---

`

func TestLoad_EnvPaths(t *testing.T) {
	dir := t.TempDir()
	f1 := filepath.Join(dir, "verifier.md")
	f2 := filepath.Join(dir, "checker.md")
	if err := os.WriteFile(f1, []byte(validFrontmatter+"verify prompt"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f2, []byte(validFrontmatter+"checker prompt"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("TEST_SUB_AGENT_PATHS", f1+","+f2)

	result, err := Load("TEST_SUB_AGENT_PATHS", "reviewer", ".", nil, nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("got %d results, want 2", len(result))
	}
	if result[0].Name != "verifier" {
		t.Errorf("result[0].Name = %q, want %q", result[0].Name, "verifier")
	}
	if !strings.Contains(result[0].Prompt, "verify prompt") {
		t.Errorf("result[0].Prompt missing expected content")
	}
	if result[1].Name != "checker" {
		t.Errorf("result[1].Name = %q, want %q", result[1].Name, "checker")
	}
}

func TestLoad_InlineNaming(t *testing.T) {
	t.Setenv("TEST_SUB_AGENT_PATHS", "")

	inline := []string{validFrontmatter + "prompt one", validFrontmatter + "prompt two"}
	result, err := Load("TEST_SUB_AGENT_PATHS", "reviewer", ".", nil, inline)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("got %d results, want 2", len(result))
	}
	if result[0].Name != "sub-reviewer-1" {
		t.Errorf("result[0].Name = %q, want %q", result[0].Name, "sub-reviewer-1")
	}
	if result[1].Name != "sub-reviewer-2" {
		t.Errorf("result[1].Name = %q, want %q", result[1].Name, "sub-reviewer-2")
	}
}

func TestLoad_TomlPaths(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "agent.md")
	if err := os.WriteFile(f, []byte(validFrontmatter+"file content"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("TEST_SUB_AGENT_PATHS", "")

	result, err := Load("TEST_SUB_AGENT_PATHS", "finalizer", dir, []string{"agent.md"}, nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d results, want 1", len(result))
	}
	if result[0].Name != "agent" {
		t.Errorf("result[0].Name = %q, want %q", result[0].Name, "agent")
	}
}

func TestLoad_NilWhenEmpty(t *testing.T) {
	t.Setenv("TEST_SUB_AGENT_PATHS", "")

	result, err := Load("TEST_SUB_AGENT_PATHS", "reviewer", ".", nil, nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if result != nil {
		t.Errorf("got %v, want nil", result)
	}
}

func TestLoad_EnvOverridesTomlInline(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "env-agent.md")
	if err := os.WriteFile(f, []byte(validFrontmatter+"env content"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("TEST_SUB_AGENT_PATHS", f)

	inline := []string{validFrontmatter + "inline content"}
	result, err := Load("TEST_SUB_AGENT_PATHS", "reviewer", ".", []string{"/some/path.md"}, inline)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d results, want 1", len(result))
	}
	if !strings.Contains(result[0].Prompt, "env content") {
		t.Errorf("result[0].Prompt should contain env content (env should take priority)")
	}
}

func TestLoad_InlineOverridesTomlPaths(t *testing.T) {
	t.Setenv("TEST_SUB_AGENT_PATHS", "")

	inline := []string{validFrontmatter + "inline msg"}
	result, err := Load("TEST_SUB_AGENT_PATHS", "finalizer", ".", []string{"/nonexistent/path.md"}, inline)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d results, want 1", len(result))
	}
	if !strings.Contains(result[0].Prompt, "inline msg") {
		t.Errorf("result[0].Prompt should contain inline msg (inline should take priority over paths)")
	}
}

func TestLoad_ValidationErrorNoFrontmatter(t *testing.T) {
	t.Setenv("TEST_SUB_AGENT_PATHS", "")

	inline := []string{"plain text without frontmatter"}
	_, err := Load("TEST_SUB_AGENT_PATHS", "reviewer", ".", nil, inline)
	if err == nil {
		t.Fatal("expected error for missing frontmatter, got nil")
	}
	if !strings.Contains(err.Error(), "missing YAML frontmatter") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "missing YAML frontmatter")
	}
}

func TestLoad_ValidationErrorNoClosingFrontmatter(t *testing.T) {
	t.Setenv("TEST_SUB_AGENT_PATHS", "")

	inline := []string{"---\nmode: subagent\nprompt without closing"}
	_, err := Load("TEST_SUB_AGENT_PATHS", "reviewer", ".", nil, inline)
	if err == nil {
		t.Fatal("expected error for missing closing ---, got nil")
	}
	if !strings.Contains(err.Error(), "missing closing ---") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "missing closing ---")
	}
}

func TestLoad_ValidationErrorNoModeSubagent(t *testing.T) {
	t.Setenv("TEST_SUB_AGENT_PATHS", "")

	inline := []string{"---\ndescription: test\n---\nprompt"}
	_, err := Load("TEST_SUB_AGENT_PATHS", "reviewer", ".", nil, inline)
	if err == nil {
		t.Fatal("expected error for missing mode: subagent, got nil")
	}
	if !strings.Contains(err.Error(), "mode: subagent") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "mode: subagent")
	}
}

func TestLoad_ValidationErrorInFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "bad-agent.md")
	if err := os.WriteFile(f, []byte("no frontmatter"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("TEST_SUB_AGENT_PATHS", "")

	_, err := Load("TEST_SUB_AGENT_PATHS", "reviewer", dir, []string{"bad-agent.md"}, nil)
	if err == nil {
		t.Fatal("expected error for invalid file, got nil")
	}
	if !strings.Contains(err.Error(), "missing YAML frontmatter") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "missing YAML frontmatter")
	}
}

func TestLoad_EnsureToolRestrictions_AddsWhenMissing(t *testing.T) {
	t.Setenv("TEST_SUB_AGENT_PATHS", "")

	prompt := "---\ndescription: \"test\"\nmode: subagent\n---\n\nPrompt text"
	inline := []string{prompt}
	result, err := Load("TEST_SUB_AGENT_PATHS", "reviewer", ".", nil, inline)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !strings.Contains(result[0].Prompt, "submit_review: false") {
		t.Error("expected submit_review: false to be injected")
	}
	if !strings.Contains(result[0].Prompt, "submit_final_review: false") {
		t.Error("expected submit_final_review: false to be injected")
	}
	if !strings.Contains(result[0].Prompt, "tools:") {
		t.Error("expected tools: section to be injected")
	}
}

func TestLoad_EnsureToolRestrictions_ExtendsExistingTools(t *testing.T) {
	t.Setenv("TEST_SUB_AGENT_PATHS", "")

	prompt := "---\ndescription: \"test\"\nmode: subagent\ntools:\n  webfetch: false\n---\n\nPrompt"
	inline := []string{prompt}
	result, err := Load("TEST_SUB_AGENT_PATHS", "reviewer", ".", nil, inline)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !strings.Contains(result[0].Prompt, "submit_review: false") {
		t.Error("expected submit_review: false to be injected")
	}
	if !strings.Contains(result[0].Prompt, "submit_final_review: false") {
		t.Error("expected submit_final_review: false to be injected")
	}
	if !strings.Contains(result[0].Prompt, "webfetch: false") {
		t.Error("expected existing webfetch: false to be preserved")
	}
}

func TestLoad_EnsureToolRestrictions_NoopWhenPresent(t *testing.T) {
	t.Setenv("TEST_SUB_AGENT_PATHS", "")

	prompt := "---\ndescription: \"test\"\nmode: subagent\ntools:\n  submit_review: false\n  submit_final_review: false\n---\n\nPrompt"
	inline := []string{prompt}
	result, err := Load("TEST_SUB_AGENT_PATHS", "reviewer", ".", nil, inline)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if result[0].Prompt != prompt {
		t.Errorf("prompt was modified when restrictions already present:\ngot:\n%s\nwant:\n%s", result[0].Prompt, prompt)
	}
}

func TestLoad_EnsureToolRestrictions_NormalizesListToolsWhenPresent(t *testing.T) {
	t.Setenv("TEST_SUB_AGENT_PATHS", "")

	prompt := "---\ndescription: \"test\"\nmode: subagent\ntools:\n  - type: read\n    allowed: true\n  - type: submit_review\n    allowed: false\n  - type: submit_final_review\n    allowed: false\n---\n\nPrompt"
	inline := []string{prompt}
	result, err := Load("TEST_SUB_AGENT_PATHS", "reviewer", ".", nil, inline)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if strings.Contains(result[0].Prompt, "- type:") {
		t.Errorf("expected list tools to be normalized, got:\n%s", result[0].Prompt)
	}
	for _, want := range []string{"read: true", "submit_review: false", "submit_final_review: false"} {
		if !strings.Contains(result[0].Prompt, want) {
			t.Errorf("expected normalized prompt to contain %q, got:\n%s", want, result[0].Prompt)
		}
	}
}

func TestLoad_EnsureToolRestrictions_NormalizesAndExtendsListTools(t *testing.T) {
	t.Setenv("TEST_SUB_AGENT_PATHS", "")

	prompt := "---\ndescription: \"test\"\nmode: subagent\ntools:\n  - type: read\n    allowed: true\n---\n\nPrompt"
	inline := []string{prompt}
	result, err := Load("TEST_SUB_AGENT_PATHS", "reviewer", ".", nil, inline)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if strings.Contains(result[0].Prompt, "- type:") {
		t.Errorf("expected list tools to be normalized, got:\n%s", result[0].Prompt)
	}
	if !strings.Contains(result[0].Prompt, "read: true") {
		t.Error("expected read permission to be preserved")
	}
	if !strings.Contains(result[0].Prompt, "submit_review: false") {
		t.Error("expected submit_review restriction")
	}
	if !strings.Contains(result[0].Prompt, "submit_final_review: false") {
		t.Error("expected submit_final_review restriction")
	}
}

func TestLoadWithOptions_DisablesLegacyEnv(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, "env.md")
	tomlPath := filepath.Join(dir, "toml.md")
	envPrompt := "---\nmode: subagent\n---\nenv"
	tomlPrompt := "---\nmode: subagent\n---\ntoml"
	if err := os.WriteFile(envPath, []byte(envPrompt), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tomlPath, []byte(tomlPrompt), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("TEST_SUB_AGENT_PATHS", envPath)

	result, err := LoadWithOptions("TEST_SUB_AGENT_PATHS", "reviewer", dir, []string{"toml.md"}, nil, Options{UseLegacyEnv: false})
	if err != nil {
		t.Fatalf("LoadWithOptions: %v", err)
	}
	if len(result) != 1 || result[0].Name != "toml" || !strings.Contains(result[0].Prompt, "toml") {
		t.Fatalf("result = %+v, want TOML path when legacy env is disabled", result)
	}
}
