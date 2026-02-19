package agentsmd

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const agentsFile = "AGENTS.md"
const claudeFile = "CLAUDE.md"

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

// targetFiles lists the filenames to find and overwrite with empty content.
var targetFiles = map[string]struct{}{
	agentsFile: {},
	claudeFile: {},
}

// Swapper finds and empties AGENTS.md and CLAUDE.md files in the project tree.
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

// Swap finds every AGENTS.md and CLAUDE.md in the project tree and overwrites
// them with an empty string. It also ensures empty AGENTS.md and CLAUDE.md
// exist at the project root. Returns the paths of all overwritten files.
func (s *Swapper) Swap() ([]string, error) {
	overwritten, err := s.overwriteAll()
	if err != nil {
		return nil, fmt.Errorf("overwrite agent files: %w", err)
	}

	// Ensure root files exist (empty).
	for _, name := range []string{agentsFile, claudeFile} {
		rootPath := filepath.Join(s.projectDir, name)
		if err := os.WriteFile(rootPath, []byte(""), 0o644); err != nil { // #nosec G306 -- non-sensitive project file
			return nil, fmt.Errorf("write %s: %w", name, err)
		}
	}

	return overwritten, nil
}

// overwriteAll walks the project tree and overwrites every AGENTS.md and CLAUDE.md
// with an empty string. Returns the paths of overwritten files.
func (s *Swapper) overwriteAll() ([]string, error) {
	var overwritten []string

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

		if _, match := targetFiles[d.Name()]; match {
			if writeErr := os.WriteFile(path, []byte(""), 0o644); writeErr != nil { // #nosec G306 -- non-sensitive project file
				return fmt.Errorf("overwrite %s: %w", path, writeErr)
			}
			overwritten = append(overwritten, path)
		}

		return nil
	})

	return overwritten, err
}
