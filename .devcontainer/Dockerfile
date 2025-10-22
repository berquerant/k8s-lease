ARG GO_VERSION=1.25

# ghcr.io/devcontainers/features/docker-in-docker:2 requires bookworm
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-bookworm

RUN go install golang.org/x/tools/gopls@v0.20.0
RUN go install golang.org/x/tools/cmd/goimports@v0.37.0
RUN --mount=type=cache,target=/go/pkg/mod/ \
    --mount=type=bind,source=go.sum,target=go.sum \
    --mount=type=bind,source=go.mod,target=go.mod \
    go mod download
RUN --mount=type=cache,target=/go/pkg/mod/ \
    --mount=type=bind,source=go.sum,target=go.sum \
    --mount=type=bind,source=go.mod,target=go.mod \
    go build -o /usr/bin/golangci-lint github.com/golangci/golangci-lint/v2/cmd/golangci-lint
