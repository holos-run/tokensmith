.PHONY: help
help: ## Show this help message
	@echo "Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

.PHONY: test
test: ## Run all tests
	go test -v -race -cover ./...

.PHONY: build
build: ## Build the tokensmith binary
	go build -o bin/tokensmith ./cmd/tokensmith

.PHONY: lint
lint: ## Run linters
	golangci-lint run ./...

.PHONY: fmt
fmt: ## Format code
	go fmt ./...

.PHONY: clean
clean: ## Clean build artifacts
	rm -rf bin/

.PHONY: run
run: build ## Build and run the application
	./bin/tokensmith --help

.PHONY: tidy
tidy: ## Tidy go modules
	go mod tidy

.PHONY: deps
deps: ## Download dependencies
	go mod download
