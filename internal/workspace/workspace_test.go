package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNew_FullConfig(t *testing.T) {
	providerJSON := json.RawMessage(`{
		"provider": {
			"my-proxy": {
				"npm": "@ai-sdk/openai-compatible",
				"models": {
					"claude-sonnet": {"name": "Claude Sonnet"}
				}
			}
		},
		"model": "my-proxy/claude-sonnet"
	}`)

	ws, err := NewReviewer(Config{
		ProviderJSON: providerJSON,
		Model:        "my-proxy/claude-sonnet",
		MaxSteps:     50,
	}, "You are a code reviewer.")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = ws.Cleanup() }()

	if ws.Dir() == "" {
		t.Fatal("Dir() is empty")
	}

	// Verify opencode/opencode.json
	configPath := filepath.Join(ws.Dir(), "opencode", "opencode.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("parse config: %v", err)
	}

	if parsed["model"] != "my-proxy/claude-sonnet" {
		t.Errorf("model = %v, want my-proxy/claude-sonnet", parsed["model"])
	}
	if parsed["default_agent"] != "reviewer" {
		t.Errorf("default_agent = %v, want reviewer", parsed["default_agent"])
	}
	if _, ok := parsed["provider"]; !ok {
		t.Error("provider section missing")
	}

	perm, ok := parsed["permission"].(map[string]any)
	if !ok {
		t.Fatal("permission is not a map")
	}
	if perm["read"] != "allow" {
		t.Errorf("permission.read = %v, want allow", perm["read"])
	}
	if perm["edit"] != "deny" {
		t.Errorf("permission.edit = %v, want deny", perm["edit"])
	}

	agent, ok := parsed["agent"].(map[string]any)
	if !ok {
		t.Fatal("agent is not a map")
	}
	def, ok := agent["default"].(map[string]any)
	if !ok {
		t.Fatal("agent.default is not a map")
	}
	if def["steps"] != float64(50) {
		t.Errorf("steps = %v, want 50", def["steps"])
	}

	// Verify opencode/agents/reviewer.md
	agentPath := filepath.Join(ws.Dir(), "opencode", "agents", "reviewer.md")
	agentData, err := os.ReadFile(agentPath)
	if err != nil {
		t.Fatalf("read agent file: %v", err)
	}
	agentContent := string(agentData)
	if !strings.Contains(agentContent, "You are a code reviewer.") {
		t.Error("agent file missing prompt content")
	}
	if !strings.Contains(agentContent, "model: my-proxy/claude-sonnet") {
		t.Error("agent file missing model in frontmatter")
	}

	// Verify opencode/tools/submit_review.ts
	toolPath := filepath.Join(ws.Dir(), "opencode", "tools", "submit_review.ts")
	toolData, err := os.ReadFile(toolPath)
	if err != nil {
		t.Fatalf("read submit_review.ts: %v", err)
	}
	if !strings.Contains(string(toolData), "@opencode-ai/plugin") {
		t.Error("submit_review.ts missing expected content")
	}

	// Verify submit_final_review.ts is NOT written for reviewer workspace
	finalToolPath := filepath.Join(ws.Dir(), "opencode", "tools", "submit_final_review.ts")
	if _, err := os.Stat(finalToolPath); !os.IsNotExist(err) {
		t.Error("submit_final_review.ts should not exist in reviewer workspace")
	}
}

func TestNew_NoProvider(t *testing.T) {
	ws, err := NewReviewer(Config{
		Model: "test/model",
	}, "Review code.")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = ws.Cleanup() }()

	configPath := filepath.Join(ws.Dir(), "opencode", "opencode.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("parse config: %v", err)
	}

	if _, ok := parsed["provider"]; ok {
		t.Error("provider should not be present when ProviderJSON is nil")
	}
}

func TestNew_ReviewerToolWrittenWithAgentPrompt(t *testing.T) {
	ws, err := NewReviewer(Config{Model: "test/model"}, "Review code.")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = ws.Cleanup() }()

	toolPath := filepath.Join(ws.Dir(), "opencode", "tools", "submit_review.ts")
	if _, err := os.Stat(toolPath); err != nil {
		t.Errorf("submit_review.ts should be written when AgentPrompt is set: %v", err)
	}
}

func TestNew_ReviewerToolNotWrittenWithoutAgentPrompt(t *testing.T) {
	ws, err := NewReviewer(Config{Model: "test/model"}, "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = ws.Cleanup() }()

	toolPath := filepath.Join(ws.Dir(), "opencode", "tools", "submit_review.ts")
	if _, err := os.Stat(toolPath); !os.IsNotExist(err) {
		t.Error("submit_review.ts should not be written when AgentPrompt is empty")
	}
}

func TestNew_FinalizerWorkspace(t *testing.T) {
	ws, err := NewFinalizer(Config{
		Model: "test/model",
	}, "You are the final review editor.")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = ws.Cleanup() }()

	// submit_final_review.ts should exist
	finalToolPath := filepath.Join(ws.Dir(), "opencode", "tools", "submit_final_review.ts")
	finalToolData, err := os.ReadFile(finalToolPath)
	if err != nil {
		t.Fatalf("read submit_final_review.ts: %v", err)
	}
	if !strings.Contains(string(finalToolData), "@opencode-ai/plugin") {
		t.Error("submit_final_review.ts missing expected content")
	}

	// reviewer.md should exist
	finalizerAgentPath := filepath.Join(ws.Dir(), "opencode", "agents", "reviewer.md")
	finalizerData, err := os.ReadFile(finalizerAgentPath)
	if err != nil {
		t.Fatalf("read reviewer.md: %v", err)
	}
	if !strings.Contains(string(finalizerData), "You are the final review editor.") {
		t.Error("reviewer.md missing prompt content")
	}

	// submit_review.ts should NOT exist
	reviewToolPath := filepath.Join(ws.Dir(), "opencode", "tools", "submit_review.ts")
	if _, err := os.Stat(reviewToolPath); !os.IsNotExist(err) {
		t.Error("submit_review.ts should not exist in finalizer workspace")
	}

	// opencode.json should have default_agent = "reviewer"
	configPath := filepath.Join(ws.Dir(), "opencode", "opencode.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if parsed["default_agent"] != "reviewer" {
		t.Errorf("default_agent = %v, want reviewer", parsed["default_agent"])
	}
}

func TestNew_NoAgentPrompt(t *testing.T) {
	ws, err := NewReviewer(Config{
		Model: "test/model",
	}, "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = ws.Cleanup() }()

	agentPath := filepath.Join(ws.Dir(), "opencode", "agents", "reviewer.md")
	if _, err := os.Stat(agentPath); !os.IsNotExist(err) {
		t.Error("agent file should not exist when AgentPrompt is empty")
	}

	configPath := filepath.Join(ws.Dir(), "opencode", "opencode.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("parse config: %v", err)
	}

	if _, ok := parsed["default_agent"]; ok {
		t.Error("default_agent should not be set when no agent prompt")
	}
}

func TestNew_DefaultMaxSteps(t *testing.T) {
	ws, err := NewReviewer(Config{Model: "test/model"}, "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = ws.Cleanup() }()

	configPath := filepath.Join(ws.Dir(), "opencode", "opencode.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("parse config: %v", err)
	}

	agent := parsed["agent"].(map[string]any)
	def := agent["default"].(map[string]any)
	if def["steps"] != float64(30) {
		t.Errorf("default steps = %v, want 30", def["steps"])
	}
}

func TestCleanup(t *testing.T) {
	ws, err := NewReviewer(Config{Model: "test/model"}, "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	dir := ws.Dir()
	if err := ws.Cleanup(); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("directory should be removed after Cleanup")
	}
}

func TestCleanup_Nil(t *testing.T) {
	var ws *Workspace
	if err := ws.Cleanup(); err != nil {
		t.Fatalf("Cleanup on nil should not error: %v", err)
	}
}
