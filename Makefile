.PHONY: build test lint install clean

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X github.com/boneskull/gh-stack/cmd.version=$(VERSION)"

build:
	go build $(LDFLAGS) -o gh-stack .

test:
	go test ./... -v

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
	mkdir -p ~/.local/share/gh/extensions/gh-stack
	cp gh-stack ~/.local/share/gh/extensions/gh-stack/

# Install development tools
tools:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.8.0
