package workspace

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

const (
	defaultMaxSteps = 30
	agentName       = "reviewer"
	opencodeSubdir  = "opencode"
	agentsDir       = "opencode/agents"
	toolsDir        = "opencode/tools"
)

// Workspace manages a temporary directory used as XDG_CONFIG_HOME for opencode.
// Structure: <temp-dir>/opencode/opencode.json and <temp-dir>/opencode/agents/reviewer.md.
type Workspace struct {
	dir string
}

// New creates a temporary workspace directory containing opencode.json,
// agents/reviewer.md, and tools/submit_review.ts based on the provided config.
func New(cfg Config) (*Workspace, error) {
	dir, err := os.MkdirTemp("", "opencode-review-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}

	ocDir := filepath.Join(dir, opencodeSubdir)
	if err := os.MkdirAll(ocDir, 0o750); err != nil {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("create opencode dir: %w", err)
	}

	configPath := filepath.Join(ocDir, "opencode.json")
	configData, err := buildOpenCodeConfig(cfg)
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("build opencode config: %w", err)
	}

	if err := os.WriteFile(configPath, configData, 0o600); err != nil {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("write opencode.json: %w", err)
	}

	toolsPath := filepath.Join(dir, toolsDir)
	if err := os.MkdirAll(toolsPath, 0o750); err != nil {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("create tools dir: %w", err)
	}

	if err := os.WriteFile(filepath.Join(toolsPath, "submit_review.ts"), submitReviewTS, 0o600); err != nil {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("write submit_review.ts: %w", err)
	}

	if cfg.AgentPrompt != "" {
		if err := writeAgentFile(dir, cfg); err != nil {
			_ = os.RemoveAll(dir)
			return nil, fmt.Errorf("write agent file: %w", err)
		}
	}

	slog.Debug("workspace created",
		"dir", dir,
		"config_path", configPath,
		"config_content", string(configData),
		"has_agent_prompt", cfg.AgentPrompt != "",
		"has_provider_json", len(cfg.ProviderJSON) > 0,
	)

	return &Workspace{dir: dir}, nil
}

// Dir returns the workspace directory path (for XDG_CONFIG_HOME).
func (w *Workspace) Dir() string {
	return w.dir
}

// Cleanup removes the temporary workspace directory.
func (w *Workspace) Cleanup() error {
	if w == nil || w.dir == "" {
		return nil
	}
	return os.RemoveAll(w.dir)
}

func buildOpenCodeConfig(cfg Config) ([]byte, error) {
	maxSteps := cfg.MaxSteps
	if maxSteps == 0 {
		maxSteps = defaultMaxSteps
	}

	content := map[string]any{
		"model": cfg.Model,
		"permission": map[string]any{
			"read": "allow",
			"glob": "allow",
			"grep": "allow",
			"edit": "deny",
			"bash": map[string]any{
				"*": "deny",
			},
		},
		"agent": map[string]any{
			"default": map[string]any{
				"steps": maxSteps,
			},
		},
	}

	if cfg.AgentPrompt != "" {
		content["default_agent"] = agentName
	}

	if len(cfg.ProviderJSON) > 0 {
		var providerData map[string]any
		if err := json.Unmarshal(cfg.ProviderJSON, &providerData); err != nil {
			return nil, fmt.Errorf("parse provider JSON: %w", err)
		}

		if p, ok := providerData["provider"]; ok {
			content["provider"] = p
		}
	}

	return json.MarshalIndent(content, "", "  ")
}
