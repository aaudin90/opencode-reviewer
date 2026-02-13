# opencode-reviewer

Automated code review pipeline powered by OpenCode.

## Architecture

```
cmd/reviewer/main.go    → CLI entry point (kong + TOML config)
internal/config/        → Configuration loading (TOML)
internal/git/           → Git operations (diff, fetch, log)
internal/runner/        → OpenCode serve/run management
internal/agentsmd/      → AGENTS.md swap/restore
agents/                 → Agent instruction files
prompts/                → Prompt templates
```

## Commit Format

All commits use the format: `78288: description`

The numeric ID corresponds to the task identifier.

## Commands

- `make build` — build binary
- `make run` — run with dev config
- `make test` — run tests with race detector
- `make linter` — run gofmt + golangci-lint
- `make deps` — go mod tidy
- `make dev-config` — create configs/dev.toml from example

## Agent Docs

- [agentdocs/makefile-usage.md](agentdocs/makefile-usage.md) — Makefile targets
- [agentdocs/code-style.md](agentdocs/code-style.md) — Code style guide
