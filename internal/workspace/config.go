package workspace

import "encoding/json"

// Config holds parameters for workspace generation.
type Config struct {
	ProviderJSON json.RawMessage
	Model        string
	MaxSteps     int
	AgentPrompt  string
}
