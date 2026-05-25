package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/aaudin90/opencode-reviewer/internal/shared/config"
	"github.com/aaudin90/opencode-reviewer/internal/shared/sse"
	"github.com/aaudin90/opencode-reviewer/internal/shared/workspace"
)

const (
	healthPollInterval = 500 * time.Millisecond
	healthTimeout      = 30 * time.Second
	healthBodyLimit    = 1024
	stopGracePeriod    = 10 * time.Second
	abortTimeout       = 5 * time.Second
	maxRetries         = 3
	// toolCallWaitTimeout is a grace period after sendMessage returns.
	// By that point the agent has finished; if it called the expected tool,
	// the SSE event is already buffered and sseCh fires immediately.
	// This timeout only covers potential network delay in SSE delivery.
	// If it expires, the attempt is treated as a miss and triggers a retry.
	toolCallWaitTimeout = 3 * time.Second
	precheckTimeout     = 3 * time.Minute
	precheckPrompt      = "Health check. Reply with exactly: OK\nDo not use tools. Do not add explanations or markdown."
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
	name       string
	logPath    string
	logFile    *os.File
}

// New creates a Runner for the given config and working directory.
// For subprocess mode (cfg.Endpoint == ""), baseURL is set in StartServe after
// a free port is allocated; cfg.Port is used as a hint (or default 4096 as fallback).
func New(cfg config.OpenCodeConfig, workDir string, ws *workspace.Workspace, name string) *Runner {
	if name == "" {
		name = "opencode"
	}
	r := &Runner{
		cfg:        cfg,
		workDir:    workDir,
		ws:         ws,
		httpClient: &http.Client{},
		procDone:   make(chan error, 1),
		name:       name,
	}
	if cfg.Endpoint != "" {
		r.baseURL = strings.TrimRight(cfg.Endpoint, "/")
	}
	return r
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

	port, err := allocatePort(r.cfg.Port)
	if err != nil {
		return fmt.Errorf("allocate port: %w", err)
	}
	r.baseURL = fmt.Sprintf("http://localhost:%d", port)

	cmd := exec.CommandContext(ctx, r.cfg.Binary, r.serveArgs(port)...) // #nosec G204 -- binary from trusted config
	cmd.Dir = r.workDir
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} // isolate into its own process group
	if r.ws != nil {
		bunInstallCacheDir := filepath.Join(r.ws.CacheDir(), "bun", "install", "cache")
		cmd.Env = append(cmd.Env,
			"XDG_CONFIG_HOME="+r.ws.Dir(),
			"XDG_CACHE_HOME="+r.ws.CacheDir(),
			"XDG_DATA_HOME="+r.ws.DataDir(),
			"XDG_STATE_HOME="+r.ws.StateDir(),
			"BUN_INSTALL_CACHE_DIR="+bunInstallCacheDir,
		)
		slog.Info("workspace configured",
			"XDG_CONFIG_HOME", r.ws.Dir(),
			"XDG_CACHE_HOME", r.ws.CacheDir(),
			"XDG_DATA_HOME", r.ws.DataDir(),
			"XDG_STATE_HOME", r.ws.StateDir(),
			"BUN_INSTALL_CACHE_DIR", bunInstallCacheDir,
		)
	}
	output, err := r.processOutput(port)
	if err != nil {
		return err
	}
	cmd.Stdout = output
	cmd.Stderr = output

	slog.Info("starting opencode serve",
		"binary", r.cfg.Binary,
		"workDir", r.workDir,
		"port", port,
		"print_logs", r.cfg.PrintLogs,
		"log_level", r.cfg.LogLevel,
		"log_path", r.logPath,
	)

	if err := cmd.Start(); err != nil {
		_ = r.closeLogFile()
		return fmt.Errorf("start opencode serve: %w", err)
	}
	r.proc = cmd

	go func() {
		waitErr := cmd.Wait()
		if closeErr := r.closeLogFile(); waitErr == nil && closeErr != nil {
			waitErr = closeErr
		}
		r.procDone <- waitErr
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

func (r *Runner) serveArgs(port int) []string {
	args := []string{"serve", "--port", strconv.Itoa(port)}
	if r.cfg.PrintLogs {
		args = append(args, "--print-logs")
	}
	if r.cfg.LogLevel != "" {
		args = append(args, "--log-level", r.cfg.LogLevel)
	}
	return args
}

func (r *Runner) processOutput(port int) (io.Writer, error) {
	if !r.cfg.PrintLogs {
		return os.Stderr, nil
	}
	logDir := config.ResolveOpenCodeLogDir(r.cfg.LogDir, r.workDir)
	if err := os.MkdirAll(logDir, 0o750); err != nil {
		return nil, fmt.Errorf("create opencode log dir: %w", err)
	}
	path := filepath.Join(logDir, fmt.Sprintf("%s-%s-%d.log", r.name, time.Now().Format("20060102-150405"), port))
	file, err := os.OpenFile(filepath.Clean(path), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create opencode log file: %w", err)
	}
	r.logPath = path
	r.logFile = file
	return file, nil
}

func (r *Runner) LogPath() string {
	return r.logPath
}

func (r *Runner) closeLogFile() error {
	if r.logFile == nil {
		return nil
	}
	err := r.logFile.Close()
	r.logFile = nil
	return err
}

func schemaRetryPromptFor(toolName string, validationErr error, schemaHint string) string {
	return fmt.Sprintf(
		"You called `%s` but the arguments did not match the required schema.\n"+
			"Error: %v\n"+
			"Call the tool again with the correct schema:\n%s",
		toolName, validationErr, schemaHint,
	)
}

func retryPromptFor(toolName string) string {
	return fmt.Sprintf(
		"STOP. Your previous response is invalid because you did not call the real `%s` tool.\n\n"+
			"Your only valid next action is to call the actual `%s` tool now with the complete result.\n"+
			"Do not answer with text. Do not answer with JSON. Do not use Markdown. Do not explain. "+
			"Do not say the tool was called. Do not simulate a tool call in prose or code.\n\n"+
			"If there are no findings or this review is out of scope, still call `%s` with an empty findings array. "+
			"The review cannot continue until the real tool call is emitted.",
		toolName, toolName, toolName,
	)
}

func jsonFallbackPromptFor(toolName string) string {
	return fmt.Sprintf(
		"You failed to call the `%s` tool. "+
			"Respond with ONLY one valid raw JSON object using the exact same schema as the `%s` tool arguments. "+
			"Do not wrap it in markdown fences. Do not add prose before or after it. "+
			"Do not say the tool was called. Empty findings must be represented as an empty array: \"findings\": [].",
		toolName, toolName,
	)
}

func continuationPromptFor() string {
	return "Continue from the current session state and finish the task. If the previous attempt was interrupted by a transient internal error, recover and proceed normally."
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
	s := &runSession{r: r, req: req, out: out}
	s.execute(ctx)
}

// Precheck creates a throwaway session, sends a minimal deterministic prompt
// and verifies that the server responds with text. It is intended to be called
// right after StartServe to fail fast before real review sessions.
func (r *Runner) Precheck(ctx context.Context, agentName, model string) error {
	slog.Info("running precheck", "agent", sanitizeLogValue(agentName, 128))

	ctx, cancel := context.WithTimeout(ctx, precheckTimeout)
	defer cancel()

	sessionID, err := r.createSession(ctx)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	defer r.cleanupSession(sessionID)

	resp, err := r.sendMessage(ctx, sessionID, RunRequest{Prompt: precheckPrompt, AgentName: agentName, Model: model})
	if err != nil {
		return fmt.Errorf("send message: %w", err)
	}

	text := r.extractText(resp.Parts)
	if text != "" {
		slog.Info("precheck succeeded", "response", sanitizeLogValue(text, 256))
		return nil
	}
	if resp.Info.Cost > 0 || resp.Info.Tokens.Output > 0 {
		slog.Warn("precheck: empty text response, but tokens were used — treating as success",
			"cost", resp.Info.Cost,
			"output_tokens", resp.Info.Tokens.Output,
		)
		return nil
	}
	return fmt.Errorf("empty response")
}

// StopServe gracefully stops the opencode serve subprocess.
func (r *Runner) StopServe() {
	if r.proc == nil || r.proc.Process == nil {
		return
	}

	slog.Info("stopping opencode serve")

	// Send SIGINT to the entire process group so child processes are also signalled.
	_ = syscall.Kill(-r.proc.Process.Pid, syscall.SIGINT)

	select {
	case <-r.procDone:
		slog.Info("opencode serve stopped gracefully")
	case <-time.After(stopGracePeriod):
		slog.Warn("opencode serve did not stop in time, sending SIGKILL")
		// Kill the entire process group to ensure no orphan children remain.
		_ = syscall.Kill(-r.proc.Process.Pid, syscall.SIGKILL)
		<-r.procDone
	}
}

func (r *Runner) waitHealthy(ctx context.Context, procDone <-chan error) error {
	ticker := time.NewTicker(healthPollInterval)
	defer ticker.Stop()

	var lastErr error
	for {
		if err := r.checkHealth(ctx); err == nil {
			return nil
		} else if lastErr == nil || (!errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)) {
			lastErr = err
		}

		select {
		case <-ctx.Done():
			if lastErr != nil {
				return fmt.Errorf("health check timed out after last failure: %v: %w", lastErr, ctx.Err())
			}
			return fmt.Errorf("health check timed out: %w", ctx.Err())
		case err := <-procDone:
			if lastErr != nil {
				return fmt.Errorf("process exited before becoming healthy after last failure: %v: %w", lastErr, err)
			}
			return fmt.Errorf("process exited before becoming healthy: %w", err)
		case <-ticker.C:
		}
	}
}

func (r *Runner) checkHealth(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.baseURL+"/global/health", nil)
	if err != nil {
		return fmt.Errorf("build health request: %w", err)
	}

	resp, err := r.httpClient.Do(req) // #nosec G704 -- URL from trusted config
	if err != nil {
		return fmt.Errorf("health request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, healthBodyLimit))
	if readErr != nil {
		return fmt.Errorf("health returned %s and body read failed: %w", resp.Status, readErr)
	}
	text := strings.TrimSpace(string(body))
	if text == "" {
		return fmt.Errorf("health returned %s with empty body", resp.Status)
	}
	return fmt.Errorf("health returned %s: %s", resp.Status, sanitizeLogValue(text, healthBodyLimit))
}

func (r *Runner) createSession(ctx context.Context) (string, error) {
	slog.Info("creating session")
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
		snippet := sanitizeLogValue(string(body), 512)
		slog.Warn("createSession HTTP error", "status", resp.StatusCode, "body_snippet", snippet) // #nosec G706 -- sanitized above
		return "", fmt.Errorf("unexpected status %d: %s", resp.StatusCode, body)
	}

	var sr sessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return "", fmt.Errorf("decode session response: %w", err)
	}
	return sr.ID, nil
}

func (r *Runner) sendMessage(ctx context.Context, sessionID string, runReq RunRequest) (*messageResponse, error) {
	model := r.cfg.Model
	if runReq.Model != "" {
		model = runReq.Model
	}

	body := messageRequest{
		Parts: []messagePart{{Type: "text", Text: runReq.Prompt}},
		Agent: runReq.AgentName,
		Model: parseModel(model),
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
		if ctx.Err() == nil {
			return nil, markRetryable(err)
		}
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		snippet := sanitizeLogValue(string(body), 512)
		slog.Warn("sendMessage HTTP error", "status", resp.StatusCode, "body_snippet", snippet) // #nosec G706 -- sanitized above
		if isHTTPCodeError(resp.StatusCode) {
			return nil, newCodeError(resp.StatusCode, "POST /session/{id}/message", string(body))
		}
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, body)
	}

	var mr messageResponse
	if err := json.NewDecoder(resp.Body).Decode(&mr); err != nil {
		return nil, fmt.Errorf("decode message response: %w", err)
	}
	return &mr, nil
}

type retryableError struct {
	err error
}

func (e *retryableError) Error() string {
	return e.err.Error()
}

func (e *retryableError) Unwrap() error {
	return e.err
}

func markRetryable(err error) error {
	if err == nil {
		return nil
	}
	return &retryableError{err: err}
}

func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var retryable *retryableError
	return errors.As(err, &retryable) || sse.IsRetryable(err)
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

func (r *Runner) getSessionStats(sessionID string, childSessions *sync.Map) SessionStats {
	ids := []string{sessionID}
	childSessions.Range(func(key, _ any) bool {
		if id, ok := key.(string); ok {
			ids = append(ids, id)
		}
		return true
	})

	var total SessionStats
	for _, id := range ids {
		s, err := r.fetchSessionStats(id)
		if err != nil {
			slog.Warn("failed to get stats for session", "session", id, "error", err)
			continue
		}
		total = total.Add(s)
	}
	return total
}

func (r *Runner) fetchSessionStats(sessionID string) (SessionStats, error) {
	ctx, cancel := context.WithTimeout(context.Background(), abortTimeout)
	defer cancel()

	url := fmt.Sprintf("%s/session/%s/message", r.baseURL, sessionID)
	req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if reqErr != nil {
		return SessionStats{}, reqErr
	}

	resp, doErr := r.httpClient.Do(req) // #nosec G704 -- URL from trusted config
	if doErr != nil {
		return SessionStats{}, doErr
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return SessionStats{}, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, body)
	}

	var messages []sessionMessage
	if decErr := json.NewDecoder(resp.Body).Decode(&messages); decErr != nil {
		return SessionStats{}, fmt.Errorf("decode session messages: %w", decErr)
	}

	var stats SessionStats
	for _, m := range messages {
		if m.Info.Role != "assistant" {
			continue
		}
		stats = stats.WithMessage(m.Info, "")
	}
	return stats, nil
}

func (r *Runner) getChildSessions(ctx context.Context, sessionID string) ([]string, error) {
	url := fmt.Sprintf("%s/session/%s/children", r.baseURL, sessionID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := r.httpClient.Do(req) // #nosec G704 -- URL from trusted config
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, body)
	}

	var sessions []sessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&sessions); err != nil {
		return nil, fmt.Errorf("decode children response: %w", err)
	}

	ids := make([]string, len(sessions))
	for i, s := range sessions {
		ids[i] = s.ID
	}
	return ids, nil
}

func (r *Runner) pollChildren(ctx context.Context, sessionID string, childSessions *sync.Map) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ids, err := r.getChildSessions(ctx, sessionID)
			if err != nil {
				slog.Debug("failed to poll child sessions", "session", sessionID, "error", err)
				continue
			}
			for _, id := range ids {
				if _, loaded := childSessions.LoadOrStore(id, true); !loaded {
					slog.Info("discovered child session via polling", "child_session_id", id, "parent_session_id", sessionID)
				}
			}
		}
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

// allocatePort returns a free TCP port. If hint > 0 it is returned as-is
// (caller-configured port, no dynamic allocation). Otherwise a free ephemeral
// port is obtained from the OS; falls back to defaultPort on error.
func allocatePort(hint int) (int, error) {
	if hint > 0 {
		return hint, nil
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("find free port: %w", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port, nil
}

// sanitizeLogValue truncates s to maxLen and replaces control characters
// (newlines, carriage returns) to prevent log injection (gosec G706).
func sanitizeLogValue(s string, maxLen int) string {
	if len(s) > maxLen {
		s = s[:maxLen]
	}
	r := strings.NewReplacer("\n", " ", "\r", " ")
	return r.Replace(s)
}
