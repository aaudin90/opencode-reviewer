package promptconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Load resolves reviewer messages by priority:
//  1. OR_MESSAGE_PATHS env (comma-separated file paths, relative to cwd) → read files → return contents
//  2. tomlInline (if non-empty) → return as-is
//  3. tomlPaths (relative to configDir) → read files → return contents
//  4. nil (caller decides what to do)
func Load(configDir string, tomlPaths []string, tomlInline []string) ([]string, error) {
	if raw := os.Getenv("OR_MESSAGE_PATHS"); raw != "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("get working directory: %w", err)
		}
		return readAll(splitPaths(raw), cwd)
	}
	if len(tomlInline) > 0 {
		return tomlInline, nil
	}
	if len(tomlPaths) > 0 {
		return readAll(tomlPaths, configDir)
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
