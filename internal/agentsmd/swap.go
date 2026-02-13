package agentsmd

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

const (
	agentsFile = "AGENTS.md"
	backupExt  = ".reviewer-backup"
)

// Swap replaces AGENTS.md in projectDir with the content from src,
// backing up the original if it exists.
func Swap(projectDir, src string) error {
	target := filepath.Join(projectDir, agentsFile)
	backup := target + backupExt

	// Backup existing AGENTS.md if present.
	if _, err := os.Stat(target); err == nil {
		slog.Info("backing up AGENTS.md", "backup", backup)

		data, err := os.ReadFile(target)
		if err != nil {
			return fmt.Errorf("read existing AGENTS.md: %w", err)
		}

		if err := os.WriteFile(backup, data, 0o644); err != nil {
			return fmt.Errorf("write backup: %w", err)
		}
	}

	// Read source content.
	content, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read agents source %s: %w", src, err)
	}

	// Write new AGENTS.md.
	if err := os.WriteFile(target, content, 0o644); err != nil {
		return fmt.Errorf("write AGENTS.md: %w", err)
	}

	slog.Info("swapped AGENTS.md", "source", src)

	return nil
}

// Restore restores the original AGENTS.md from backup.
func Restore(projectDir string) error {
	target := filepath.Join(projectDir, agentsFile)
	backup := target + backupExt

	if _, err := os.Stat(backup); os.IsNotExist(err) {
		// No backup — just remove the reviewer AGENTS.md.
		_ = os.Remove(target)
		return nil
	}

	data, err := os.ReadFile(backup)
	if err != nil {
		return fmt.Errorf("read backup: %w", err)
	}

	if err := os.WriteFile(target, data, 0o644); err != nil {
		return fmt.Errorf("restore AGENTS.md: %w", err)
	}

	_ = os.Remove(backup)

	slog.Info("restored AGENTS.md from backup")

	return nil
}
