package agentconfig

import (
	"fmt"
	"strings"

	"github.com/aaudin90/opencode-reviewer/internal/envconfig"
)

// Load reads agent prompt by priority:
//
//	OR_AGENT_PROMPT_PATH env (file) > inlinePrompt (TOML inline) > configPath (TOML path) > built-in default.
func Load(configPath string, inlinePrompt string) (string, error) {
	data, err := envconfig.Resolve("OR_AGENT_PROMPT_PATH", inlinePrompt, configPath)
	if err != nil {
		return "", fmt.Errorf("load agent prompt: %w", err)
	}
	if strings.TrimSpace(data) == "" {
		return defaultPrompt, nil
	}
	return data, nil
}
