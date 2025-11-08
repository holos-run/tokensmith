.PHONY: test
test:
	go test -v ./...

.PHONY: build
build:
	go build -o bin/tokensmith ./cmd/tokensmith

.PHONY: lint
lint:
	golangci-lint run ./...

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: clean
clean:
	rm -rf bin/

.PHONY: run
run:
	go run ./cmd/tokensmith

.PHONY: help
help:
	@echo "Available targets:"
	@echo "  make test    - Run tests"
	@echo "  make build   - Build the binary"
	@echo "  make lint    - Run linters"
	@echo "  make fmt     - Format code"
	@echo "  make clean   - Clean build artifacts"
	@echo "  make run     - Run the application"
