package agentconfig

import (
	"fmt"
	"strings"

	"github.com/aaudin90/opencode-reviewer/internal/envconfig"
)

// Load reads agent prompt from REVIEW_AGENT_CONFIG_PATH (priority),
// REVIEW_AGENT_CONFIG env var, or the given configPath fallback.
// Returns the built-in default prompt if no source is configured.
func Load(configPath string) (string, error) {
	data, err := envconfig.ReadEnvOrFile("REVIEW_AGENT_CONFIG_PATH", "REVIEW_AGENT_CONFIG", configPath)
	if err != nil {
		return "", fmt.Errorf("load agent config: %w", err)
	}
	if strings.TrimSpace(data) == "" {
		return defaultPrompt, nil
	}
	return data, nil
}
