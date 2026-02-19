package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/aaudin90/opencode-reviewer/internal/config"
	"github.com/aaudin90/opencode-reviewer/internal/workspace"
)

const (
	defaultPort        = 4096
	healthPollInterval = 500 * time.Millisecond
	healthTimeout      = 30 * time.Second
	stopGracePeriod    = 10 * time.Second
	abortTimeout       = 5 * time.Second
	agentName          = "reviewer"
)

// Runner manages an opencode serve subprocess and HTTP interaction.
type Runner struct {
	cfg        config.OpenCodeConfig
	workDir    string
	ws         *workspace.Workspace
	httpClient *http.Client
	proc       *exec.Cmd
	baseURL    string
	procDone   chan error
}

// New creates a Runner for the given config and working directory.
func New(cfg config.OpenCodeConfig, workDir string, ws *workspace.Workspace) *Runner {
	baseURL := cfg.Endpoint
	if baseURL == "" {
		port := cfg.Port
		if port == 0 {
			port = defaultPort
		}
		baseURL = fmt.Sprintf("http://localhost:%d", port)
	}

	return &Runner{
		cfg:        cfg,
		workDir:    workDir,
		ws:         ws,
		httpClient: &http.Client{},
		baseURL:    strings.TrimRight(baseURL, "/"),
		procDone:   make(chan error, 1),
	}
}

// StartServe starts the opencode serve subprocess and waits until it is healthy.
// If cfg.Endpoint is set, no subprocess is started — only a health check is performed.
func (r *Runner) StartServe(ctx context.Context) error {
	if r.cfg.Endpoint != "" {
		slog.Info("external endpoint configured, skipping subprocess start", "endpoint", r.cfg.Endpoint)
		healthCtx, cancel := context.WithTimeout(ctx, healthTimeout)
		defer cancel()
		return r.waitHealthy(healthCtx, nil)
	}

	cmd := exec.CommandContext(ctx, r.cfg.Binary, "serve") // #nosec G204 -- binary from trusted config
	cmd.Dir = r.workDir
	cmd.Env = os.Environ()
	if r.ws != nil {
		cmd.Env = append(cmd.Env,
			"XDG_CONFIG_HOME="+r.ws.Dir(),
		)
		slog.Debug("workspace env vars set",
			"XDG_CONFIG_HOME", r.ws.Dir(),
		)
	}
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr

	slog.Info("starting opencode serve", "binary", r.cfg.Binary, "workDir", r.workDir)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start opencode serve: %w", err)
	}
	r.proc = cmd

	go func() {
		r.procDone <- cmd.Wait()
	}()

	healthCtx, cancel := context.WithTimeout(ctx, healthTimeout)
	defer cancel()

	if err := r.waitHealthy(healthCtx, r.procDone); err != nil {
		// Process already exited or health timed out — kill if still running.
		if r.proc.Process != nil {
			_ = r.proc.Process.Kill()
		}
		r.proc = nil
		return err
	}

	slog.Info("opencode serve is healthy")
	return nil
}

// Run executes a single review: creates a session, sends the prompt, and returns the response text.
func (r *Runner) Run(ctx context.Context, req RunRequest) (string, error) {
	timeout := time.Duration(r.cfg.StageTimeout) * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	sessionID, err := r.createSession(ctx)
	if err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}

	slog.Info("session created", "id", sessionID)

	resp, err := r.sendMessage(ctx, sessionID, req)
	if err != nil {
		if ctx.Err() != nil {
			slog.Warn("request timed out, aborting session", "id", sessionID)
			_ = r.abortSession(sessionID)
		}
		r.cleanupSession(sessionID)
		return "", fmt.Errorf("send message: %w", err)
	}

	r.cleanupSession(sessionID)

	result := r.extractText(resp.Parts)
	t := resp.Info.Tokens
	slog.Info("review completed",
		"message", resp.Info.ID,
		"length", len(result),
		"cost", resp.Info.Cost,
		"tokens_input", t.Input,
		"tokens_output", t.Output,
		"tokens_reasoning", t.Reasoning,
		"tokens_cache_read", t.Cache.Read,
		"tokens_cache_write", t.Cache.Write,
	)
	return result, nil
}

// StopServe gracefully stops the opencode serve subprocess.
func (r *Runner) StopServe() {
	if r.proc == nil || r.proc.Process == nil {
		return
	}

	slog.Info("stopping opencode serve")

	_ = r.proc.Process.Signal(syscall.SIGINT)

	select {
	case <-r.procDone:
		slog.Info("opencode serve stopped gracefully")
	case <-time.After(stopGracePeriod):
		slog.Warn("opencode serve did not stop in time, sending SIGKILL")
		_ = r.proc.Process.Kill()
		<-r.procDone
	}
}

func (r *Runner) waitHealthy(ctx context.Context, procDone <-chan error) error {
	ticker := time.NewTicker(healthPollInterval)
	defer ticker.Stop()

	for {
		if r.checkHealth(ctx) {
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("health check timed out: %w", ctx.Err())
		case err := <-procDone:
			return fmt.Errorf("process exited before becoming healthy: %w", err)
		case <-ticker.C:
		}
	}
}

func (r *Runner) checkHealth(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.baseURL+"/global/health", nil)
	if err != nil {
		return false
	}

	resp, err := r.httpClient.Do(req) // #nosec G704 -- URL from trusted config
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	return resp.StatusCode == http.StatusOK
}

func (r *Runner) createSession(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL+"/session", nil)
	if err != nil {
		return "", err
	}

	resp, err := r.httpClient.Do(req) // #nosec G704 -- URL from trusted config
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("unexpected status %d: %s", resp.StatusCode, body)
	}

	var sr sessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return "", fmt.Errorf("decode session response: %w", err)
	}
	return sr.ID, nil
}

func (r *Runner) sendMessage(ctx context.Context, sessionID string, runReq RunRequest) (*messageResponse, error) {
	body := messageRequest{
		Parts: []messagePart{{Type: "text", Text: runReq.Prompt}},
		Agent: agentName,
		Model: parseModel(r.cfg.Model),
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal message: %w", err)
	}

	url := fmt.Sprintf("%s/session/%s/message", r.baseURL, sessionID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.httpClient.Do(req) // #nosec G704 -- URL from trusted config
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, body)
	}

	var mr messageResponse
	if err := json.NewDecoder(resp.Body).Decode(&mr); err != nil {
		return nil, fmt.Errorf("decode message response: %w", err)
	}
	return &mr, nil
}

func (r *Runner) abortSession(sessionID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), abortTimeout)
	defer cancel()

	url := fmt.Sprintf("%s/session/%s/abort", r.baseURL, sessionID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return err
	}

	resp, err := r.httpClient.Do(req) // #nosec G704 -- URL from trusted config
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

func (r *Runner) deleteSession(ctx context.Context, sessionID string) error {
	url := fmt.Sprintf("%s/session/%s", r.baseURL, sessionID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}

	resp, err := r.httpClient.Do(req) // #nosec G704 -- URL from trusted config
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

func (r *Runner) cleanupSession(sessionID string) {
	ctx, cancel := context.WithTimeout(context.Background(), abortTimeout)
	defer cancel()
	if err := r.deleteSession(ctx, sessionID); err != nil {
		slog.Warn("failed to delete session", "id", sessionID, "error", err)
	}
}

func (r *Runner) extractText(parts []messagePart) string {
	var sb strings.Builder
	for _, p := range parts {
		if p.Type == "text" {
			sb.WriteString(p.Text)
		}
	}
	return sb.String()
}
