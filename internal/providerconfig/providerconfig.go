package providerconfig

import (
	"encoding/json"
	"fmt"

	"github.com/aaudin90/opencode-reviewer/internal/envconfig"
)

type providerEntry struct {
	Models map[string]modelEntry `json:"models"`
}

type modelEntry struct {
	Cost  *costEntry  `json:"cost,omitempty"`
	Limit *limitEntry `json:"limit,omitempty"`
}

type costEntry struct {
	Input     float64 `json:"input"`
	Output    float64 `json:"output"`
	CacheRead float64 `json:"cacheRead"`
}

type limitEntry struct {
	Context int `json:"context"`
	Output  int `json:"output"`
}

type configShape struct {
	Provider map[string]providerEntry `json:"provider"`
	Model    string                   `json:"model"`
}

// Load reads provider config JSON from OR_PROVIDER_CONFIG_PATH (priority),
// OR_PROVIDER_CONFIG env var, or the given configPath fallback.
// Returns validated json.RawMessage.
// Returns nil without error if no source is available.
func Load(configPath string) (json.RawMessage, error) {
	raw, err := envconfig.ReadEnvOrFile("OR_PROVIDER_CONFIG_PATH", "OR_PROVIDER_CONFIG", configPath)
	if err != nil {
		return nil, fmt.Errorf("load provider config: %w", err)
	}
	if raw == "" {
		return nil, nil
	}

	data := json.RawMessage(raw)
	if err := Validate(data); err != nil {
		return nil, fmt.Errorf("validate provider config: %w", err)
	}
	return data, nil
}

// Validate checks that provider config JSON has required fields.
func Validate(data json.RawMessage) error {
	var cfg configShape
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse JSON: %w", err)
	}

	if len(cfg.Provider) == 0 {
		return fmt.Errorf("provider is required and must not be empty")
	}

	for name, p := range cfg.Provider {
		if len(p.Models) == 0 {
			return fmt.Errorf("provider %q: models is required and must not be empty", name)
		}

		for modelName, m := range p.Models {
			if m.Cost != nil {
				if m.Cost.Input < 0 {
					return fmt.Errorf("provider %q model %q: cost.input must be non-negative", name, modelName)
				}
				if m.Cost.Output < 0 {
					return fmt.Errorf("provider %q model %q: cost.output must be non-negative", name, modelName)
				}
			}
			if m.Limit != nil {
				if m.Limit.Context <= 0 {
					return fmt.Errorf("provider %q model %q: limit.context must be positive", name, modelName)
				}
			}
		}
	}

	if cfg.Model == "" {
		return fmt.Errorf("model is required")
	}

	return nil
}
