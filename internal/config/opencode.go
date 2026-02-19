package config

type OpenCodeConfig struct {
	Endpoint           string `toml:"endpoint"`
	Port               int    `toml:"port"`
	Model              string `toml:"model"`
	Binary             string `toml:"binary"`
	StageTimeout       int    `toml:"stage_timeout"`
	MaxSteps           int    `toml:"max_steps"`
	ProviderConfigPath string `toml:"provider_config_path"`
	MinVersion         string `toml:"min_version"`
}
