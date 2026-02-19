package envconfig

import (
	"fmt"
	"os"
	"path/filepath"
)

// ReadEnvOrFile resolves a string value from three sources in priority order:
//  1. fileEnvKey env var — treated as a file path, content is read and returned
//  2. inlineEnvKey env var — value returned as-is
//  3. fallbackPath — file path from TOML config, content is read and returned
//
// Returns empty string without error if none of the sources are set.
func ReadEnvOrFile(fileEnvKey, inlineEnvKey, fallbackPath string) (string, error) {
	if path := os.Getenv(fileEnvKey); path != "" {
		cleanPath := filepath.Clean(path)
		data, err := os.ReadFile(cleanPath) // #nosec G304 G703 -- path from trusted env var
		if err != nil {
			return "", fmt.Errorf("read %s=%q: %w", fileEnvKey, path, err)
		}
		return string(data), nil
	}

	if val := os.Getenv(inlineEnvKey); val != "" {
		return val, nil
	}

	if fallbackPath != "" {
		cleanPath := filepath.Clean(fallbackPath)
		data, err := os.ReadFile(cleanPath) // #nosec G304 -- path from trusted TOML config
		if err != nil {
			return "", fmt.Errorf("read config path %q: %w", fallbackPath, err)
		}
		return string(data), nil
	}

	return "", nil
}
