package main

import (
	"os"
	"path/filepath"
	"strings"
)

func loadToolOverrides(toolsDir string) map[string][]byte {
	if toolsDir == "" {
		return nil
	}

	entries, err := os.ReadDir(toolsDir)
	if err != nil {
		return nil
	}

	tools := make(map[string][]byte)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.EqualFold(filepath.Ext(entry.Name()), ".ts") {
			continue
		}
		path := filepath.Join(toolsDir, entry.Name())
		data, readErr := os.ReadFile(filepath.Clean(path)) // #nosec G304 G703 -- path from trusted config dir
		if readErr != nil {
			continue
		}
		tools[entry.Name()] = data
	}
	if len(tools) == 0 {
		return nil
	}
	return tools
}
