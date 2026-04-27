# opencode-reviewer

Automated code review pipeline powered by OpenCode.

## Architecture

```
cmd/reviewer/main.go         → CLI entry point (kong + TOML config)
internal/config/             → Configuration loading (TOML + ENV + config-dir defaults)
internal/git/                → Git operations (diff, fetch, log)
internal/diff/               → Diff parsing, filtering, context file generation
internal/runner/             → OpenCode serve/run management
internal/pipeline/           → Review pipeline orchestration
internal/agentsmd/           → AGENTS.md & CLAUDE.md swap (empty for review)
internal/agentconfig/        → Agent prompt loading (config-dir / TOML / deprecated env fallback)
internal/providerconfig/     → Provider JSON loading (config-dir / TOML / deprecated env fallback)
internal/subagentconfig/     → Sub-agent prompt loading
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
| `[opencode]` | `max_steps`            | Max agent steps per session (default: 50)             |
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
| `[pipeline]` | `review_sub_agent_prompt_paths`    | Paths to reviewer sub-agent prompt files (relative to TOML)     |
| `[pipeline]` | `review_sub_agent_prompts`         | Inline reviewer sub-agent prompts (alternative to paths)        |
| `[pipeline]` | `finalizer_sub_agent_prompt_paths` | Paths to finalizer sub-agent prompt files (relative to TOML)    |
| `[pipeline]` | `finalizer_sub_agent_prompts`      | Inline finalizer sub-agent prompts (alternative to paths)       |
| `[gitlab]`   | `url`                      | GitLab instance URL (e.g. https://gitlab.example.com)    |
| `[gitlab]`   | `token`                    | GitLab private access token                              |
| `[gitlab]`   | `project_id`               | Numeric GitLab project ID                                |
| `[gitlab]`   | `clear_comments`           | Delete open MR discussions before posting (default: false) |
### Environment Variables

Use config-dir or TOML for file-based configuration. Environment variables override scalar TOML settings; provider and prompt env vars are deprecated fallbacks when config-dir is inactive.

| Variable                          | Description                                                              |
|-----------------------------------|--------------------------------------------------------------------------|
| `OR_CONFIG_DIR`                   | Path to config directory, used when `--config-dir` is not set            |
| `OR_DISABLE_CONFIG_DIR_AUTO_DISCOVERY` | Disable `.opencodereview` auto-discovery (`true` or `1`)             |
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
| `OR_PROVIDER_CONFIG_PATH`         | Deprecated fallback: path to provider JSON file                          |
| `OR_PROVIDER_CONFIG`              | Deprecated fallback: inline provider JSON config                         |
| `OR_AGENT_PROMPT_PATH`            | Deprecated fallback: path to reviewer agent prompt file                  |
| `OR_MESSAGE_PATHS`                | Deprecated fallback: comma-separated paths to reviewer message files     |
| `OR_FINALIZER_PROMPT_PATH`        | Deprecated fallback: path to finalizer agent prompt file                 |
| `OR_FINALIZER_MESSAGE_PATH`       | Deprecated fallback: path to finalizer user message file                 |
| `OR_REVIEW_SUB_AGENT_PROMPT_PATHS`    | Deprecated fallback: comma-separated paths to reviewer sub-agent prompt files |
| `OR_FINALIZER_SUB_AGENT_PROMPT_PATHS` | Deprecated fallback: comma-separated paths to finalizer sub-agent prompt files |
| `OR_GITLAB_URL`                   | GitLab instance URL (overrides `gitlab.url`)                             |
| `OR_GITLAB_TOKEN`                 | GitLab private access token (overrides `gitlab.token`)                   |
| `OR_GITLAB_PROJECT_ID`            | Numeric GitLab project ID (overrides `gitlab.project_id`)                |
| `OR_GITLAB_CLEAR_COMMENTS`        | Set `true` or `1` to clear open MR discussions before posting (overrides `gitlab.clear_comments`) |
| `OR_SLOG_LEVEL`                   | Log level: `debug`, `info`, `warn`, `error` (default: `info`)            |

### Priority Order

- **Config directory**: `--config-dir` flag > `OR_CONFIG_DIR` env > `<project_dir or cwd>/.opencodereview`
- **Branch**: `--branch` CLI flag > `OR_BRANCH` env > `git.branch` TOML field
- **Provider config**: config-dir `provider.json` > TOML path > deprecated `OR_PROVIDER_CONFIG_PATH` / `OR_PROVIDER_CONFIG` fallback
- **Agent prompt**: config-dir `reviewer/agent.md` > `review_agent_prompt` TOML > `review_agent_prompt_path` TOML > deprecated `OR_AGENT_PROMPT_PATH` fallback > built-in default
- **Messages**: config-dir `reviewer/messages/*.md` > `review_messages` TOML > `review_message_paths` TOML > deprecated `OR_MESSAGE_PATHS` fallback > (none)
- **Finalizer prompt**: config-dir `finalizer/agent.md` > `finalizer_prompt` TOML > `finalizer_prompt_path` TOML > deprecated `OR_FINALIZER_PROMPT_PATH` fallback > built-in default
- **Finalizer message**: config-dir `finalizer/message.md` > `finalizer_message` TOML > `finalizer_message_path` TOML > deprecated `OR_FINALIZER_MESSAGE_PATH` fallback > built-in default
- **Reviewer sub-agents**: config-dir `reviewer/sub-agents/*.md` > `review_sub_agent_prompts` TOML > `review_sub_agent_prompt_paths` TOML > deprecated `OR_REVIEW_SUB_AGENT_PROMPT_PATHS` fallback > (none)
- **Finalizer sub-agents**: config-dir `finalizer/sub-agents/*.md` > `finalizer_sub_agent_prompts` TOML > `finalizer_sub_agent_prompt_paths` TOML > deprecated `OR_FINALIZER_SUB_AGENT_PROMPT_PATHS` fallback > (none)
- **All other ENV vars**: override TOML value if set

## Commit Format

All commits use the format: `78288: description`

The numeric ID corresponds to the task identifier.

## Commands

Use `NO_PROXY="*"` only for commands that make network calls (downloading dependencies, HTTP requests, git fetch/push). Local build/test/lint commands do **not** need it.
Local full review runs (`opencode-reviewer ...` and `go run ./cmd/reviewer ...`) must use `NO_PROXY="*"`.

```bash
NO_PROXY="*" go mod tidy
NO_PROXY="*" opencode serve --port 4097
make build
make run
make test
make linter
```

- `make build` — build binary
- `make run` — run with dev config
- `make test` — run tests with race detector
- `make linter` — run gofmt + golangci-lint + govulncheck + staticcheck + gosec
- `make deps` — go mod tidy
- `make dev-config` — create configs/dev.toml from example

## After Code Changes

After modifying code, always run:
1. `make test` — tests must pass
2. `make linter` — fix all reported issues (govulncheck, staticcheck, gosec included)

## CLI Flags and Configuration Sync

Whenever CLI flags or environment variables are added, removed, or changed — including in `cmd/reviewer/main.go`, `internal/config/config.go`, `internal/agentconfig/`, `internal/providerconfig/`, or `internal/promptconfig/` — the following files **must** be updated in the same commit:

1. **`README.md`** — CLI Flags table, Environment Variables table, TOML Fields Reference, and Priority Order sections.
2. **`cmd/reviewer/main.go`** — the `kong.Description(...)` help text (the multi-line string passed to `kong.New`), which is shown by `--help`.

## File Structure

- One type per file: each struct/interface gets its own `.go` file with all its methods
- The "main" type of a package stays in the primary file (e.g., `Runner` in `runner.go`)
- Supporting types go into separate files named after the type (e.g., `RunRequest` → `request.go`)

## Agent Docs

- [agentdocs/makefile-usage.md](agentdocs/makefile-usage.md) — Makefile targets
- [agentdocs/code-style.md](agentdocs/code-style.md) — Code style guide
