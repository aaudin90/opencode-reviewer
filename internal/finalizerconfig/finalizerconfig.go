package finalizerconfig

import (
	"fmt"
	"strings"

	"github.com/aaudin90/opencode-reviewer/internal/envconfig"
)

// Load reads the finalizer agent prompt by priority:
//
//	REVIEW_FINALIZER_PROMPT_PATH env (file) > inlinePrompt (TOML inline) > configPath (TOML path) > built-in default.
func Load(configPath string, inlinePrompt string) (string, error) {
	data, err := envconfig.Resolve("REVIEW_FINALIZER_PROMPT_PATH", inlinePrompt, configPath)
	if err != nil {
		return "", fmt.Errorf("load finalizer prompt: %w", err)
	}
	if strings.TrimSpace(data) == "" {
		return defaultPrompt, nil
	}
	return data, nil
}

// LoadMessage reads the finalizer user message by priority:
//
//	REVIEW_FINALIZER_MESSAGE_PATH env (file) > inlineMessage (TOML inline) > configPath (TOML path) > built-in default.
func LoadMessage(configPath string, inlineMessage string) (string, error) {
	data, err := envconfig.Resolve("REVIEW_FINALIZER_MESSAGE_PATH", inlineMessage, configPath)
	if err != nil {
		return "", fmt.Errorf("load finalizer message: %w", err)
	}
	if strings.TrimSpace(data) == "" {
		return defaultMessage, nil
	}
	return data, nil
}
