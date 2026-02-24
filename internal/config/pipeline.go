package config

type PipelineConfig struct {
	AgentConfigPath     string   `toml:"agent_config_path"`
	PromptPaths         []string `toml:"prompt_paths"`
	FinalizerConfigPath string   `toml:"finalizer_config_path"`
}
