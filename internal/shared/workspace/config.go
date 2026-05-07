package workspace

import (
	"encoding/json"

	"github.com/aaudin90/opencode-reviewer/internal/shared/subagentconfig"
)

// Config holds parameters for workspace generation.
type Config struct {
	ProviderJSON  json.RawMessage
	Model         string
	MaxSteps      int
	SubAgents     []subagentconfig.SubAgent
	ToolOverrides map[string][]byte
}
