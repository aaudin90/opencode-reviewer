# opencode-reviewer

Automated code review pipeline powered by [OpenCode](https://opencode.ai). It runs parallel LLM review sessions against a Git branch diff and publishes a consolidated Markdown review.

## How It Works

1. Fetches and checks out the target branch.
2. Resolves file-based configuration from the checked-out branch.
3. Builds a diff against the base branch and writes it to `.opencode-review/diff.md`.
4. Starts one or more reviewer sessions in parallel, one per reviewer message.
5. Starts a finalizer session and publishes the final review through the configured VCS publisher.

## Prerequisites

- Go 1.26.2
- [opencode](https://opencode.ai) CLI available in `PATH`, or configured via `opencode.binary`
- Access to an LLM provider, configured through `provider.json`

## Build

```bash
make build
```

The binaries are written to `build/opencode-reviewer` and `build/opencode-reviewer-comment-warrior`.

The Docker base image contains both CLIs in `PATH`: `opencode-reviewer` and `opencode-reviewer-comment-warrior`.

## Quick Start

The primary configuration path is a project-local `.opencodereview` directory.

```bash
./build/opencode-reviewer --config-dir /path/to/repo/.opencodereview --branch my-feature-branch
```

If `--config-dir` and `OR_CONFIG_DIR` are not set, the reviewer auto-discovers `.opencodereview` in `project_dir` after checking out the target branch, or in the current working directory when `project_dir` is not known yet.

For local development with the legacy TOML example:

```bash
make dev-config
make review BRANCH=my-feature-branch
```

Full CLI help is available with:

```bash
./build/opencode-reviewer --help
```

Comment-warrior handles existing GitLab discussions after the branch is checked out:

```bash
./build/opencode-reviewer-comment-warrior --config ./configs/dev.toml --mr-iid 123 --dry-run
```

Example CI command:

```bash
opencode-reviewer-comment-warrior --branch "${CI_MERGE_REQUEST_SOURCE_BRANCH_NAME}" --mr-iid "${CI_MERGE_REQUEST_IID}"
```

## `.opencodereview`

`.opencodereview` is the main and highest-priority way to configure review prompts and provider files. The older env-based provider/prompt configuration is deprecated and remains only as a fallback when config-dir mode is inactive.

File-based configuration is read after the target branch is prepared, so `reviewer/messages/*.md` can be supplied by the branch under review.

Config directory priority:

```text
--config-dir flag
  > OR_CONFIG_DIR env
  > <project_dir or cwd>/.opencodereview auto-discovery after checkout
```

Recommended structure:

```text
.opencodereview/
  provider.json
  reviewer/
    agent.md
    tools/
      submit_review.ts
      custom_tool.ts
    messages/
      01-bugs.md
      02-security.md
    sub-agents/
      verifier.md
  finalizer/
    agent.md
    message.md
    tools/
      submit_final_review.ts
      custom_tool.ts
    sub-agents/
      verifier.md
  comment-warrior/
    agent.md
    finding-message.md
    mention-message.md
    tools/
      submit_comment_warrior_decision.ts
    sub-agents/
      verifier.md
```

File mapping:

| Path | Used as |
|---|---|
| `provider.json` | OpenCode provider configuration |
| `reviewer/agent.md` | Phase 1 reviewer system prompt |
| `reviewer/messages/*.md` | Phase 1 reviewer messages; files are sorted lexicographically |
| `reviewer/sub-agents/*.md` | Reviewer sub-agent prompts; files are sorted lexicographically |
| `reviewer/tools/*.ts` | Optional reviewer tool overrides or custom tools |
| `finalizer/agent.md` | Phase 2 finalizer system prompt |
| `finalizer/message.md` | Phase 2 finalizer user message |
| `finalizer/sub-agents/*.md` | Finalizer sub-agent prompts; files are sorted lexicographically |
| `finalizer/tools/*.ts` | Optional finalizer tool overrides or custom tools |
| `comment-warrior/agent.md` | Comment-warrior system prompt |
| `comment-warrior/finding-message.md` | User message for follow-up on opencode-reviewer finding comments |
| `comment-warrior/mention-message.md` | User message for human `#ai` mention requests |
| `comment-warrior/sub-agents/*.md` | Comment-warrior sub-agent prompts; files are sorted lexicographically |
| `comment-warrior/tools/*.ts` | Optional comment-warrior tool overrides or custom tools |

`submit_review.ts`, `submit_final_review.ts`, and `submit_comment_warrior_decision.ts` are built in. Add files under phase `tools` directories only when you need to override a built-in tool or add a custom OpenCode tool.

When files exist in `.opencodereview`, they override the corresponding TOML prompt/provider paths. Scalar settings such as branch, GitLab URL, and timeouts can be set only with an explicit `--config` TOML file or env vars. Keep the model in `provider.json` unless you intentionally need an env or explicit TOML override.
Model fallback chain priority is `OR_OPENCODE_FALLBACK_MODELS` > `opencode.fallback_models`; the primary model comes from `OR_OPENCODE_MODEL`, `opencode.model`, or `provider.json`.

## Explicit TOML

Use `--config some.toml` when you want scalar settings in TOML:

```toml
project_dir = "/path/to/project"

[env]
  LLM_PROXY_API_KEY = ""

[opencode]
  # Prefer setting model in provider.json.
  # model = "llm-proxy/kimi-k2.5"
  # fallback_models = ["openai/gpt-5", "anthropic/claude-sonnet-4-5"]
  max_steps = 50
  print_logs = false
  # log_level = "DEBUG"
  log_dir = "opencode-review-logs"

[git]
  remote = "origin"
  branch = ""
  base_branch = "main"

[gitlab]
  url = "https://gitlab.example.com"
  token = ""
  project_id = 0
```

See [configs/example.toml](configs/example.toml) for the full TOML reference.

## CLI And Env

Common flags:

| Flag | Description |
|---|---|
| `--config-dir DIR` | Use a config directory explicitly; files are read after checkout |
| `--config FILE` | Use a TOML config explicitly |
| `--branch BRANCH` | Branch to review; overrides `OR_BRANCH` and `git.branch` |
| `--review-dump FILE` | Save final review JSON for debugging |
| `--fast-review FILE` | Replay a saved review JSON without running LLM stages |

Comment-warrior flags:

| Flag | Description |
|---|---|
| `--config FILE` | Use a TOML config explicitly |
| `--config-dir DIR` | Use a config directory explicitly; files are read after checkout |
| `--branch BRANCH` | Branch to checkout; priority is flag, `OR_BRANCH`, CI source branch, TOML |
| `--mr-iid IID` | GitLab merge request IID |
| `--dry-run` | Build tasks and run the agent without GitLab mutations |
| `--max-discussions N` | Limit processed discussions; `0` means no limit |
| `--discussion-id ID` | Process one GitLab discussion |
| `--disable-config-dir-auto-discovery` | Disable `.opencodereview` auto-discovery |

Common env vars:

| Variable | Description |
|---|---|
| `OR_CONFIG_DIR` | Config directory used after checkout when `--config-dir` is not set |
| `OR_DISABLE_CONFIG_DIR_AUTO_DISCOVERY` | Disable `.opencodereview` auto-discovery with `true` or `1` |
| `OR_PROJECT_DIR` | Project repository path |
| `OR_BRANCH` | Branch to review |
| `OR_OPENCODE_MODEL` | LLM model identifier in `provider/model` format |
| `OR_OPENCODE_FALLBACK_MODELS` | Comma-separated fallback models in `provider/model` format |
| `OR_OPENCODE_PRINT_LOGS` | Pass `--print-logs` to `opencode serve` with `true` or `1` |
| `OR_OPENCODE_LOG_LEVEL` | Pass `--log-level` to `opencode serve`: `DEBUG`, `INFO`, `WARN`, or `ERROR` |
| `OR_OPENCODE_LOG_DIR` | OpenCode log directory; relative paths are resolved under `project_dir` |
| `OR_OPENCODE_KEEP_XDG_DIRS` | Keep temporary OpenCode XDG dirs after the run for debugging with `true` or `1` |
| `OR_GITLAB_URL` | GitLab instance URL |
| `OR_GITLAB_TOKEN` | GitLab private access token |
| `OR_GITLAB_PROJECT_ID` | Numeric GitLab project ID |
| `OR_COMMENT_WARRIOR_MR_IID` | MR IID for comment-warrior |
| `OR_COMMENT_WARRIOR_AGENT_PROMPT_PATH` | Deprecated fallback comment-warrior agent prompt path when config-dir is inactive |
| `OR_COMMENT_WARRIOR_FINDING_MESSAGE_PATH` | Deprecated fallback comment-warrior finding message path when config-dir is inactive |
| `OR_COMMENT_WARRIOR_MENTION_MESSAGE_PATH` | Deprecated fallback comment-warrior mention message path when config-dir is inactive |
| `OR_SLOG_LEVEL` | Log level: `debug`, `info`, `warn`, `error` |

Deprecated fallback env vars such as `OR_PROVIDER_CONFIG_PATH`, `OR_PROVIDER_CONFIG`, `OR_AGENT_PROMPT_PATH`, `OR_MESSAGE_PATHS`, and finalizer/sub-agent prompt env vars are ignored when config-dir mode is active.

For CI log artifacts:

```yaml
variables:
  OR_OPENCODE_PRINT_LOGS: "true"
  OR_OPENCODE_LOG_LEVEL: "INFO"
  OR_OPENCODE_LOG_DIR: "opencode-review-logs"

artifacts:
  when: always
  paths:
    - opencode-review-logs/
  expire_in: 7 days
```

## Development

```bash
make build       # build binary
make test        # run tests with race detector
make linter      # gofmt + golangci-lint + govulncheck + staticcheck + gosec
make deps        # go mod tidy
make dev-config  # create configs/dev.toml from example
make clean       # remove build/
```

Use `NO_PROXY="*"` only for commands that make network calls, such as dependency downloads or remote Git operations.
