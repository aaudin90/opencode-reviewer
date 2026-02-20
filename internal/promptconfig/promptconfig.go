package promptconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Load resolves prompt file paths from REVIEW_PROMPT_PATHS env (comma-separated)
// or from the tomlPaths list (resolved relative to configDir).
// Returns empty slice if neither is set — pipeline will use default prompt.
func Load(configDir string, tomlPaths []string) ([]string, error) {
	if raw := os.Getenv("REVIEW_PROMPT_PATHS"); raw != "" {
		return resolveAndValidate(splitPaths(raw), "")
	}
	if len(tomlPaths) > 0 {
		return resolveAndValidate(tomlPaths, configDir)
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

func resolveAndValidate(paths []string, baseDir string) ([]string, error) {
	result := make([]string, 0, len(paths))
	for _, p := range paths {
		abs := p
		if !filepath.IsAbs(p) {
			if baseDir == "" {
				abs = filepath.Clean(p)
			} else {
				abs = filepath.Join(baseDir, p)
			}
		}
		if _, err := os.Stat(abs); err != nil { // #nosec G703 -- path from trusted config or env
			return nil, fmt.Errorf("prompt file %q: %w", abs, err)
		}
		result = append(result, abs)
	}
	return result, nil
}
