package agentsmd

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const agentsFile = "AGENTS.md"

// heavyDirs contains directory names that should be skipped during tree walk
// to avoid traversing large dependency/build/cache directories.
var heavyDirs = map[string]struct{}{
	".git":             {},
	"node_modules":     {},
	"vendor":           {},
	".gradle":          {},
	"build":            {},
	".build":           {},
	"dist":             {},
	".next":            {},
	".nuxt":            {},
	"venv":             {},
	".venv":            {},
	"__pycache__":      {},
	".tox":             {},
	".eggs":            {},
	"target":           {},
	".cache":           {},
	".idea":            {},
	".vscode":          {},
	"Pods":             {},
	".dart_tool":       {},
	".pub-cache":       {},
	".terraform":       {},
	".bundle":          {},
	"bower_components": {},
	".mypy_cache":      {},
	".pytest_cache":    {},
	".cargo":           {},
	".npm":             {},
	".yarn":            {},
	".pnpm-store":      {},
	".opencode-review": {},
}

// Swapper removes existing AGENTS.md files from the project tree
// and writes review-mode content as root AGENTS.md.
// Restoration is handled externally via gitClient.Clean().
type Swapper struct {
	projectDir string
}

// NewSwapper creates a Swapper for the given project directory.
func NewSwapper(projectDir string) *Swapper {
	return &Swapper{
		projectDir: projectDir,
	}
}

// Swap removes every AGENTS.md found in the project tree and writes
// the provided content as AGENTS.md at the project root.
// It returns the paths of all removed AGENTS.md files.
func (s *Swapper) Swap(content string) ([]string, error) {
	if strings.TrimSpace(content) == "" {
		return nil, errors.New("agents md content must not be empty")
	}

	removed, err := s.removeAll()
	if err != nil {
		return nil, fmt.Errorf("remove agents files: %w", err)
	}

	agentsPath := filepath.Join(s.projectDir, agentsFile)

	if err := os.WriteFile(agentsPath, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil { // #nosec G306 -- non-sensitive project file
		return nil, fmt.Errorf("write %s: %w", agentsFile, err)
	}

	return removed, nil
}

// removeAll walks the project tree and removes every AGENTS.md.
// It returns the paths of removed files.
func (s *Swapper) removeAll() ([]string, error) {
	var removed []string

	err := filepath.WalkDir(s.projectDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			if _, skip := heavyDirs[d.Name()]; skip {
				return filepath.SkipDir
			}
			return nil
		}

		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}

		if d.Name() == agentsFile {
			if removeErr := os.Remove(path); removeErr != nil {
				return fmt.Errorf("remove %s: %w", path, removeErr)
			}
			removed = append(removed, path)
		}

		return nil
	})

	return removed, err
}
