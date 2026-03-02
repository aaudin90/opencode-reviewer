package workspace

import (
	"encoding/json"

	"github.com/aaudin90/opencode-reviewer/internal/subagentconfig"
)

// Config holds parameters for workspace generation.
type Config struct {
	ProviderJSON json.RawMessage
	Model        string
	MaxSteps     int
	SubAgents    []subagentconfig.SubAgent
}
