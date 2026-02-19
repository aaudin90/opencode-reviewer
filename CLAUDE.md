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
configs/                     → TOML configs, provider.json, agent-prompt.md
agents/                      → Agent instruction files (orchestrator, reviewers, formatter)
prompts/                     → Prompt templates
```

## Configuration

TOML config file (`configs/example.toml`) with sections:

| Section      | Key                    | Description                                           |
|--------------|------------------------|-------------------------------------------------------|
| (root)       | `project_dir`          | Absolute path to the project repository (required)    |
| `[env]`      | `KEY = "VALUE"`        | Env vars set if not already defined (e.g. API keys)   |
| `[opencode]` | `endpoint`             | URL of running opencode serve (optional)              |
| `[opencode]` | `port`                 | Port for opencode subprocess (default: 4096)          |
| `[opencode]` | `model`                | LLM model identifier                                  |
| `[opencode]` | `binary`               | Path to opencode binary (default: opencode)           |
| `[opencode]` | `stage_timeout`        | Max seconds per review stage (default: 600)           |
| `[opencode]` | `max_steps`            | Max agent steps per session (default: 30)             |
| `[opencode]` | `provider_config_path` | Path to provider JSON config (relative to TOML file)  |
| `[git]`      | `remote`               | Git remote name (default: origin)                     |
| `[git]`      | `branch`               | Branch to review                                      |
| `[git]`      | `base_branch`          | Base branch for diff (default: main)                  |
| `[pipeline]` | `agent_config_path`    | Path to agent prompt file (relative to TOML file)     |
| `[output]`   | `file_path`            | Report output path (default: review-report.md)        |
| `[output]`   | `format_project_dir`   | Project dir for path formatting in report             |

### Environment Variables

| Variable                    | Description                                                     |
|-----------------------------|-----------------------------------------------------------------|
| `REVIEW_BRANCH`             | Branch to review (overridden by `--branch` flag)                |
| `REVIEW_PROVIDER_CONFIG_PATH` | Path to provider JSON file (overrides TOML `provider_config_path`) |
| `REVIEW_PROVIDER_CONFIG`    | Inline provider JSON config                                     |
| `REVIEW_AGENT_CONFIG_PATH`  | Path to agent prompt file (overrides TOML `agent_config_path`)  |
| `REVIEW_AGENT_CONFIG`       | Inline agent prompt or JSON with `"prompt"` field               |

### Priority Order

- **Provider config**: `REVIEW_PROVIDER_CONFIG_PATH` env > `REVIEW_PROVIDER_CONFIG` env > `provider_config_path` TOML field
- **Agent prompt**: `REVIEW_AGENT_CONFIG_PATH` env > `REVIEW_AGENT_CONFIG` env > `agent_config_path` TOML field
- **Branch**: `--branch` CLI flag > `REVIEW_BRANCH` env > `git.branch` TOML field

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

## File Structure

- One type per file: each struct/interface gets its own `.go` file
- The "main" type of a package stays in the primary file (e.g., `Runner` in `runner.go`)
- Supporting types go into separate files named after the type (e.g., `RunRequest` → `request.go`)

## Agent Docs

- [agentdocs/makefile-usage.md](agentdocs/makefile-usage.md) — Makefile targets
- [agentdocs/code-style.md](agentdocs/code-style.md) — Code style guide
