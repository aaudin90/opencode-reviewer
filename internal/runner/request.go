package runner

// RunRequest holds parameters for a single review run.
type RunRequest struct {
	Prompt     string // initial user message text sent to the agent
	ToolName   string // expected tool call name awaited via SSE (e.g. "submit_review")
	PromptPath string // logical label for logging (e.g. "message-0", "finalizer")
	AgentName  string // opencode agent name (e.g. "reviewer", "finalizer")
}
