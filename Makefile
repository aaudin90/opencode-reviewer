BINARY := opencode-reviewer
COMMENT_WARRIOR_BINARY := opencode-reviewer-comment-warrior
BUILD_DIR := ./build
MAIN := ./cmd/reviewer
COMMENT_WARRIOR_MAIN := ./cmd/comment-warrior
export GOTOOLCHAIN := go1.26.3

.PHONY: dev-config run build review test linter tools deps clean

dev-config:
	@if [ ! -f ./configs/dev.toml ]; then cp ./configs/example.toml ./configs/dev.toml; fi

run: dev-config
	@go run $(MAIN) --config ./configs/dev.toml

build:
	@go build -o $(BUILD_DIR)/$(BINARY) $(MAIN)
	@go build -o $(BUILD_DIR)/$(COMMENT_WARRIOR_BINARY) $(COMMENT_WARRIOR_MAIN)

review: build
	$(BUILD_DIR)/$(BINARY) --config ./configs/dev.toml --branch $(BRANCH)

test:
	@go test -race -cover ./...

linter:
	@test -z "$$(gofmt -l .)" || (echo "gofmt issues" && exit 1)
	@-golangci-lint run -c ./golangci.yml ./...
	@-go tool govulncheck ./...
	@-go tool staticcheck ./...
	@-go tool gosec ./...

tools:
	@GOPROXY=https://proxy.golang.org,direct go mod tidy
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

deps:
	@GOPROXY=https://proxy.golang.org,direct go mod tidy

clean:
	@rm -rf $(BUILD_DIR)
