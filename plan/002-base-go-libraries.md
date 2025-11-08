# Plan 002: Base Go Libraries and Project Foundation

**Status**: Approved

## Overview
Set up the foundational Go libraries and project structure with Cobra for CLI, slog for logging, Connect RPC for gRPC communication, and establish a basic table-driven test structure. The end goal is to have a buildable project with good help output and passing unit tests.

## Goals
1. Install and configure core Go dependencies (Cobra, slog, Connect RPC)
2. Set up Cobra CLI with proper command structure and help output
3. Configure structured JSON logging using standard library `slog`
4. Establish table-driven test patterns and basic unit test infrastructure
5. Ensure `make build` succeeds and produces a working binary
6. Ensure `make test` runs and passes basic unit tests
7. Provide excellent CLI help output using Cobra

## Technical Decisions

### Core Libraries
- **Cobra** (github.com/spf13/cobra) - CLI framework
  - Industry standard for Go CLI applications
  - Excellent help generation and flag handling
  - Subcommand support for future extensibility

- **slog** (log/slog - standard library) - Structured logging
  - Built-in to Go 1.21+, no external dependency
  - Native JSON output support
  - Structured fields for better log analysis
  - Configurable log levels

- **Connect RPC** (connectrpc.com/connect) - gRPC framework
  - Modern, simpler alternative to traditional gRPC
  - Better ergonomics and code generation
  - HTTP/1.1 and HTTP/2 support
  - Backward compatible with gRPC protocol

### Project Structure
```
tokensmith/
├── cmd/
│   └── tokensmith/
│       ├── main.go              # Entry point
│       └── commands/
│           ├── root.go          # Root command with global flags
│           └── version.go       # Version command (example)
├── internal/
│   └── logging/
│       ├── logging.go           # Centralized slog configuration
│       └── logging_test.go      # Table-driven tests for logging
├── tools.go                     # Build-time tool dependencies
├── go.mod
├── go.sum
└── Makefile
```

## Implementation Steps

### 1. Add Core Dependencies

#### 1.1 Update go.mod with dependencies
```bash
go get github.com/spf13/cobra@latest
go get connectrpc.com/connect@latest
go get google.golang.org/protobuf@latest
```

#### 1.2 Create tools.go for build-time dependencies
```go
//go:build tools

// Package tools tracks tool dependencies for the project.
package tools

import (
	_ "github.com/bufbuild/buf/cmd/buf"
	_ "connectrpc.com/connect/cmd/protoc-gen-connect-go"
	_ "google.golang.org/protobuf/cmd/protoc-gen-go"
)
```

### 2. Set Up Logging Infrastructure

#### 2.1 Create internal/logging/logging.go
```go
package logging

import (
	"log/slog"
	"os"
)

// Config holds logging configuration.
type Config struct {
	Level  slog.Level
	Format string // "json" or "text"
}

// NewLogger creates a new configured logger.
func NewLogger(cfg Config) *slog.Logger {
	var handler slog.Handler
	opts := &slog.HandlerOptions{
		Level: cfg.Level,
	}

	switch cfg.Format {
	case "text":
		handler = slog.NewTextHandler(os.Stdout, opts)
	default: // "json" is default
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	return slog.New(handler)
}

// SetDefault sets the default global logger.
func SetDefault(cfg Config) {
	slog.SetDefault(NewLogger(cfg))
}

// ParseLevel converts a string to slog.Level.
func ParseLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
```

#### 2.2 Create internal/logging/logging_test.go
```go
package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  slog.Level
	}{
		{
			name:  "debug level",
			input: "debug",
			want:  slog.LevelDebug,
		},
		{
			name:  "info level",
			input: "info",
			want:  slog.LevelInfo,
		},
		{
			name:  "warn level",
			input: "warn",
			want:  slog.LevelWarn,
		},
		{
			name:  "error level",
			input: "error",
			want:  slog.LevelError,
		},
		{
			name:  "unknown defaults to info",
			input: "invalid",
			want:  slog.LevelInfo,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseLevel(tt.input)
			if got != tt.want {
				t.Errorf("ParseLevel(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestNewLogger(t *testing.T) {
	tests := []struct {
		name   string
		config Config
	}{
		{
			name: "json logger with info level",
			config: Config{
				Level:  slog.LevelInfo,
				Format: "json",
			},
		},
		{
			name: "text logger with debug level",
			config: Config{
				Level:  slog.LevelDebug,
				Format: "text",
			},
		},
		{
			name: "default format (json)",
			config: Config{
				Level:  slog.LevelWarn,
				Format: "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := NewLogger(tt.config)
			if logger == nil {
				t.Fatal("NewLogger() returned nil")
			}

			// Test that the logger can log without panicking
			logger.Info("test message", slog.String("key", "value"))
		})
	}
}
```

### 3. Set Up Cobra CLI Structure

#### 3.1 Create cmd/tokensmith/commands/root.go
```go
package commands

import (
	"os"

	"github.com/holos-run/tokensmith/internal/logging"
	"github.com/spf13/cobra"
)

var (
	// Global flags
	logLevel  string
	logFormat string
)

// NewRootCmd creates the root command for tokensmith.
func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tokensmith",
		Short: "Tokensmith - Envoy External Authorizer for OIDC Token Exchange",
		Long: `Tokensmith is an Envoy external authorizer (ext_authz) for Istio 1.27+
that exchanges OIDC ID tokens for Kubernetes service accounts in one cluster
for ID tokens for valid Kubernetes service accounts in another cluster.

This enables secure cross-cluster authentication in multi-cluster Kubernetes
environments using native service account identities.`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// Configure logging before any command runs
			cfg := logging.Config{
				Level:  logging.ParseLevel(logLevel),
				Format: logFormat,
			}
			logging.SetDefault(cfg)
		},
		SilenceUsage:  true, // Don't show usage on errors
		SilenceErrors: true, // We'll handle errors ourselves
	}

	// Global flags available to all commands
	cmd.PersistentFlags().StringVar(&logLevel, "log-level", "info",
		"Log level (debug, info, warn, error)")
	cmd.PersistentFlags().StringVar(&logFormat, "log-format", "json",
		"Log format (json, text)")

	// Add subcommands
	cmd.AddCommand(NewVersionCmd())

	return cmd
}

// Execute runs the root command.
func Execute() {
	if err := NewRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
```

#### 3.2 Create cmd/tokensmith/commands/version.go
```go
package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	// These will be set by build flags
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

// NewVersionCmd creates the version command.
func NewVersionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("tokensmith version %s\n", version)
			fmt.Printf("commit: %s\n", commit)
			fmt.Printf("built: %s\n", date)
		},
	}

	return cmd
}
```

#### 3.3 Create cmd/tokensmith/commands/root_test.go
```go
package commands

import (
	"testing"
)

func TestNewRootCmd(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "help flag",
			args: []string{"--help"},
		},
		{
			name: "version command",
			args: []string{"version"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := NewRootCmd()
			cmd.SetArgs(tt.args)

			// Should not panic
			if err := cmd.Execute(); err != nil {
				// Help flag returns ErrHelp which is expected
				if tt.name == "help flag" {
					return
				}
				t.Errorf("Execute() error = %v", err)
			}
		})
	}
}
```

#### 3.4 Update cmd/tokensmith/main.go
```go
package main

import "github.com/holos-run/tokensmith/cmd/tokensmith/commands"

func main() {
	commands.Execute()
}
```

### 4. Update Makefile

#### 4.1 Add useful targets
```makefile
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
```

### 5. Update .gitignore

Add entries for build artifacts and common editor files:
```gitignore
# Binaries
bin/
*.exe
*.exe~
*.dll
*.so
*.dylib

# Test binary, built with `go test -c`
*.test

# Output of the go coverage tool
*.out
coverage.txt

# Go workspace file
go.work
go.work.sum

# IDE
.idea/
.vscode/
*.swp
*.swo
*~

# macOS
.DS_Store

# Generated files
*.pb.go
*connect.go
```

## Implementation Checklist

- [ ] Add Cobra dependency to go.mod
- [ ] Add Connect RPC dependency to go.mod
- [ ] Add protobuf dependency to go.mod
- [ ] Create tools.go for build tools
- [ ] Create internal/logging/logging.go
- [ ] Create internal/logging/logging_test.go with table-driven tests
- [ ] Create cmd/tokensmith/commands/root.go
- [ ] Create cmd/tokensmith/commands/version.go
- [ ] Create cmd/tokensmith/commands/root_test.go
- [ ] Update cmd/tokensmith/main.go to use Cobra
- [ ] Update Makefile with new targets
- [ ] Update .gitignore
- [ ] Run `go mod tidy`
- [ ] Run `make test` - should pass
- [ ] Run `make build` - should succeed
- [ ] Run `./bin/tokensmith --help` - should show good help output
- [ ] Run `./bin/tokensmith version` - should show version info
- [ ] Test different log levels and formats
- [ ] Commit changes

## Success Criteria

1. ✅ `go mod tidy` completes without errors
2. ✅ `make build` successfully builds the binary
3. ✅ `make test` runs and all tests pass
4. ✅ `./bin/tokensmith --help` shows comprehensive help text
5. ✅ `./bin/tokensmith version` displays version information
6. ✅ `./bin/tokensmith --log-level debug version` outputs JSON logs
7. ✅ `./bin/tokensmith --log-format text version` outputs text logs
8. ✅ Table-driven tests demonstrate testing pattern for the project
9. ✅ All code follows Go conventions and passes `go fmt`
10. ✅ Project structure is clean and ready for feature development

## Testing the Implementation

After implementation, verify with these commands:

```bash
# Build the project
make build

# Run tests
make test

# Test help output
./bin/tokensmith --help

# Test version command
./bin/tokensmith version

# Test JSON logging (default)
./bin/tokensmith --log-level debug version

# Test text logging
./bin/tokensmith --log-format text version

# Test error handling with invalid log level
./bin/tokensmith --log-level invalid version
```

## Next Steps

After this plan is complete:
1. Plan 003: Implement OIDC token exchange overview (future)
2. Plan 004: Implement Connect RPC greet service (demonstrates full RPC flow)

## Notes

- This plan focuses on infrastructure and does not implement business logic
- The version command serves as a simple example of adding commands
- Table-driven tests in logging package establish the pattern for the project
- All logging is structured and outputs JSON by default for easy parsing
- Cobra provides excellent help generation out of the box
