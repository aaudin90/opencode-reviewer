package config

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

// ReviewDir is the subdirectory inside the project for temporary review files.
const ReviewDir = ".opencode-review"

type Config struct {
	OpenCode OpenCodeConfig `toml:"opencode"`
	Git      GitConfig      `toml:"git"`
	Pipeline PipelineConfig `toml:"pipeline"`
	Output   OutputConfig   `toml:"output"`
}

type PipelineConfig struct {
	AgentsMDContent string `toml:"agents_md_content"`
}

type OpenCodeConfig struct {
	Endpoint     string `toml:"endpoint"`
	Port         int    `toml:"port"`
	Model        string `toml:"model"`
	Binary       string `toml:"binary"`
	StageTimeout int    `toml:"stage_timeout"`
}

type GitConfig struct {
	ProjectDir string `toml:"project_dir"`
	Remote     string `toml:"remote"`
	Branch     string `toml:"branch"`
	BaseBranch string `toml:"base_branch"`
}

type OutputConfig struct {
	FilePath         string `toml:"file_path"`
	FormatProjectDir string `toml:"format_project_dir"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path from CLI flag, not user input
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	if cfg.OpenCode.Binary == "" {
		cfg.OpenCode.Binary = "opencode"
	}

	if cfg.OpenCode.StageTimeout == 0 {
		cfg.OpenCode.StageTimeout = 600
	}

	if cfg.Git.Remote == "" {
		cfg.Git.Remote = "origin"
	}

	if cfg.Git.BaseBranch == "" {
		cfg.Git.BaseBranch = "main"
	}

	if cfg.Output.FilePath == "" {
		cfg.Output.FilePath = "review-report.md"
	}

	return &cfg, nil
}
