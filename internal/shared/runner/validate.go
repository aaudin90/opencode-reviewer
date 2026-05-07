package runner

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"golang.org/x/mod/semver"

	"github.com/aaudin90/opencode-reviewer/internal/shared/config"
)

var versionRe = regexp.MustCompile(`(\d+\.\d+\.\d+)`)

// ValidateBinary checks the presence and version of the opencode binary.
// Skipped when cfg.Endpoint is set (external instance does not require a local binary).
func ValidateBinary(cfg config.OpenCodeConfig) error {
	if cfg.Endpoint != "" {
		return nil
	}

	if _, err := exec.LookPath(cfg.Binary); err != nil {
		return fmt.Errorf("opencode binary %q not found in PATH: %w", cfg.Binary, err)
	}

	version, err := readVersion(cfg.Binary)
	if err != nil {
		// cannot read version — log a warning and proceed
		slog.Warn("could not read opencode version", "binary", cfg.Binary, "error", err)
		return nil
	}

	slog.Info("opencode version", "version", version)

	if cfg.MinVersion == "" {
		return nil
	}

	min := normalizeVersion(cfg.MinVersion)
	actual := normalizeVersion(version)

	if !semver.IsValid(min) {
		return fmt.Errorf("invalid min_version %q in config", cfg.MinVersion)
	}
	if !semver.IsValid(actual) {
		slog.Warn("opencode version has unexpected format, skipping check", "version", version)
		return nil
	}

	if semver.Compare(actual, min) < 0 {
		return fmt.Errorf("opencode version %s is below required minimum %s", version, cfg.MinVersion)
	}

	return nil
}

func readVersion(binary string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, binary, "--version").CombinedOutput() // #nosec G204
	if err != nil {
		return "", fmt.Errorf("run %s --version: %w", binary, err)
	}

	m := versionRe.FindString(strings.TrimSpace(string(out)))
	if m == "" {
		return "", fmt.Errorf("no version found in output: %q", out)
	}
	return m, nil
}

func normalizeVersion(v string) string {
	if strings.HasPrefix(v, "v") {
		return v
	}
	return "v" + v
}
