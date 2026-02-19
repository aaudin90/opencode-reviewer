package workspace

import (
	"fmt"
	"os"
	"path/filepath"
)

func writeAgentFile(dir string, cfg Config) error {
	agentDir := filepath.Join(dir, agentsDir)
	if err := os.MkdirAll(agentDir, 0o750); err != nil {
		return fmt.Errorf("create agents dir: %w", err)
	}

	content := fmt.Sprintf(`---
description: "Automated code reviewer"
model: %s
permission:
  edit: deny
  bash:
    "*": deny
---

%s
`, cfg.Model, cfg.AgentPrompt)

	path := filepath.Join(agentDir, agentName+".md")
	return os.WriteFile(path, []byte(content), 0o600)
}
