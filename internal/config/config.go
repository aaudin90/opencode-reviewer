package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/BurntSushi/toml"
)

// ReviewDir is the subdirectory inside the project for temporary review files.
const ReviewDir = ".opencode-review"

type Config struct {
	ProjectDir string            `toml:"project_dir"`
	Env        map[string]string `toml:"env"`
	OpenCode   OpenCodeConfig    `toml:"opencode"`
	Git        GitConfig         `toml:"git"`
	Pipeline   PipelineConfig    `toml:"pipeline"`
}

// Load reads config from a TOML file at path and applies defaults.
// If path is empty, only defaults are applied.
// Callers must call ApplyEnvOverrides after applying [env] values.
func Load(path string) (*Config, error) {
	var cfg Config

	if path != "" {
		data, err := os.ReadFile(path) // #nosec G304 -- path from CLI flag, not user input
		if err != nil {
			return nil, fmt.Errorf("read config %s: %w", path, err)
		}

		if err := toml.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("parse config %s: %w", path, err)
		}
	}

	applyDefaults(&cfg)

	return &cfg, nil
}

func applyDefaults(cfg *Config) {
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
}

// ApplyEnvOverrides overrides config values with environment variables.
// Must be called after applyEnv so that [env] values are visible as REVIEW_* variables.
// Priority: system env > [env] section > TOML fields.
//
// Supported variables:
//
//	REVIEW_PROJECT_DIR               → project_dir
//	REVIEW_OPENCODE_ENDPOINT         → opencode.endpoint
//	REVIEW_OPENCODE_PORT             → opencode.port
//	REVIEW_OPENCODE_MODEL            → opencode.model
//	REVIEW_OPENCODE_BINARY           → opencode.binary
//	REVIEW_OPENCODE_STAGE_TIMEOUT    → opencode.stage_timeout
//	REVIEW_OPENCODE_MAX_STEPS        → opencode.max_steps
//	REVIEW_OPENCODE_MIN_VERSION      → opencode.min_version
//	REVIEW_GIT_REMOTE                → git.remote
//	REVIEW_BRANCH                    → git.branch
//	REVIEW_GIT_BASE_BRANCH           → git.base_branch
func ApplyEnvOverrides(cfg *Config) {
	if v := os.Getenv("REVIEW_PROJECT_DIR"); v != "" {
		cfg.ProjectDir = v
	}

	if v := os.Getenv("REVIEW_OPENCODE_ENDPOINT"); v != "" {
		cfg.OpenCode.Endpoint = v
	}

	if v := os.Getenv("REVIEW_OPENCODE_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.OpenCode.Port = port
		}
	}

	if v := os.Getenv("REVIEW_OPENCODE_MODEL"); v != "" {
		cfg.OpenCode.Model = v
	}

	if v := os.Getenv("REVIEW_OPENCODE_BINARY"); v != "" {
		cfg.OpenCode.Binary = v
	}

	if v := os.Getenv("REVIEW_OPENCODE_STAGE_TIMEOUT"); v != "" {
		if t, err := strconv.Atoi(v); err == nil {
			cfg.OpenCode.StageTimeout = t
		}
	}

	if v := os.Getenv("REVIEW_OPENCODE_MAX_STEPS"); v != "" {
		if s, err := strconv.Atoi(v); err == nil {
			cfg.OpenCode.MaxSteps = s
		}
	}

	if v := os.Getenv("REVIEW_OPENCODE_MIN_VERSION"); v != "" {
		cfg.OpenCode.MinVersion = v
	}

	if v := os.Getenv("REVIEW_GIT_REMOTE"); v != "" {
		cfg.Git.Remote = v
	}

	if v := os.Getenv("REVIEW_BRANCH"); v != "" {
		cfg.Git.Branch = v
	}

	if v := os.Getenv("REVIEW_GIT_BASE_BRANCH"); v != "" {
		cfg.Git.BaseBranch = v
	}
}
