package runner

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"time"
)

// Runner manages opencode serve/run lifecycle.
type Runner struct {
	binary       string
	endpoint     string
	port         int
	stageTimeout time.Duration
	workDir      string
}

// NewRunner creates a new opencode runner.
func NewRunner(binary, endpoint, workDir string, port int, stageTimeoutSec int) *Runner {
	return &Runner{
		binary:       binary,
		endpoint:     endpoint,
		port:         port,
		stageTimeout: time.Duration(stageTimeoutSec) * time.Second,
		workDir:      workDir,
	}
}

// StartServe starts opencode serve in background and returns cancel func.
func (r *Runner) StartServe(ctx context.Context) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, r.binary, "serve", "--port", fmt.Sprintf("%d", r.port))
	cmd.Dir = r.workDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	slog.Info("starting opencode serve", "port", r.port, "dir", r.workDir)

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start opencode serve: %w", err)
	}

	// Give it time to start up.
	time.Sleep(2 * time.Second)

	return cmd, nil
}

// Run executes opencode run with the given prompt and returns the output.
func (r *Runner) Run(ctx context.Context, prompt string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, r.stageTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, r.binary, "run", "--endpoint", r.endpoint, prompt)
	cmd.Dir = r.workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	slog.Info("running opencode", "prompt_len", len(prompt))

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("opencode run: %w (stderr: %s)", err, stderr.String())
	}

	return stdout.String(), nil
}
