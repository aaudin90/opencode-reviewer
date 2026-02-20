package runner

import "github.com/aaudin90/opencode-reviewer/internal/sse"

// RunEvent is emitted by Runner.Run on the result channel.
// Exactly one of ToolCall, Final, or Err is non-nil per event.
type RunEvent struct {
	ToolCall *sse.ToolCall // intermediate: completed tool call
	Final    *RunResult    // terminal: review completed
	Err      error         // terminal: execution error
}
