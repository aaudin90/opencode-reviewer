FROM golang:1.25 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /app/build/opencode-reviewer ./cmd/reviewer

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    git ca-certificates && \
    rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/build/opencode-reviewer /usr/local/bin/opencode-reviewer

ENTRYPOINT ["opencode-reviewer"]
