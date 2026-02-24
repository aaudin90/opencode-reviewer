package workspace

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

const (
	defaultMaxSteps    = 30
	reviewerAgentName  = "reviewer"
	finalizerAgentName = "reviewer" // TODO 79400
	opencodeSubdir     = "opencode"
	agentsDir          = "opencode/agents"
	toolsDir           = "opencode/tools"
)

// Workspace manages a temporary directory used as XDG_CONFIG_HOME for opencode.
// Structure: <temp-dir>/opencode/opencode.json and agent-specific files.
type Workspace struct {
	dir string
}

// NewReviewer creates a reviewer workspace: agents/reviewer.md + tools/submit_review.ts.
func NewReviewer(cfg Config, agentPrompt string) (*Workspace, error) {
	return newWorkspace(cfg, reviewerAgentName, "submit_review.ts", submitReviewTS, agentPrompt)
}

// NewFinalizer creates a finalizer workspace: agents/reviewer.md + tools/submit_final_review.ts.
func NewFinalizer(cfg Config, finalizerPrompt string) (*Workspace, error) {
	return newWorkspace(cfg, finalizerAgentName, "submit_final_review.ts", submitFinalReviewTS, finalizerPrompt)
}

// newWorkspace is the common constructor: writes opencode.json, tools/<toolFile>, agents/<agentName>.md.
func newWorkspace(cfg Config, agentName, toolFileName string, toolContent []byte, prompt string) (*Workspace, error) {
	dir, err := os.MkdirTemp("", "opencode-review-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}

	ocDir := filepath.Join(dir, opencodeSubdir)
	if err := os.MkdirAll(ocDir, 0o750); err != nil {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("create opencode dir: %w", err)
	}

	defaultAgent := ""
	if prompt != "" {
		defaultAgent = agentName
	}

	configPath := filepath.Join(ocDir, "opencode.json")
	configData, err := buildOpenCodeConfig(cfg, defaultAgent)
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("build opencode config: %w", err)
	}

	if err := os.WriteFile(configPath, configData, 0o600); err != nil {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("write opencode.json: %w", err)
	}

	if prompt != "" {
		toolsPath := filepath.Join(dir, toolsDir)
		if err := os.MkdirAll(toolsPath, 0o750); err != nil {
			_ = os.RemoveAll(dir)
			return nil, fmt.Errorf("create tools dir: %w", err)
		}

		if err := os.WriteFile(filepath.Join(toolsPath, toolFileName), toolContent, 0o600); err != nil {
			_ = os.RemoveAll(dir)
			return nil, fmt.Errorf("write %s: %w", toolFileName, err)
		}

		if err := writeAgentFile(dir, agentName, cfg, prompt); err != nil {
			_ = os.RemoveAll(dir)
			return nil, fmt.Errorf("write %s agent file: %w", agentName, err)
		}
	}

	slog.Debug("workspace created",
		"dir", dir,
		"config_path", configPath,
		"config_content", string(configData),
		"has_prompt", prompt != "",
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

func buildOpenCodeConfig(cfg Config, defaultAgent string) ([]byte, error) {
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

	if defaultAgent != "" {
		content["default_agent"] = defaultAgent
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
