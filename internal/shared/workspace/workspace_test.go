package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "opencode-review-workspace-test-home-*")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("HOME", home)
	_ = os.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	_ = os.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	_ = os.Setenv("BUN_INSTALL_CACHE_DIR", filepath.Join(home, ".bun", "install", "cache"))
	code := m.Run()
	_ = os.RemoveAll(home)
	os.Exit(code)
}

func newReviewer(cfg Config, prompt string) (*Workspace, error) {
	return NewAgent(cfg, AgentSpec{
		Name:         "reviewer",
		ToolFileName: "submit_review.ts",
		ToolContent:  []byte("@opencode-ai/plugin reviewer"),
		Prompt:       prompt,
	})
}

func newFinalizer(cfg Config, prompt string) (*Workspace, error) {
	return NewAgent(cfg, AgentSpec{
		Name:         "finalizer",
		ToolFileName: "submit_final_review.ts",
		ToolContent:  []byte("@opencode-ai/plugin finalizer"),
		Prompt:       prompt,
	})
}

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

	ws, err := newReviewer(Config{
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

func TestNew_ModelDefaultsFromProviderJSON(t *testing.T) {
	providerJSON := json.RawMessage(`{
		"provider": {"my-proxy": {"models": {"deepseek-v4-flash": {"name": "DeepSeek"}}}},
		"model": "my-proxy/deepseek-v4-flash"
	}`)

	ws, err := newReviewer(Config{
		ProviderJSON: providerJSON,
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
	if parsed["model"] != "my-proxy/deepseek-v4-flash" {
		t.Fatalf("model = %v, want provider JSON model", parsed["model"])
	}

	agentPath := filepath.Join(ws.Dir(), "opencode", "agents", "reviewer.md")
	agentData, err := os.ReadFile(agentPath)
	if err != nil {
		t.Fatalf("read agent file: %v", err)
	}
	if !strings.Contains(string(agentData), "model: my-proxy/deepseek-v4-flash") {
		t.Fatal("agent file missing provider JSON model in frontmatter")
	}
}

func TestNew_ModelOverrideWinsOverProviderJSON(t *testing.T) {
	providerJSON := json.RawMessage(`{
		"provider": {"my-proxy": {"models": {"deepseek-v4-flash": {"name": "DeepSeek"}}}},
		"model": "my-proxy/deepseek-v4-flash"
	}`)

	ws, err := newReviewer(Config{
		ProviderJSON: providerJSON,
		Model:        "my-proxy/override",
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
	if parsed["model"] != "my-proxy/override" {
		t.Fatalf("model = %v, want explicit override", parsed["model"])
	}
}

func TestNew_NoProvider(t *testing.T) {
	ws, err := newReviewer(Config{
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
	ws, err := newReviewer(Config{Model: "test/model"}, "Review code.")
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
	ws, err := newReviewer(Config{Model: "test/model"}, "")
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
	ws, err := newFinalizer(Config{
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

	// finalizer.md should exist
	finalizerAgentPath := filepath.Join(ws.Dir(), "opencode", "agents", "finalizer.md")
	finalizerData, err := os.ReadFile(finalizerAgentPath)
	if err != nil {
		t.Fatalf("read finalizer.md: %v", err)
	}
	if !strings.Contains(string(finalizerData), "You are the final review editor.") {
		t.Error("finalizer.md missing prompt content")
	}

	// submit_review.ts should NOT exist
	reviewToolPath := filepath.Join(ws.Dir(), "opencode", "tools", "submit_review.ts")
	if _, err := os.Stat(reviewToolPath); !os.IsNotExist(err) {
		t.Error("submit_review.ts should not exist in finalizer workspace")
	}

	// opencode.json should have default_agent = "finalizer"
	configPath := filepath.Join(ws.Dir(), "opencode", "opencode.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if parsed["default_agent"] != "finalizer" {
		t.Errorf("default_agent = %v, want finalizer", parsed["default_agent"])
	}
}

func TestNew_NoAgentPrompt(t *testing.T) {
	ws, err := newReviewer(Config{
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
	ws, err := newReviewer(Config{Model: "test/model"}, "")
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
	if def["steps"] != float64(50) {
		t.Errorf("default steps = %v, want 50", def["steps"])
	}
}

func TestCleanup(t *testing.T) {
	ws, err := newReviewer(Config{Model: "test/model"}, "")
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

func TestCleanup_KeepXDGDirs(t *testing.T) {
	for _, value := range []string{"true", "1"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("OR_OPENCODE_KEEP_XDG_DIRS", value)

			ws, err := newReviewer(Config{Model: "test/model"}, "")
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			t.Cleanup(func() { _ = os.RemoveAll(ws.Dir()) })

			dir := ws.Dir()
			if err := ws.Cleanup(); err != nil {
				t.Fatalf("Cleanup: %v", err)
			}

			if _, err := os.Stat(dir); err != nil {
				t.Fatalf("directory should be kept after Cleanup: %v", err)
			}
		})
	}
}

func TestCleanup_Nil(t *testing.T) {
	var ws *Workspace
	if err := ws.Cleanup(); err != nil {
		t.Fatalf("Cleanup on nil should not error: %v", err)
	}
}

func TestXDGDirs(t *testing.T) {
	ws, err := newReviewer(Config{Model: "test/model"}, "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = ws.Cleanup() }()

	if got, want := ws.CacheDir(), filepath.Join(ws.Dir(), "cache"); got != want {
		t.Fatalf("CacheDir = %q, want %q", got, want)
	}
	if got, want := ws.DataDir(), filepath.Join(ws.Dir(), "data"); got != want {
		t.Fatalf("DataDir = %q, want %q", got, want)
	}
	if got, want := ws.StateDir(), filepath.Join(ws.Dir(), "state"); got != want {
		t.Fatalf("StateDir = %q, want %q", got, want)
	}
}

func TestNew_SeedsOpenCodeCaches(t *testing.T) {
	home := t.TempDir()
	xdgConfig := filepath.Join(home, ".config")
	xdgCache := filepath.Join(home, ".cache")
	bunCache := filepath.Join(home, ".bun", "install", "cache")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdgConfig)
	t.Setenv("XDG_CACHE_HOME", xdgCache)
	t.Setenv("BUN_INSTALL_CACHE_DIR", bunCache)

	sourceConfig := filepath.Join(xdgConfig, "opencode")
	writeTestFile(t, filepath.Join(sourceConfig, "node_modules", "pkg", "index.js"), "module")
	writeTestFile(t, filepath.Join(sourceConfig, "package.json"), `{"name":"opencode"}`)
	writeTestFile(t, filepath.Join(sourceConfig, "bun.lock"), "lock")
	writeTestFile(t, filepath.Join(sourceConfig, "opencode.json"), `{"bad":true}`)
	writeTestFile(t, filepath.Join(sourceConfig, "agents", "reviewer.md"), "bad agent")
	writeTestFile(t, filepath.Join(sourceConfig, "tools", "submit_review.ts"), "bad tool")
	writeTestFile(t, filepath.Join(sourceConfig, "sub-agents", "helper.md"), "bad sub-agent")
	writeTestFile(t, filepath.Join(xdgCache, "opencode", "provider", "cache.txt"), "provider")
	writeTestFile(t, filepath.Join(xdgCache, "opencode", "snapshot", "bad.txt"), "snapshot")
	writeTestFile(t, filepath.Join(xdgCache, "opencode", "state", "bad.txt"), "state")
	writeTestFile(t, filepath.Join(bunCache, "ai-sdk", "pkg.tgz"), "bun")

	ws, err := newReviewer(Config{Model: "test/model"}, "Review code.")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = ws.Cleanup() }()

	for path, want := range map[string]string{
		filepath.Join(ws.Dir(), "opencode", "node_modules", "pkg", "index.js"):           "module",
		filepath.Join(ws.Dir(), "opencode", "package.json"):                              `{"name":"opencode"}`,
		filepath.Join(ws.Dir(), "opencode", "bun.lock"):                                  "lock",
		filepath.Join(ws.Dir(), "cache", "opencode", "provider", "cache.txt"):            "provider",
		filepath.Join(ws.Dir(), "cache", "bun", "install", "cache", "ai-sdk", "pkg.tgz"): "bun",
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read seeded file %s: %v", path, err)
		}
		if string(data) != want {
			t.Fatalf("%s = %q, want %q", path, string(data), want)
		}
	}

	configData, err := os.ReadFile(filepath.Join(ws.Dir(), "opencode", "opencode.json"))
	if err != nil {
		t.Fatalf("read generated config: %v", err)
	}
	if strings.Contains(string(configData), `"bad"`) {
		t.Fatal("source opencode.json should not overwrite generated config")
	}

	agentData, err := os.ReadFile(filepath.Join(ws.Dir(), "opencode", "agents", "reviewer.md"))
	if err != nil {
		t.Fatalf("read generated agent: %v", err)
	}
	if strings.Contains(string(agentData), "bad agent") {
		t.Fatal("source agents should not overwrite generated agents")
	}

	toolData, err := os.ReadFile(filepath.Join(ws.Dir(), "opencode", "tools", "submit_review.ts"))
	if err != nil {
		t.Fatalf("read generated tool: %v", err)
	}
	if strings.Contains(string(toolData), "bad tool") {
		t.Fatal("source tools should not overwrite generated tools")
	}
	if _, err := os.Stat(filepath.Join(ws.Dir(), "opencode", "sub-agents", "helper.md")); !os.IsNotExist(err) {
		t.Fatal("source sub-agents should not be copied")
	}
	if _, err := os.Stat(filepath.Join(ws.Dir(), "cache", "opencode", "snapshot", "bad.txt")); !os.IsNotExist(err) {
		t.Fatal("source snapshot cache should not be copied")
	}
	if _, err := os.Stat(filepath.Join(ws.Dir(), "cache", "opencode", "state", "bad.txt")); !os.IsNotExist(err) {
		t.Fatal("source state cache should not be copied")
	}
}

func TestNew_MissingSourceCachesAreIgnored(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("BUN_INSTALL_CACHE_DIR", filepath.Join(home, ".bun", "install", "cache"))

	ws, err := newReviewer(Config{Model: "test/model"}, "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = ws.Cleanup() }()

	if _, err := os.Stat(filepath.Join(ws.Dir(), "opencode", "opencode.json")); err != nil {
		t.Fatalf("generated config should still exist: %v", err)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestNew_ToolOverrides(t *testing.T) {
	ws, err := newReviewer(Config{
		Model: "test/model",
		ToolOverrides: map[string][]byte{
			"submit_review.ts": []byte("override content"),
			"custom.ts":        []byte("custom tool"),
		},
	}, "Review code.")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = ws.Cleanup() }()

	overridePath := filepath.Join(ws.Dir(), "opencode", "tools", "submit_review.ts")
	overrideData, err := os.ReadFile(overridePath)
	if err != nil {
		t.Fatalf("read override tool: %v", err)
	}
	if string(overrideData) != "override content" {
		t.Fatalf("submit_review.ts = %q, want override content", string(overrideData))
	}

	customPath := filepath.Join(ws.Dir(), "opencode", "tools", "custom.ts")
	customData, err := os.ReadFile(customPath)
	if err != nil {
		t.Fatalf("read custom tool: %v", err)
	}
	if string(customData) != "custom tool" {
		t.Fatalf("custom.ts = %q, want custom tool", string(customData))
	}
}
