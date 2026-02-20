# opencode-reviewer

Automated code review pipeline powered by [OpenCode](https://opencode.ai). Runs parallel LLM-based review sessions against a Git branch diff and produces a structured Markdown report.

## How It Works

1. Fetches the target branch from the remote and builds a diff against the base branch.
2. Writes the diff into a temporary workspace (`.opencode-review/diff.md`).
3. Starts one or more OpenCode sessions in parallel — one per prompt file.
4. Each session reads the diff, explores the codebase, and calls the `submit_review` tool with structured findings.
5. All session results are merged into a single Markdown report.

## Prerequisites

- Go 1.25+
- [opencode](https://opencode.ai) CLI installed and accessible via `PATH` (or configured via `opencode.binary`)
- Access to an LLM provider (configured via `provider.json`)

## Build

```bash
make build          # produces build/opencode-reviewer
```

## Quick Start

```bash
# Copy the example config and adjust it
make dev-config     # copies configs/example.toml → configs/dev.toml

# Run a review
make review BRANCH=my-feature-branch
# or directly:
./build/opencode-reviewer --config configs/dev.toml --branch my-feature-branch
```

Without a config file, all settings can be provided via environment variables:

```bash
REVIEW_PROJECT_DIR=/path/to/project \
REVIEW_BRANCH=my-feature-branch \
REVIEW_OPENCODE_MODEL=llm-proxy/kimi-k2.5 \
REVIEW_PROVIDER_CONFIG_PATH=/path/to/provider.json \
  ./build/opencode-reviewer
```

## CLI Flags

| Flag | Type | Description |
|---|---|---|
| `--config FILE` | path | Path to TOML config file. Optional — all settings can be provided via environment variables. |
| `--branch BRANCH` | string | Branch to review. Overrides `REVIEW_BRANCH` env and `git.branch` TOML. **Highest priority** for branch. |

## Configuration

The TOML config file is **optional**. All parameters can be set via environment variables.

### TOML Structure

```toml
project_dir = "/path/to/your/project"

[env]
  LLM_PROXY_API_KEY = ""

[opencode]
  endpoint             = ""
  port                 = 4096
  model                = "llm-proxy/kimi-k2.5"
  binary               = "opencode"
  stage_timeout        = 600
  max_steps            = 30
  min_version          = ""
  provider_config_path = "provider.json"

[git]
  remote      = "origin"
  branch      = ""
  base_branch = "main"

[pipeline]
  agent_config_path = "agent-prompt.md"
  prompt_paths = [
    "../prompt-examples/review-bugs.md",
    "../prompt-examples/review-security.md",
    "../prompt-examples/review-architecture.md",
    "../prompt-examples/review-style.md",
  ]

[output]
  file_path          = "review-report.md"
  format_project_dir = ""
```

### TOML Fields Reference

#### Root

| Key | Default | Description |
|---|---|---|
| `project_dir` | — | **Required.** Absolute path to the project repository to review. |

#### `[env]`

Arbitrary key-value pairs set as environment variables **before** provider/agent configs are loaded. Values are applied only if the variable is **not already set** in the environment. Useful for API keys referenced by `{env:...}` placeholders in `provider.json`.

#### `[opencode]`

| Key | Default | Description |
|---|---|---|
| `endpoint` | — | URL of a running `opencode serve` instance. If set, the reviewer connects to it instead of starting a subprocess. |
| `port` | `4096` | Port for the managed `opencode serve` subprocess (used when `endpoint` is empty). |
| `model` | — | LLM model identifier passed to OpenCode (e.g. `llm-proxy/kimi-k2.5`). |
| `binary` | `opencode` | Path to the OpenCode CLI binary. Resolved via `PATH` if not absolute. |
| `stage_timeout` | `600` | Maximum seconds allowed for a single review session. |
| `max_steps` | `30` | Maximum agent steps per review session. |
| `min_version` | — | Minimum required OpenCode version (semver). Reviewer fails if the binary is older. |
| `provider_config_path` | — | Path to provider JSON config. Relative to the TOML file, or absolute. |

#### `[git]`

| Key | Default | Description |
|---|---|---|
| `remote` | `origin` | Git remote name used for `git fetch`. |
| `branch` | — | Branch to review. Can be overridden by `REVIEW_BRANCH` env or `--branch` flag. |
| `base_branch` | `main` | Base branch to diff against. |

#### `[pipeline]`

| Key | Default | Description |
|---|---|---|
| `agent_config_path` | — | Path to the agent system prompt file. Relative to the TOML file, or absolute. If not set, the built-in default prompt is used. |
| `prompt_paths` | — | List of user prompt files. Each file starts a separate parallel review session. Relative to the TOML file, or absolute. If not set, the built-in default prompt is used for a single session. |

#### `[output]`

| Key | Default | Description |
|---|---|---|
| `file_path` | `review-report.md` | Output path for the review report. Relative to `project_dir`, or absolute. |
| `format_project_dir` | — | If set, replaces `project_dir` in file paths shown in the report (useful for CI display). |

### Environment Variables

All environment variables override their TOML counterparts when set.

| Variable | Overrides | Description |
|---|---|---|
| `REVIEW_PROJECT_DIR` | `project_dir` | Path to the project repository. |
| `REVIEW_BRANCH` | `git.branch` | Branch to review. Overridden by `--branch` CLI flag. |
| `REVIEW_GIT_REMOTE` | `git.remote` | Git remote name. |
| `REVIEW_GIT_BASE_BRANCH` | `git.base_branch` | Base branch for diff. |
| `REVIEW_OPENCODE_ENDPOINT` | `opencode.endpoint` | OpenCode API endpoint URL. |
| `REVIEW_OPENCODE_PORT` | `opencode.port` | OpenCode subprocess port. |
| `REVIEW_OPENCODE_MODEL` | `opencode.model` | LLM model identifier. |
| `REVIEW_OPENCODE_BINARY` | `opencode.binary` | Path to the OpenCode binary. |
| `REVIEW_OPENCODE_STAGE_TIMEOUT` | `opencode.stage_timeout` | Timeout per review session in seconds. |
| `REVIEW_OPENCODE_MAX_STEPS` | `opencode.max_steps` | Max agent steps per session. |
| `REVIEW_OPENCODE_MIN_VERSION` | `opencode.min_version` | Minimum required OpenCode version. |
| `REVIEW_PROVIDER_CONFIG_PATH` | `opencode.provider_config_path` | Path to provider JSON file (takes priority over inline). |
| `REVIEW_PROVIDER_CONFIG` | `opencode.provider_config_path` | Inline provider JSON config string. |
| `REVIEW_AGENT_CONFIG_PATH` | `pipeline.agent_config_path` | Path to agent prompt file (takes priority over inline). |
| `REVIEW_AGENT_CONFIG` | `pipeline.agent_config_path` | Inline agent prompt text, or JSON with a `"prompt"` field. |
| `REVIEW_PROMPT_PATHS` | `pipeline.prompt_paths` | Comma-separated paths to prompt files. Relative to CWD. |
| `REVIEW_OUTPUT_FILE_PATH` | `output.file_path` | Report output path. |
| `REVIEW_OUTPUT_FORMAT_PROJECT_DIR` | `output.format_project_dir` | Project dir substituted into report paths for display. |

### Priority Order

Settings are resolved in this order (first match wins):

#### Branch

```
--branch flag  >  REVIEW_BRANCH env  >  git.branch TOML
```

#### Provider config (JSON)

```
REVIEW_PROVIDER_CONFIG_PATH (reads file)
  > REVIEW_PROVIDER_CONFIG (inline JSON string)
  > opencode.provider_config_path TOML (reads file)
  > none (provider config is not set)
```

#### Agent system prompt

```
REVIEW_AGENT_CONFIG_PATH (reads file)
  > REVIEW_AGENT_CONFIG (inline text)
  > pipeline.agent_config_path TOML (reads file)
  > built-in default prompt
```

#### Prompt files (parallel sessions)

```
REVIEW_PROMPT_PATHS (comma-separated, relative to CWD)
  > pipeline.prompt_paths TOML (relative to TOML file)
  > built-in default (single session, general review)
```

#### All other parameters

```
ENV variable  >  TOML value  >  built-in default (if any)
```

#### `[env]` section

The `[env]` section has the **lowest priority**. Values are applied only when the variable is not already set in the environment. It exists to supply defaults (e.g. API keys) without overriding real environment variables.

## Prompt System

### Agent Prompt (system prompt)

The agent prompt defines the agent's behaviour, review process, and output format. It instructs the agent to call the `submit_review` tool with structured findings. All sessions share the same agent prompt.

- Configure via `pipeline.agent_config_path` in TOML, `REVIEW_AGENT_CONFIG_PATH` env (file path), or `REVIEW_AGENT_CONFIG` env (inline text).
- If not configured, the built-in default prompt (`internal/agentconfig/default-prompt.md`) is used.

### Prompt Files (user prompts, parallel sessions)

Each prompt file starts a separate review session running in parallel. This enables focused, parallel reviews from different angles.

The repository includes ready-made examples in `prompt-examples/`:

| File | Focus |
|---|---|
| `review-bugs.md` | Nil dereferences, race conditions, resource leaks, error handling |
| `review-security.md` | Injection, path traversal, secrets in logs, unsafe HTTP |
| `review-architecture.md` | Package structure, interfaces, separation of concerns |
| `review-style.md` | Naming conventions, godoc, magic numbers, dead code |

Configure via `pipeline.prompt_paths` in TOML or `REVIEW_PROMPT_PATHS` env (comma-separated).

## Development

```bash
make build       # build binary → build/opencode-reviewer
make test        # run tests with race detector
make linter      # gofmt + golangci-lint + govulncheck + staticcheck + gosec
make deps        # go mod tidy
make dev-config  # create configs/dev.toml from example (no-op if exists)
make clean       # remove build/
```

All commands that make network calls must be prefixed with `NO_PROXY="*"` in the corporate network:

```bash
NO_PROXY="*" make build
NO_PROXY="*" make test
NO_PROXY="*" make deps
```

## Project Structure

```
cmd/reviewer/main.go         CLI entry point (kong + TOML config)
internal/config/             Configuration loading (TOML + ENV + defaults)
internal/git/                Git operations (fetch, diff, log)
internal/diff/               Diff parsing, filtering, context file generation
internal/runner/             OpenCode serve/run lifecycle management
internal/pipeline/           Review pipeline orchestration
internal/agentconfig/        Agent system prompt loading
internal/providerconfig/     Provider JSON loading and validation
internal/promptconfig/       Prompt file path resolution
internal/envconfig/          Shared ENV-or-file resolution logic
internal/agentsmd/           AGENTS.md / CLAUDE.md swap for review workspace
internal/workspace/          Temporary OpenCode workspace setup
configs/                     TOML configs and provider.json examples
prompt-examples/             Ready-made prompt files for parallel sessions
```
