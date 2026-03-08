GO_CMD ?= go
BINARY_DIR ?= bin

.PHONY: build test lint vet clean help

## build: Build tiangong and tg binaries
build:
	$(GO_CMD) build -o $(BINARY_DIR)/tiangong ./cmd/tiangong
	$(GO_CMD) build -o $(BINARY_DIR)/tg ./cmd/tg

## test: Run all tests
test:
	$(GO_CMD) test ./...

## lint: Run golangci-lint
lint:
	golangci-lint run ./...

## vet: Run go vet
vet:
	$(GO_CMD) vet ./...

## clean: Remove build artifacts
clean:
	rm -rf $(BINARY_DIR)

## help: Show this help message
help:
	@echo "Available targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'

build: ## Default target
