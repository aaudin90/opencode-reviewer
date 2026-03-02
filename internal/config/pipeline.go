package config

type PipelineConfig struct {
	ReviewAgentPromptPath        string   `toml:"review_agent_prompt_path"`
	ReviewAgentPrompt            string   `toml:"review_agent_prompt"`
	ReviewMessagePaths           []string `toml:"review_message_paths"`
	ReviewMessages               []string `toml:"review_messages"`
	FinalizerPromptPath          string   `toml:"finalizer_prompt_path"`
	FinalizerPrompt              string   `toml:"finalizer_prompt"`
	FinalizerMessagePath         string   `toml:"finalizer_message_path"`
	FinalizerMessage             string   `toml:"finalizer_message"`
	ReviewSubAgentPromptPaths    []string `toml:"review_sub_agent_prompt_paths"`
	ReviewSubAgentPrompts        []string `toml:"review_sub_agent_prompts"`
	FinalizerSubAgentPromptPaths []string `toml:"finalizer_sub_agent_prompt_paths"`
	FinalizerSubAgentPrompts     []string `toml:"finalizer_sub_agent_prompts"`
}
