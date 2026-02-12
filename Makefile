.PHONY: build test test-unit e2e lint install clean tools hooks

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X github.com/boneskull/gh-stack/cmd.version=$(VERSION)"

build:
	go build $(LDFLAGS) -o gh-stack .

test: ## Run all tests (unit + E2E)
	go test ./... -v

test-unit: ## Run unit tests only (faster)
	go test ./cmd/... ./internal/... -v

e2e: ## Run E2E tests only
	go test ./e2e/... -v

lint:
	golangci-lint run ./...

# Run all checks (what CI does)
ci: lint test build

install:
	go install $(LDFLAGS) .

clean:
	rm -f gh-stack

# Install as gh extension
gh-install: build
	gh extension remove gh-stack || true
	gh extension install .

# Install development tools
tools:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.8.0
	go install github.com/evilmartians/lefthook/v2@latest

# Install git hooks via lefthook
hooks:
	lefthook install
