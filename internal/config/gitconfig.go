package config

type GitConfig struct {
	Remote     string `toml:"remote"`
	Branch     string `toml:"branch"`
	BaseBranch string `toml:"base_branch"`
}
