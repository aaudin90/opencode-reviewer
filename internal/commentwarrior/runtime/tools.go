package commentwarriorruntime

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func loadToolOverrides(toolsDir string) (map[string][]byte, error) {
	if toolsDir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(toolsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read tools directory %q: %w", toolsDir, err)
	}
	tools := make(map[string][]byte)
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".ts") {
			continue
		}
		path := filepath.Join(toolsDir, entry.Name())
		data, err := os.ReadFile(filepath.Clean(path)) // #nosec G304 G703 -- trusted config-dir path
		if err != nil {
			return nil, fmt.Errorf("read tool override %q: %w", path, err)
		}
		tools[entry.Name()] = data
	}
	if len(tools) == 0 {
		return nil, nil
	}
	return tools, nil
}
