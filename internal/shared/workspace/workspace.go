package workspace

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

const (
	defaultMaxSteps = 50
	keepXDGDirsEnv  = "OR_OPENCODE_KEEP_XDG_DIRS"
	opencodeSubdir  = "opencode"
	agentsDir       = "opencode/agents"
	toolsDir        = "opencode/tools"
	xdgCacheSubdir  = "cache"
	xdgDataSubdir   = "data"
	xdgStateSubdir  = "state"
)

// Workspace manages a temporary directory used as XDG_CONFIG_HOME for opencode.
// Structure: <temp-dir>/opencode/opencode.json and agent-specific files.
type Workspace struct {
	dir string
}

type AgentSpec struct {
	Name         string
	ToolFileName string
	ToolContent  []byte
	Prompt       string
}

// NewAgent creates a workspace for a single opencode agent.
func NewAgent(cfg Config, spec AgentSpec) (*Workspace, error) {
	return newWorkspace(cfg, spec)
}

// newWorkspace writes opencode.json, tools/<toolFile>, agents/<agentName>.md.
func newWorkspace(cfg Config, spec AgentSpec) (*Workspace, error) {
	dir, err := os.MkdirTemp("", "opencode-review-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}

	ocDir := filepath.Join(dir, opencodeSubdir)
	if err := os.MkdirAll(ocDir, 0o750); err != nil {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("create opencode dir: %w", err)
	}
	seedOpenCodeCaches(dir)

	defaultAgent := ""
	if spec.Prompt != "" {
		if spec.Name == "" {
			_ = os.RemoveAll(dir)
			return nil, fmt.Errorf("agent name is required when prompt is set")
		}
		if spec.ToolFileName == "" {
			_ = os.RemoveAll(dir)
			return nil, fmt.Errorf("tool file name is required when prompt is set")
		}
		defaultAgent = spec.Name
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

	if spec.Prompt != "" {
		toolsPath := filepath.Join(dir, toolsDir)
		if err := os.MkdirAll(toolsPath, 0o750); err != nil {
			_ = os.RemoveAll(dir)
			return nil, fmt.Errorf("create tools dir: %w", err)
		}

		if err := os.WriteFile(filepath.Join(toolsPath, spec.ToolFileName), spec.ToolContent, 0o600); err != nil {
			_ = os.RemoveAll(dir)
			return nil, fmt.Errorf("write %s: %w", spec.ToolFileName, err)
		}
		for name, content := range cfg.ToolOverrides {
			if err := os.WriteFile(filepath.Join(toolsPath, name), content, 0o600); err != nil {
				_ = os.RemoveAll(dir)
				return nil, fmt.Errorf("write tool override %q: %w", name, err)
			}
		}

		if err := writeAgentFile(dir, spec.Name, cfg, spec.Prompt); err != nil {
			_ = os.RemoveAll(dir)
			return nil, fmt.Errorf("write %s agent file: %w", spec.Name, err)
		}
	}

	for _, sa := range cfg.SubAgents {
		if err := writeSubAgentFile(dir, sa.Name, sa.Prompt); err != nil {
			_ = os.RemoveAll(dir)
			return nil, fmt.Errorf("write sub-agent %q: %w", sa.Name, err)
		}
	}

	slog.Debug("workspace created",
		"dir", dir,
		"config_path", configPath,
		"config_content", string(configData),
		"has_prompt", spec.Prompt != "",
		"has_provider_json", len(cfg.ProviderJSON) > 0,
		"sub_agents", len(cfg.SubAgents),
	)

	return &Workspace{dir: dir}, nil
}

// Dir returns the workspace directory path (for XDG_CONFIG_HOME).
func (w *Workspace) Dir() string {
	return w.dir
}

// CacheDir returns the workspace cache directory path (for XDG_CACHE_HOME).
func (w *Workspace) CacheDir() string {
	if w == nil || w.dir == "" {
		return ""
	}
	return filepath.Join(w.dir, xdgCacheSubdir)
}

// DataDir returns the workspace data directory path (for XDG_DATA_HOME).
func (w *Workspace) DataDir() string {
	if w == nil || w.dir == "" {
		return ""
	}
	return filepath.Join(w.dir, xdgDataSubdir)
}

// StateDir returns the workspace state directory path (for XDG_STATE_HOME).
func (w *Workspace) StateDir() string {
	if w == nil || w.dir == "" {
		return ""
	}
	return filepath.Join(w.dir, xdgStateSubdir)
}

// Cleanup removes the temporary workspace directory.
func (w *Workspace) Cleanup() error {
	if w == nil || w.dir == "" {
		return nil
	}
	if keepXDGDirs() {
		slog.Info("workspace cleanup skipped", "dir", w.dir, "env", keepXDGDirsEnv)
		return nil
	}
	return os.RemoveAll(w.dir)
}

func keepXDGDirs() bool {
	switch os.Getenv(keepXDGDirsEnv) {
	case "true", "1":
		return true
	default:
		return false
	}
}

func buildOpenCodeConfig(cfg Config, defaultAgent string) ([]byte, error) {
	maxSteps := cfg.MaxSteps
	if maxSteps == 0 {
		maxSteps = defaultMaxSteps
	}

	model := cfg.Model
	content := map[string]any{
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
		if model == "" {
			if providerModel, ok := providerData["model"].(string); ok {
				model = providerModel
			}
		}
	}
	if model != "" {
		content["model"] = model
	}

	return json.MarshalIndent(content, "", "  ")
}

func resolveModel(cfg Config) string {
	if cfg.Model != "" {
		return cfg.Model
	}
	if len(cfg.ProviderJSON) == 0 {
		return ""
	}
	var providerData struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(cfg.ProviderJSON, &providerData); err != nil {
		return ""
	}
	return providerData.Model
}
