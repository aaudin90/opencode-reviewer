package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/aaudin90/opencode-reviewer/internal/sse"
)

var errToolNotCalled = errors.New("agent did not call the expected tool")

type sseResult struct {
	data json.RawMessage
	err  error
}

type runSession struct {
	r             *Runner
	req           RunRequest
	ctx           context.Context
	cancel        context.CancelFunc
	sessionID     string
	sseClient     *sse.Client
	childSessions *sync.Map
	out           chan<- RunEvent
	pollCancel    context.CancelFunc
	pollDone      chan struct{}
	runStart      time.Time
}

func (s *runSession) execute(ctx context.Context) {
	s.ctx, s.cancel = context.WithTimeout(ctx, time.Duration(s.r.cfg.StageTimeout)*time.Second)
	defer s.cancel()

	s.runStart = time.Now()
	deadline, _ := s.ctx.Deadline()
	slog.Info("run started", "stage_timeout_s", s.r.cfg.StageTimeout, "deadline", deadline.Format(time.RFC3339), "prompt", s.req.PromptPath)

	createStart := time.Now()
	sessionID, err := s.r.createSession(s.ctx)
	if err != nil {
		s.out <- RunEvent{Err: fmt.Errorf("create session: %w", err)}
		return
	}
	s.sessionID = sessionID
	slog.Info("session created", "id", s.sessionID, "prompt", s.req.PromptPath, "elapsed", time.Since(createStart).String())

	s.sseClient = sse.New(s.r.httpClient, s.r.baseURL)
	s.childSessions = &sync.Map{}
	pollCtx, pollCancel := context.WithCancel(s.ctx)
	s.pollCancel = pollCancel
	s.pollDone = make(chan struct{})
	go func() {
		defer close(s.pollDone)
		s.r.pollChildren(pollCtx, s.sessionID, s.childSessions)
	}()

	var lastInvalidData json.RawMessage
	var lastSchemaErr error

	for attempt := range maxRetries {
		var prompt string
		switch {
		case attempt == 0:
			prompt = s.req.Prompt
		case lastSchemaErr != nil:
			prompt = schemaRetryPromptFor(s.req.ToolName, lastSchemaErr, s.req.SchemaHint)
			slog.Warn("retrying: schema validation failed", "tool", s.req.ToolName, "attempt", attempt)
		default:
			prompt = retryPromptFor(s.req.ToolName)
			slog.Warn("retrying: tool not called", "tool", s.req.ToolName, "attempt", attempt)
		}

		slog.Info("sending message to session", "session", s.sessionID, "attempt", attempt)
		data, err := s.sendAndAwaitTool(prompt)
		if err != nil {
			if errors.Is(err, errToolNotCalled) {
				lastInvalidData = nil
				lastSchemaErr = nil
				continue
			}
			s.emitErr(err)
			return
		}

		if s.req.ValidateFunc != nil {
			if vErr := s.req.ValidateFunc(data); vErr != nil {
				slog.Warn("schema validation failed",
					"tool", s.req.ToolName, "attempt", attempt, "error", vErr)
				lastInvalidData = data
				lastSchemaErr = vErr
				continue
			}
		}

		s.emitToolResult(data, attempt)
		return
	}

	s.jsonFallback(lastInvalidData)
}

func (s *runSession) sendAndAwaitTool(prompt string) (json.RawMessage, error) {
	attemptCtx, attemptCancel := context.WithCancel(s.ctx)

	events := make(chan sse.ToolCall, 256)
	sseCh := make(chan sseResult, 1)
	go func() {
		data, sseErr := s.sseClient.WaitForToolResult(attemptCtx, s.sessionID, s.req.ToolName, s.childSessions, events)
		sseCh <- sseResult{data, sseErr}
	}()

	tcDone := make(chan struct{})
	go func() {
		defer close(tcDone)
		for tc := range events {
			slog.Info("tool call", "tool", tc.Tool, "session", tc.SessionID, "args", string(tc.Input))
			s.out <- RunEvent{ToolCall: &tc}
		}
	}()

	msgStart := time.Now()
	resp, err := s.r.sendMessage(s.ctx, s.sessionID, RunRequest{Prompt: prompt, AgentName: s.req.AgentName})
	if err != nil {
		attemptCancel()
		<-tcDone
		if s.ctx.Err() != nil {
			return nil, fmt.Errorf("stage timeout: %w", s.ctx.Err())
		}
		return nil, fmt.Errorf("send message: %w", err)
	}
	t := resp.Info.Tokens
	slog.Info("message completed", "message", resp.Info.ID,
		"tokens_input", t.Input, "tokens_output", t.Output, "elapsed", time.Since(msgStart).String())

	slog.Info("waiting for tool call result", "session", s.sessionID, "tool", s.req.ToolName)
	select {
	case res := <-sseCh:
		attemptCancel()
		<-tcDone
		if res.err == nil {
			return res.data, nil
		}
		return nil, errToolNotCalled
	case <-time.After(toolCallWaitTimeout):
		attemptCancel()
		<-tcDone
		return nil, errToolNotCalled
	case <-s.ctx.Done():
		attemptCancel()
		<-tcDone
		return nil, fmt.Errorf("stage timeout: %w", s.ctx.Err())
	}
}

func (s *runSession) emitToolResult(toolData json.RawMessage, attempt int) {
	stats := s.cleanup()
	slog.Info("review completed via tool call",
		"session", s.sessionID, "prompt", s.req.PromptPath, "tool", s.req.ToolName, "attempt", attempt,
		"elapsed", time.Since(s.runStart).String(), "cost", stats.Cost,
		"tokens_input", stats.Tokens.Input, "tokens_output", stats.Tokens.Output,
		"tokens_reasoning", stats.Tokens.Reasoning,
		"tokens_cache_read", stats.Tokens.Cache.Read, "tokens_cache_write", stats.Tokens.Cache.Write,
	)
	s.out <- RunEvent{Final: &RunResult{ToolArgs: toolData, Stats: stats}}
}

func (s *runSession) emitErr(err error) {
	s.pollCancel()
	<-s.pollDone
	if s.ctx.Err() != nil {
		slog.Warn("request timed out, aborting session", "id", s.sessionID)
		_ = s.r.abortSession(s.sessionID)
	}
	s.r.cleanupSession(s.sessionID)
	s.out <- RunEvent{Err: err}
}

func (s *runSession) jsonFallback(lastToolData json.RawMessage) {
	slog.Warn("all retries exhausted, requesting JSON fallback",
		"tool", s.req.ToolName, "session", s.sessionID)

	msgStart := time.Now()
	resp, err := s.r.sendMessage(s.ctx, s.sessionID, RunRequest{
		Prompt:    jsonFallbackPromptFor(s.req.ToolName),
		AgentName: s.req.AgentName,
	})
	if err != nil {
		s.pollCancel()
		<-s.pollDone
		s.r.cleanupSession(s.sessionID)
		s.out <- RunEvent{Err: fmt.Errorf("json fallback message: %w", err)}
		return
	}

	stats := s.cleanup()
	fallbackText := s.r.extractText(resp.Parts)
	slog.Info("review completed via JSON fallback",
		"session", s.sessionID, "prompt", s.req.PromptPath, "tool", s.req.ToolName,
		"elapsed", time.Since(s.runStart).String(), "msg_elapsed", time.Since(msgStart).String(),
		"length", len(fallbackText), "cost", stats.Cost,
		"tokens_input", stats.Tokens.Input, "tokens_output", stats.Tokens.Output,
		"tokens_reasoning", stats.Tokens.Reasoning,
		"tokens_cache_read", stats.Tokens.Cache.Read, "tokens_cache_write", stats.Tokens.Cache.Write,
	)

	result := &RunResult{FallbackText: fallbackText, Stats: stats}
	if lastToolData != nil {
		result.ToolArgs = lastToolData
	}
	s.out <- RunEvent{Final: result}
}

func (s *runSession) cleanup() SessionStats {
	s.pollCancel()
	<-s.pollDone
	stats := s.r.getSessionStats(s.sessionID, s.childSessions)
	s.r.cleanupSession(s.sessionID)
	return stats
}
