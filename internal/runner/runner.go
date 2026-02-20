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
	"github.com/aaudin90/opencode-reviewer/internal/sse"
	"github.com/aaudin90/opencode-reviewer/internal/workspace"
)

type sseResult struct {
	data json.RawMessage
	err  error
}

const (
	defaultPort         = 4096
	healthPollInterval  = 500 * time.Millisecond
	healthTimeout       = 30 * time.Second
	stopGracePeriod     = 10 * time.Second
	abortTimeout        = 5 * time.Second
	agentName           = "reviewer"
	maxToolCallRetries  = 3
	toolCallWaitTimeout = 3 * time.Second
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

func retryPromptFor(toolName string) string {
	return fmt.Sprintf(
		"You did not call the `%s` tool. "+
			"You MUST call it now with all your findings. Do not output text — use the tool.",
		toolName,
	)
}

// Run executes a single review and streams events on the returned channel.
// The channel is closed after the final event (RunEvent.Final or RunEvent.Err).
func (r *Runner) Run(ctx context.Context, req RunRequest) <-chan RunEvent {
	ch := make(chan RunEvent, 64)
	go func() {
		defer close(ch)
		r.run(ctx, req, ch)
	}()
	return ch
}

// run is the internal implementation of Run. It attempts to obtain a
// submit_review tool call via SSE up to maxToolCallRetries times, then falls
// back to text extraction.
func (r *Runner) run(ctx context.Context, req RunRequest, out chan<- RunEvent) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(r.cfg.StageTimeout)*time.Second)
	defer cancel()

	sessionID, err := r.createSession(ctx)
	if err != nil {
		out <- RunEvent{Err: fmt.Errorf("create session: %w", err)}
		return
	}
	slog.Info("session created", "id", sessionID)

	sseClient := sse.New(r.httpClient, r.baseURL)
	var lastResp *messageResponse

	for attempt := range maxToolCallRetries {
		attemptCtx, attemptCancel := context.WithCancel(ctx)

		prompt := req.Prompt
		if attempt > 0 {
			prompt = retryPromptFor(req.ToolName)
			slog.Warn("retrying tool call", "tool", req.ToolName, "attempt", attempt)
		}

		// SSE goroutine starts BEFORE sendMessage to avoid missing the event.
		events := make(chan sse.ToolCall, 256)
		sseCh := make(chan sseResult, 1)
		go func() {
			data, sseErr := sseClient.WaitForToolResult(attemptCtx, sessionID, req.ToolName, events)
			sseCh <- sseResult{data, sseErr}
		}()

		resp, err := r.sendMessage(ctx, sessionID, RunRequest{Prompt: prompt})
		if err != nil {
			attemptCancel()
			if ctx.Err() != nil {
				slog.Warn("request timed out, aborting session", "id", sessionID)
				_ = r.abortSession(sessionID)
			}
			r.cleanupSession(sessionID)
			out <- RunEvent{Err: fmt.Errorf("send message: %w", err)}
			return
		}
		lastResp = resp

		t := resp.Info.Tokens
		slog.Info("message completed", "message", resp.Info.ID, "cost", resp.Info.Cost,
			"tokens_input", t.Input, "tokens_output", t.Output)

		// After sendMessage completes, the tool call event should already be buffered.
		// Wait toolCallWaitTimeout — if the agent called the tool the goroutine returns immediately.
		select {
		case res := <-sseCh:
			attemptCancel()
			// Drain all tool call events (channel is already closed by WaitForToolResult).
			for tc := range events {
				out <- RunEvent{ToolCall: &tc}
			}
			if res.err == nil {
				r.cleanupSession(sessionID)
				slog.Info("review completed via tool call", "session", sessionID, "tool", req.ToolName, "attempt", attempt)
				out <- RunEvent{Final: &RunResult{ToolArgs: res.data}}
				return
			}
			slog.Warn("agent did not call tool", "tool", req.ToolName, "attempt", attempt, "err", res.err)
		case <-time.After(toolCallWaitTimeout):
			attemptCancel()
			slog.Warn("tool call wait timeout, agent likely did not call tool", "tool", req.ToolName, "attempt", attempt)
		case <-ctx.Done():
			attemptCancel()
			_ = r.abortSession(sessionID)
			r.cleanupSession(sessionID)
			out <- RunEvent{Err: fmt.Errorf("stage timeout: %w", ctx.Err())}
			return
		}
	}

	// Fallback: agent did not call the tool after all retries — use text response.
	slog.Warn("all tool call retries exhausted, falling back to text output",
		"tool", req.ToolName, "session", sessionID)
	r.cleanupSession(sessionID)

	fallbackText := ""
	if lastResp != nil {
		fallbackText = r.extractText(lastResp.Parts)
		t := lastResp.Info.Tokens
		slog.Info("review completed via text fallback",
			"message", lastResp.Info.ID,
			"length", len(fallbackText),
			"cost", lastResp.Info.Cost,
			"tokens_input", t.Input,
			"tokens_output", t.Output,
			"tokens_reasoning", t.Reasoning,
			"tokens_cache_read", t.Cache.Read,
			"tokens_cache_write", t.Cache.Write,
		)
	}

	out <- RunEvent{Final: &RunResult{FallbackText: fallbackText}}
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
