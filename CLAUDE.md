# opencode-reviewer

Automated code review pipeline powered by OpenCode.

## Architecture

```
cmd/reviewer/main.go         → CLI entry point (kong + TOML config)
internal/config/             → Configuration loading (TOML)
internal/git/                → Git operations (diff, fetch, log)
internal/diff/               → Diff parsing, filtering, context file generation
internal/runner/             → OpenCode serve/run management
internal/pipeline/           → Review pipeline orchestration
internal/agentsmd/           → AGENTS.md & CLAUDE.md swap (empty for review)
internal/agentconfig/        → Agent prompt loading (env / TOML path / file)
internal/providerconfig/     → Provider JSON loading (env / TOML path / file)
internal/workspace/          → Temporary workspace for opencode config
internal/vcs/                → VCS publisher interface, line normalizer, Markdown formatting
internal/vcs/gitlab/         → GitLab MR comments publisher (REST API client)
configs/                     → TOML configs, provider.json, agent-prompt.md
prompt-examples/             → Example prompt files for parallel review sessions
prompts/                     → Prompt templates
```

## Configuration

TOML config file (`configs/example.toml`) with sections:

| Section      | Key                    | Description                                           |
|--------------|------------------------|-------------------------------------------------------|
| (root)       | `project_dir`          | Absolute path to the project repository (required)    |
| `[env]`      | `KEY = "VALUE"`        | Env vars: override TOML fields, but not system env vars |
| `[opencode]` | `endpoint`             | URL of running opencode serve (optional)              |
| `[opencode]` | `port`                 | Port for opencode subprocess (default: 4096)          |
| `[opencode]` | `model`                | LLM model identifier                                  |
| `[opencode]` | `binary`               | Path to opencode binary (default: opencode)           |
| `[opencode]` | `stage_timeout`        | Max seconds per review stage (default: 600)           |
| `[opencode]` | `max_steps`            | Max agent steps per session (default: 30)             |
| `[opencode]` | `min_version`          | Minimum required opencode version (semver)            |
| `[opencode]` | `provider_config_path` | Path to provider JSON config (relative to TOML file)  |
| `[git]`      | `remote`               | Git remote name (default: origin)                     |
| `[git]`      | `branch`               | Branch to review                                      |
| `[git]`      | `base_branch`          | Base branch for diff (default: main)                  |
| `[pipeline]` | `review_agent_prompt_path` | Path to reviewer agent prompt file (relative to TOML)    |
| `[pipeline]` | `review_agent_prompt`      | Inline reviewer agent prompt (alternative to path)       |
| `[pipeline]` | `review_message_paths`     | List of reviewer message files; each triggers a parallel session |
| `[pipeline]` | `review_messages`          | Inline reviewer messages (alternative to paths)          |
| `[pipeline]` | `finalizer_prompt_path`    | Path to finalizer agent prompt file (relative to TOML)   |
| `[pipeline]` | `finalizer_prompt`         | Inline finalizer agent prompt (alternative to path)      |
| `[pipeline]` | `finalizer_message_path`   | Path to finalizer user message file (relative to TOML)   |
| `[pipeline]` | `finalizer_message`        | Inline finalizer user message (alternative to path)      |
| `[gitlab]`   | `url`                      | GitLab instance URL (e.g. https://gitlab.example.com)    |
| `[gitlab]`   | `token`                    | GitLab private access token                              |
| `[gitlab]`   | `project_id`               | Numeric GitLab project ID                                |
| `[gitlab]`   | `clear_comments`           | Delete open MR discussions before posting (default: false) |
### Environment Variables

Config file is optional — all parameters can be set via environment variables.

| Variable                          | Description                                                              |
|-----------------------------------|--------------------------------------------------------------------------|
| `OR_PROJECT_DIR`                  | Path to the project repository (overrides `project_dir`)                 |
| `OR_BRANCH`                       | Branch to review (overridden by `--branch` flag)                         |
| `OR_GIT_REMOTE`                   | Git remote name (overrides `git.remote`)                                 |
| `OR_GIT_BASE_BRANCH`              | Base branch for diff (overrides `git.base_branch`)                       |
| `OR_OPENCODE_ENDPOINT`            | opencode API endpoint URL (overrides `opencode.endpoint`)                |
| `OR_OPENCODE_PORT`                | opencode port (overrides `opencode.port`)                                |
| `OR_OPENCODE_MODEL`               | LLM model identifier (overrides `opencode.model`)                        |
| `OR_OPENCODE_BINARY`              | Path to opencode binary (overrides `opencode.binary`)                    |
| `OR_OPENCODE_STAGE_TIMEOUT`       | Timeout per stage in seconds (overrides `opencode.stage_timeout`)        |
| `OR_OPENCODE_MAX_STEPS`           | Max agent steps per session (overrides `opencode.max_steps`)             |
| `OR_OPENCODE_MIN_VERSION`         | Minimum opencode version (overrides `opencode.min_version`)              |
| `OR_PROVIDER_CONFIG_PATH`         | Path to provider JSON file (overrides `opencode.provider_config_path`)   |
| `OR_PROVIDER_CONFIG`              | Inline provider JSON config                                              |
| `OR_AGENT_PROMPT_PATH`            | Path to reviewer agent prompt file (overrides `pipeline.review_agent_prompt_path`) |
| `OR_MESSAGE_PATHS`                | Comma-separated paths to reviewer message files (overrides `pipeline.review_message_paths`) |
| `OR_FINALIZER_PROMPT_PATH`        | Path to finalizer agent prompt file (overrides `pipeline.finalizer_prompt_path`) |
| `OR_FINALIZER_MESSAGE_PATH`       | Path to finalizer user message file (overrides `pipeline.finalizer_message_path`) |
| `OR_GITLAB_URL`                   | GitLab instance URL (overrides `gitlab.url`)                             |
| `OR_GITLAB_TOKEN`                 | GitLab private access token (overrides `gitlab.token`)                   |
| `OR_GITLAB_PROJECT_ID`            | Numeric GitLab project ID (overrides `gitlab.project_id`)                |
| `OR_GITLAB_CLEAR_COMMENTS`        | Set `true` or `1` to clear open MR discussions before posting (overrides `gitlab.clear_comments`) |
| `OR_SLOG_LEVEL`                   | Log level: `debug`, `info`, `warn`, `error` (default: `info`)            |

### Priority Order

- **Branch**: `--branch` CLI flag > `OR_BRANCH` env > `git.branch` TOML field
- **Provider config**: `OR_PROVIDER_CONFIG_PATH` > `OR_PROVIDER_CONFIG` > TOML path
- **Agent prompt**: `OR_AGENT_PROMPT_PATH` env > `review_agent_prompt` TOML > `review_agent_prompt_path` TOML > built-in default
- **Messages**: `OR_MESSAGE_PATHS` env > `review_messages` TOML > `review_message_paths` TOML > (none)
- **Finalizer prompt**: `OR_FINALIZER_PROMPT_PATH` env > `finalizer_prompt` TOML > `finalizer_prompt_path` TOML > built-in default
- **Finalizer message**: `OR_FINALIZER_MESSAGE_PATH` env > `finalizer_message` TOML > `finalizer_message_path` TOML > built-in default
- **All other ENV vars**: override TOML value if set

## Commit Format

All commits use the format: `78288: description`

The numeric ID corresponds to the task identifier.

## Commands

**All** terminal commands and ad-hoc test scripts must be run with `NO_PROXY="*"` to bypass corporate proxy. This includes make, go, curl, opencode, git fetch/push, and any custom bash/python scripts that make network calls:

```bash
NO_PROXY="*" make build
NO_PROXY="*" make run
NO_PROXY="*" make test
NO_PROXY="*" go mod tidy
NO_PROXY="*" opencode serve --port 4097
```

- `make build` — build binary
- `make run` — run with dev config
- `make test` — run tests with race detector
- `make linter` — run gofmt + golangci-lint + govulncheck + staticcheck + gosec
- `make deps` — go mod tidy
- `make dev-config` — create configs/dev.toml from example

## After Code Changes

After modifying code, always run:
1. `NO_PROXY="*" make test` — tests must pass
2. `NO_PROXY="*" make linter` — fix all reported issues (govulncheck, staticcheck, gosec included)

## CLI Flags and Configuration Sync

Whenever CLI flags or environment variables are added, removed, or changed — including in `cmd/reviewer/main.go`, `internal/config/config.go`, `internal/agentconfig/`, `internal/providerconfig/`, or `internal/promptconfig/` — the following files **must** be updated in the same commit:

1. **`README.md`** — CLI Flags table, Environment Variables table, TOML Fields Reference, and Priority Order sections.
2. **`cmd/reviewer/main.go`** — the `kong.Description(...)` help text (the multi-line string passed to `kong.New`), which is shown by `--help`.

## File Structure

- One type per file: each struct/interface gets its own `.go` file
- The "main" type of a package stays in the primary file (e.g., `Runner` in `runner.go`)
- Supporting types go into separate files named after the type (e.g., `RunRequest` → `request.go`)

## Agent Docs

- [agentdocs/makefile-usage.md](agentdocs/makefile-usage.md) — Makefile targets
- [agentdocs/code-style.md](agentdocs/code-style.md) — Code style guide
