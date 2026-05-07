package git

import (
	"fmt"
	"log/slog"
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

	slog.Info("project directory ready")
	return nil
}
