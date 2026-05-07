package git

import (
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
)

func PrepareRepository(gitClient *Client, branch string) error {
	slog.Info("fetching remote")
	if err := gitClient.Fetch(); err != nil {
		return fmt.Errorf("fetch: %w", err)
	}

	slog.Info("cleaning working tree")
	if err := gitClient.Clean(); err != nil {
		return fmt.Errorf("clean: %w", err)
	}

	slog.Info("checking out remote branch", "branch", branch)
	if err := gitClient.CheckoutRemote(branch); err != nil {
		return fmt.Errorf("checkout remote: %w", err)
	}

	slog.Info("project directory ready", "size_mb", dirSizeMB(gitClient.Dir()))
	return nil
}

// dirSizeMB returns the total size of path in megabytes.
// Returns -1 on error.
func dirSizeMB(path string) int64 {
	var total int64
	err := filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			info, infoErr := d.Info()
			if infoErr != nil {
				return nil
			}
			total += info.Size()
		}
		return nil
	})
	if err != nil {
		return -1
	}
	return total / (1024 * 1024)
}
