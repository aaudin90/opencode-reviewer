package workspace

import (
	"fmt"
	"os"
	"path/filepath"
)

func writeAgentFile(dir, name string, cfg Config, prompt string) error {
	agentDir := filepath.Join(dir, agentsDir)
	if err := os.MkdirAll(agentDir, 0o750); err != nil {
		return fmt.Errorf("create agents dir: %w", err)
	}

	model := resolveModel(cfg)
	content := fmt.Sprintf(`---
description: "Automated code reviewer"
model: %s
permission:
  edit: deny
  bash:
    "*": deny
---

%s
`, model, prompt)

	path := filepath.Join(agentDir, name+".md")
	return os.WriteFile(path, []byte(content), 0o600)
}

func writeSubAgentFile(dir, name string, prompt string) error {
	agentDir := filepath.Join(dir, agentsDir)
	if err := os.MkdirAll(agentDir, 0o750); err != nil {
		return fmt.Errorf("create agents dir: %w", err)
	}

	path := filepath.Join(agentDir, name+".md")
	return os.WriteFile(path, []byte(prompt), 0o600)
}
