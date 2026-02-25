# opencode-reviewer

Automated code review pipeline powered by [OpenCode](https://opencode.ai). Runs parallel LLM-based review sessions against a Git branch diff and produces a structured Markdown report.

## How It Works

1. Fetches the target branch from the remote and builds a diff against the base branch.
2. Writes the diff into a temporary workspace (`.opencode-review/diff.md`).
3. **Phase 1** — Starts one or more OpenCode sessions in parallel (one per reviewer message). Each session reads the diff, explores the codebase, and calls the `submit_review` tool with structured findings.
4. **Phase 2** — Starts a single finalizer session that receives all Phase 1 results as JSON, deduplicates and merges findings, and calls the `submit_final_review` tool to produce the consolidated review.

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
| `--review-dump FILE` | path | Save the final LLM review as JSON to FILE after the pipeline completes. Useful for capturing output to replay with `--fast-review`. |
| `--fast-review FILE` | path | Skip the LLM pipeline and load the review from a previously saved JSON dump. Useful for iterating on VCS publishing without re-running LLM stages. |

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
  review_agent_prompt_path = "agent-prompt.md"
  # review_agent_prompt    = "..."          # inline alternative
  review_message_paths = [
    "../prompt-examples/review-bugs.md",
    "../prompt-examples/review-security.md",
    "../prompt-examples/review-architecture.md",
    "../prompt-examples/review-style.md",
  ]
  # review_messages        = ["...", "..."] # inline alternative
  # finalizer_prompt_path  = "finalizer.md"
  # finalizer_prompt       = "..."          # inline alternative
  # finalizer_message_path = "finalizer-message.md"
  # finalizer_message      = "..."          # inline alternative

# [gitlab]
#   url        = "https://gitlab.example.com"
#   token      = ""
#   project_id = 0
```

### TOML Fields Reference

#### Root

| Key | Default | Description |
|---|---|---|
| `project_dir` | — | **Required.** Absolute path to the project repository to review. |

#### `[env]`

Arbitrary key-value pairs set as environment variables. Values override TOML config fields but do not override variables already set in the system environment. Priority: **system env > [env] > TOML fields**. Useful for API keys referenced by `{env:...}` placeholders in `provider.json`.

#### `[opencode]`

| Key | Default | Description |
|---|---|---|
| `endpoint` | — | URL of a running `opencode serve` instance. If set, the reviewer connects to it instead of starting a subprocess. |
| `port` | — | Port for the managed `opencode serve` subprocess (used when `endpoint` is empty). If not set, a free port is allocated dynamically by the OS. |
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
| `review_agent_prompt_path` | — | Path to the reviewer agent prompt file (Phase 1). Relative to the TOML file, or absolute. If not set, the built-in default prompt is used. |
| `review_agent_prompt` | — | Inline reviewer agent prompt text. If set, `review_agent_prompt_path` is ignored (but env path still takes priority). |
| `review_message_paths` | — | List of reviewer message files. Each file starts a separate parallel review session (Phase 1). Relative to the TOML file, or absolute. |
| `review_messages` | — | Inline reviewer messages (alternative to `review_message_paths`). If set, `review_message_paths` is ignored (but env paths still take priority). |
| `finalizer_prompt_path` | — | Path to the finalizer agent prompt file (Phase 2 consolidation). Relative to the TOML file, or absolute. If not set, the built-in default finalizer prompt is used. |
| `finalizer_prompt` | — | Inline finalizer agent prompt text. If set, `finalizer_prompt_path` is ignored (but env path still takes priority). |
| `finalizer_message_path` | — | Path to the finalizer user message file. Relative to the TOML file, or absolute. If not set, the built-in default message is used. |
| `finalizer_message` | — | Inline finalizer user message text. If set, `finalizer_message_path` is ignored (but env path still takes priority). |

#### `[gitlab]`

| Key | Default | Description |
|---|---|---|
| `url` | — | GitLab instance URL (e.g. `https://gitlab.example.com`). |
| `token` | — | GitLab private access token for API authentication. |
| `project_id` | — | Numeric GitLab project ID. |
| `clear_comments` | `false` | Delete open, unanswered discussions on the MR before posting the new review. |

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
| `REVIEW_AGENT_PROMPT_PATH` | `pipeline.review_agent_prompt_path` | Path to reviewer agent prompt file. |
| `REVIEW_MESSAGE_PATHS` | `pipeline.review_message_paths` | Comma-separated paths to reviewer message files. Relative to CWD. |
| `REVIEW_FINALIZER_PROMPT_PATH` | `pipeline.finalizer_prompt_path` | Path to finalizer agent prompt file. |
| `REVIEW_FINALIZER_MESSAGE_PATH` | `pipeline.finalizer_message_path` | Path to finalizer user message file. |
| `REVIEW_GITLAB_URL` | `gitlab.url` | GitLab instance URL. |
| `REVIEW_GITLAB_TOKEN` | `gitlab.token` | GitLab private access token. |
| `REVIEW_GITLAB_PROJECT_ID` | `gitlab.project_id` | Numeric GitLab project ID. |
| `REVIEW_GITLAB_CLEAR_COMMENTS` | `gitlab.clear_comments` | Set to `true` or `1` to enable clearing open MR discussions before posting. |

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

#### Agent prompt (Phase 1 system prompt)

```
REVIEW_AGENT_PROMPT_PATH (reads file)
  > review_agent_prompt TOML (inline text)
  > review_agent_prompt_path TOML (reads file)
  > built-in default prompt
```

#### Reviewer messages (Phase 1 parallel sessions)

```
REVIEW_MESSAGE_PATHS (comma-separated, relative to CWD, reads files)
  > review_messages TOML (inline list)
  > review_message_paths TOML (relative to TOML file, reads files)
  > none (pipeline requires at least one message)
```

#### Finalizer prompt (Phase 2 system prompt)

```
REVIEW_FINALIZER_PROMPT_PATH (reads file)
  > finalizer_prompt TOML (inline text)
  > finalizer_prompt_path TOML (reads file)
  > built-in default finalizer prompt
```

#### Finalizer message (Phase 2 user message)

```
REVIEW_FINALIZER_MESSAGE_PATH (reads file)
  > finalizer_message TOML (inline text)
  > finalizer_message_path TOML (reads file)
  > built-in default finalizer message
```

#### All other parameters

```
ENV variable  >  TOML value  >  built-in default (if any)
```

#### `[env]` section

The `[env]` section has **middle priority**: overrides TOML config fields, but is overridden by variables already set in the system environment. Priority: **system env > [env] > TOML fields**.

## Prompt System

### Agent Prompt (Phase 1 system prompt)

The agent prompt defines the reviewer agent's behaviour, review process, and output format. It instructs the agent to call the `submit_review` tool with structured findings. All Phase 1 sessions share the same agent prompt.

- Configure via `pipeline.review_agent_prompt_path` (file) or `pipeline.review_agent_prompt` (inline) in TOML, or `REVIEW_AGENT_PROMPT_PATH` env (file path).
- If not configured, the built-in default prompt (`internal/agentconfig/default-prompt.md`) is used.

### Reviewer Messages (Phase 1 user messages, parallel sessions)

Each reviewer message starts a separate Phase 1 review session running in parallel. This enables focused, parallel reviews from different angles.

The repository includes ready-made examples in `prompt-examples/`:

| File | Focus |
|---|---|
| `review-bugs.md` | Nil dereferences, race conditions, resource leaks, error handling |
| `review-security.md` | Injection, path traversal, secrets in logs, unsafe HTTP |
| `review-architecture.md` | Package structure, interfaces, separation of concerns |
| `review-style.md` | Naming conventions, godoc, magic numbers, dead code |

Configure via `pipeline.review_message_paths` (file paths) or `pipeline.review_messages` (inline) in TOML, or `REVIEW_MESSAGE_PATHS` env (comma-separated paths).

### Finalizer Prompt (Phase 2 system prompt)

The finalizer prompt defines the finalizer agent's consolidation behaviour. It instructs the agent to deduplicate and merge Phase 1 findings and call the `submit_final_review` tool once.

- Configure via `pipeline.finalizer_prompt_path` (file) or `pipeline.finalizer_prompt` (inline) in TOML, or `REVIEW_FINALIZER_PROMPT_PATH` env (file path).
- If not configured, the built-in default prompt (`internal/finalizerconfig/default-prompt.md`) is used.
- An example finalizer prompt is available at `prompt-examples/finalizer.md`.

### Finalizer Message (Phase 2 user message)

The finalizer message is sent to the finalizer agent along with the Phase 1 results. It triggers the consolidation process.

- Configure via `pipeline.finalizer_message_path` (file) or `pipeline.finalizer_message` (inline) in TOML, or `REVIEW_FINALIZER_MESSAGE_PATH` env (file path).
- If not configured, the built-in default message (`internal/finalizerconfig/default-message.md`) is used.

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
internal/agentconfig/        Agent system prompt loading (Phase 1)
internal/finalizerconfig/    Finalizer agent prompt and message loading (Phase 2)
internal/providerconfig/     Provider JSON loading and validation
internal/promptconfig/       Reviewer message loading
internal/envconfig/          Shared ENV-or-file resolution logic
internal/agentsmd/           AGENTS.md / CLAUDE.md swap for review workspace
internal/workspace/          Temporary OpenCode workspace setup
internal/vcs/                VCS publisher interface, line normalizer, Markdown formatting
internal/vcs/gitlab/         GitLab MR comments publisher (REST API client)
configs/                     TOML configs and provider.json examples
prompt-examples/             Ready-made prompt files for parallel sessions
```
