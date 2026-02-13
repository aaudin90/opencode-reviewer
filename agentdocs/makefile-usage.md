# Makefile Usage

## Targets

| Target | Description |
|--------|-------------|
| `dev-config` | Creates `configs/dev.toml` from `configs/example.toml` if it doesn't exist |
| `run` | Runs the reviewer with dev config (depends on `dev-config`) |
| `build` | Builds binary to `./build/opencode-reviewer` |
| `review` | Builds and runs review on a branch: `make review BRANCH=feature-branch` |
| `test` | Runs all tests with race detector and coverage |
| `linter` | Runs gofmt check and golangci-lint |
| `tools` | Installs development tools (golangci-lint) |
| `deps` | Runs `go mod tidy` |
| `clean` | Removes build artifacts |

## Examples

```bash
# First run — create dev config and run
make run

# Run review on a specific branch
make review BRANCH=my-feature

# Full CI check locally
make linter && make test
```
