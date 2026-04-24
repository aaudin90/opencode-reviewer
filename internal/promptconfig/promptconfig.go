package promptconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	if opts.UseLegacyEnv && !opts.LegacyEnvFallback {
		if raw := os.Getenv("OR_MESSAGE_PATHS"); raw != "" {
			cwd, err := os.Getwd()
			if err != nil {
				return nil, fmt.Errorf("get working directory: %w", err)
			}
			return readAll(splitPaths(raw), cwd)
		}
	}
	if len(tomlInline) > 0 {
		return tomlInline, nil
	}
	if len(tomlPaths) > 0 {
		return readAll(tomlPaths, configDir)
	}
	if opts.UseLegacyEnv && opts.LegacyEnvFallback {
		if raw := os.Getenv("OR_MESSAGE_PATHS"); raw != "" {
			cwd, err := os.Getwd()
			if err != nil {
				return nil, fmt.Errorf("get working directory: %w", err)
			}
			return readAll(splitPaths(raw), cwd)
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

func readAll(paths []string, baseDir string) ([]string, error) {
	result := make([]string, 0, len(paths))
	for _, p := range paths {
		abs := p
		if !filepath.IsAbs(p) {
			abs = filepath.Join(baseDir, p)
		}
		data, err := os.ReadFile(filepath.Clean(abs)) // #nosec G304 G703 -- path from trusted config or env
		if err != nil {
			return nil, fmt.Errorf("read message file %q: %w", abs, err)
		}
		result = append(result, string(data))
	}
	return result, nil
}
