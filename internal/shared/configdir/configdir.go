package configdir

import (
	"fmt"
	"os"
	"path/filepath"
)

const DirectoryName = ".opencodereview"

// Resolve returns the effective config directory in priority order:
//  1. cliConfigDir
//  2. envConfigDir
//  3. auto-discovered <projectDir>/.opencodereview
func Resolve(cliConfigDir, envConfigDir, projectDir string, disableAutoDiscovery bool) (string, Source, error) {
	if cliConfigDir != "" {
		path, err := abs(cliConfigDir)
		if err != nil {
			return "", SourceNone, err
		}
		return path, SourceCLI, nil
	}

	if envConfigDir != "" {
		path, err := abs(envConfigDir)
		if err != nil {
			return "", SourceNone, err
		}
		return path, SourceEnv, nil
	}

	if disableAutoDiscovery {
		return "", SourceNone, nil
	}

	discovered, ok := Discover(projectDir)
	if !ok {
		return "", SourceNone, nil
	}
	path, err := abs(discovered)
	if err != nil {
		return "", SourceNone, err
	}
	return path, SourceAuto, nil
}

// Discover returns <projectDir>/.opencodereview when the directory exists.
func Discover(projectDir string) (string, bool) {
	if projectDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", false
		}
		projectDir = cwd
	}
	candidate := filepath.Join(projectDir, DirectoryName)
	info, err := os.Stat(candidate)
	if err != nil || !info.IsDir() {
		return "", false
	}
	return candidate, true
}

func abs(path string) (string, error) {
	resolved, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve absolute path %q: %w", path, err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("read config directory %q: %w", resolved, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("config directory %q is not a directory", resolved)
	}
	return resolved, nil
}
