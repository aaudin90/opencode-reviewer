package finalizerconfig

import (
	"fmt"
	"strings"

	"github.com/aaudin90/opencode-reviewer/internal/shared/envconfig"
)

type Options struct {
	UseLegacyEnv      bool
	LegacyEnvFallback bool
}

// Load reads the finalizer agent prompt with legacy env priority for backward compatibility.
// Use LoadWithOptions with LegacyEnvFallback for the CLI's deprecated env fallback mode.
func Load(configPath string, inlinePrompt string) (string, error) {
	return LoadWithOptions(configPath, inlinePrompt, Options{UseLegacyEnv: true})
}

func LoadWithOptions(configPath string, inlinePrompt string, opts Options) (string, error) {
	data, err := envconfig.ResolveWithOptions(
		"OR_FINALIZER_PROMPT_PATH",
		inlinePrompt,
		configPath,
		envconfig.Options{UseEnv: opts.UseLegacyEnv, EnvFallback: opts.LegacyEnvFallback},
	)
	if err != nil {
		return "", fmt.Errorf("load finalizer prompt: %w", err)
	}
	if strings.TrimSpace(data) == "" {
		return defaultPrompt, nil
	}
	return data, nil
}

// LoadMessage reads the finalizer user message with legacy env priority for backward compatibility.
// Use LoadMessageWithOptions with LegacyEnvFallback for the CLI's deprecated env fallback mode.
func LoadMessage(configPath string, inlineMessage string) (string, error) {
	return LoadMessageWithOptions(configPath, inlineMessage, Options{UseLegacyEnv: true})
}

func LoadMessageWithOptions(configPath string, inlineMessage string, opts Options) (string, error) {
	data, err := envconfig.ResolveWithOptions(
		"OR_FINALIZER_MESSAGE_PATH",
		inlineMessage,
		configPath,
		envconfig.Options{UseEnv: opts.UseLegacyEnv, EnvFallback: opts.LegacyEnvFallback},
	)
	if err != nil {
		return "", fmt.Errorf("load finalizer message: %w", err)
	}
	if strings.TrimSpace(data) == "" {
		return defaultMessage, nil
	}
	return data, nil
}
