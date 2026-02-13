# Code Style Guide

## Go Conventions

- Follow standard Go conventions: [Effective Go](https://go.dev/doc/effective_go)
- Use `gofmt` for formatting (enforced by linter)
- Use `goimports` with local prefix `github.com/aaudin90`

## Error Handling

- Always wrap errors with context: `fmt.Errorf("operation: %w", err)`
- Do not ignore errors silently — use `_ =` only for cleanup operations
- Use `log/slog` for structured logging (no third-party loggers)

## Naming

- Package names: short, lowercase, no underscores
- Interfaces: verb-er pattern (`Reader`, `Runner`)
- Avoid stuttering: `config.Config` is OK, `config.ConfigManager` is not

## Project Structure

- `cmd/` — CLI entry points only, minimal logic
- `internal/` — all business logic, not importable externally
- No `pkg/` directory — everything is internal

## Dependencies

- Minimize external dependencies
- No `gitlab.corp.mail.ru` imports
- Prefer stdlib where possible (`log/slog` over zerolog, `net/http` over frameworks)

## Testing

- Table-driven tests preferred
- Use `testify` only if already in go.mod, otherwise stdlib `testing`
- Test files next to the code they test
