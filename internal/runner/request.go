package runner

// RunRequest holds parameters for a single review run.
type RunRequest struct {
	Prompt     string
	ToolName   string // expected tool name whose completion is awaited (e.g. "submit_review")
	PromptPath string // used for logging only
}
