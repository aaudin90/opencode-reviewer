package agentconfig

import (
	"fmt"
	"strings"

	"github.com/aaudin90/opencode-reviewer/internal/envconfig"
)

type Options struct {
	UseLegacyEnv      bool
	LegacyEnvFallback bool
}

// Load reads agent prompt with legacy env priority for backward compatibility.
// Use LoadWithOptions with LegacyEnvFallback for the CLI's deprecated env fallback mode.
func Load(configPath string, inlinePrompt string) (string, error) {
	return LoadWithOptions(configPath, inlinePrompt, Options{UseLegacyEnv: true})
}

func LoadWithOptions(configPath string, inlinePrompt string, opts Options) (string, error) {
	data, err := envconfig.ResolveWithOptions(
		"OR_AGENT_PROMPT_PATH",
		inlinePrompt,
		configPath,
		envconfig.Options{UseEnv: opts.UseLegacyEnv, EnvFallback: opts.LegacyEnvFallback},
	)
	if err != nil {
		return "", fmt.Errorf("load agent prompt: %w", err)
	}
	if strings.TrimSpace(data) == "" {
		return defaultPrompt, nil
	}
	return data, nil
}
