package config

import (
	"fmt"
	"strings"
)

type OpenCodeConfig struct {
	Endpoint           string   `toml:"endpoint"`
	Port               int      `toml:"port"`
	Model              string   `toml:"model"`
	FallbackModels     []string `toml:"fallback_models"`
	Binary             string   `toml:"binary"`
	StageTimeout       int      `toml:"stage_timeout"`
	MaxSteps           int      `toml:"max_steps"`
	ProviderConfigPath string   `toml:"provider_config_path"`
	MinVersion         string   `toml:"min_version"`
	PrintLogs          bool     `toml:"print_logs"`
	LogLevel           string   `toml:"log_level"`
	LogDir             string   `toml:"log_dir"`
}

func (c OpenCodeConfig) ModelChain(providerModel string) ([]string, error) {
	fallbacks := normalizeModelList(c.FallbackModels)
	if len(fallbacks) == 0 {
		return nil, nil
	}

	primary := strings.TrimSpace(c.Model)
	if primary == "" {
		primary = strings.TrimSpace(providerModel)
	}
	if primary == "" {
		return nil, fmt.Errorf("opencode.fallback_models requires a primary model from opencode.model or provider config")
	}
	if !isFullModelID(primary) {
		return nil, fmt.Errorf("primary model %q must be in provider/model format when fallback_models is set", primary)
	}

	chain := make([]string, 0, len(fallbacks)+1)
	seen := map[string]struct{}{primary: {}}
	chain = append(chain, primary)

	for _, model := range fallbacks {
		if !isFullModelID(model) {
			return nil, fmt.Errorf("fallback model %q must be in provider/model format", model)
		}
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		chain = append(chain, model)
	}

	return chain, nil
}

func normalizeModelList(models []string) []string {
	result := make([]string, 0, len(models))
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		result = append(result, model)
	}
	return result
}

func isFullModelID(model string) bool {
	parts := strings.SplitN(model, "/", 2)
	return len(parts) == 2 && parts[0] != "" && parts[1] != ""
}
