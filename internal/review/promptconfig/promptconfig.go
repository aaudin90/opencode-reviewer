package promptconfig

import (
	"fmt"
	"os"
	"strings"

	"github.com/aaudin90/opencode-reviewer/internal/review/promptref"
	"github.com/aaudin90/opencode-reviewer/internal/shared/models"
)

type Options struct {
	UseLegacyEnv      bool
	LegacyEnvFallback bool
}

// Load resolves reviewer messages with legacy env priority for backward compatibility.
// Use LoadWithOptions with LegacyEnvFallback for the CLI's deprecated env fallback mode.
func Load(configDir string, tomlPaths []string, tomlInline []string) ([]string, error) {
	return LoadWithOptions(configDir, tomlPaths, tomlInline, Options{UseLegacyEnv: true})
}

func LoadWithOptions(configDir string, tomlPaths []string, tomlInline []string, opts Options) ([]string, error) {
	messages, err := LoadReviewMessagesWithOptions(configDir, tomlPaths, tomlInline, opts)
	if err != nil {
		return nil, err
	}
	if messages == nil {
		return nil, nil
	}
	result := make([]string, 0, len(messages))
	for _, msg := range messages {
		result = append(result, msg.Content)
	}
	return result, nil
}

func LoadReviewMessagesWithOptions(configDir string, tomlPaths []string, tomlInline []string, opts Options) ([]models.ReviewMessage, error) {
	if opts.UseLegacyEnv && !opts.LegacyEnvFallback {
		if raw := os.Getenv("OR_MESSAGE_PATHS"); raw != "" {
			cwd, err := os.Getwd()
			if err != nil {
				return nil, fmt.Errorf("get working directory: %w", err)
			}
			return promptref.LoadReviewMessages(cwd, splitPaths(raw), nil)
		}
	}
	if len(tomlInline) > 0 {
		return promptref.LoadReviewMessages(configDir, nil, tomlInline)
	}
	if len(tomlPaths) > 0 {
		return promptref.LoadReviewMessages(configDir, tomlPaths, nil)
	}
	if opts.UseLegacyEnv && opts.LegacyEnvFallback {
		if raw := os.Getenv("OR_MESSAGE_PATHS"); raw != "" {
			cwd, err := os.Getwd()
			if err != nil {
				return nil, fmt.Errorf("get working directory: %w", err)
			}
			return promptref.LoadReviewMessages(cwd, splitPaths(raw), nil)
		}
	}
	return nil, nil
}

func splitPaths(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			result = append(result, t)
		}
	}
	return result
}
