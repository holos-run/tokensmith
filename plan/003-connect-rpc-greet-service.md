# Plan 003: Connect RPC Greet Service

**Status**: Approved

## Overview
Implement a Connect RPC gRPC server with a simple greet endpoint, code generation using `go generate`, Cobra-based CLI commands, and structured JSON logging using the standard `slog` package.

## Goals
1. Set up Connect RPC (gRPC-compatible) server with a single `Greet` endpoint
2. Implement code generation using `go generate` for protobuf/Connect
3. Add Makefile target to run `go generate`
4. Create Cobra-based CLI with two commands:
   - `greet` - Client command to call the greet service
   - `serve` - Server command to start the gRPC server
5. Use standard library `slog` package for JSON logging by default

## Technical Decisions

### Connect RPC vs Traditional gRPC
- **Connect RPC** (connectrpc.com) - Modern, simpler gRPC implementation
  - Better ergonomics than traditional gRPC
  - Compatible with gRPC protocol
  - Simpler code generation
  - Better HTTP/1.1 and browser support

### Project Structure
```
tokensmith/
├── api/
│   └── greet/
│       └── v1/
│           ├── greet.proto              # Protobuf definition
│           └── greetv1connect/          # Generated Connect code (gitignored)
├── cmd/
│   └── tokensmith/
│       ├── main.go                      # Entry point with Cobra root
│       └── commands/
│           ├── root.go                  # Root command setup
│           ├── serve.go                 # Server command
│           └── greet.go                 # Client command
├── internal/
│   ├── server/
│   │   └── greet.go                     # Greet service implementation
│   └── logging/
│       └── logging.go                   # Centralized slog setup
└── tools.go                             # Go tool dependencies
```

### Dependencies
```go
// Core dependencies
connectrpc.com/connect                    // Connect RPC framework
github.com/bufbuild/protoplugin           // Protobuf plugin
google.golang.org/protobuf                // Protobuf runtime
github.com/spf13/cobra                    // CLI framework

// Code generation tools (tools.go)
github.com/bufbuild/buf/cmd/buf          // Buf CLI for protobuf
connectrpc.com/connect/cmd/protoc-gen-connect-go  // Connect code generator
google.golang.org/protobuf/cmd/protoc-gen-go      // Standard protobuf generator
```

## Implementation Steps

### 1. Project Setup and Dependencies

#### 1.1 Add Go module dependencies
```bash
go get connectrpc.com/connect
go get github.com/spf13/cobra
go get google.golang.org/protobuf
```

#### 1.2 Create tools.go for build-time dependencies
This ensures code generation tools are tracked and versioned:
```go
//go:build tools
package tools

import (
    _ "github.com/bufbuild/buf/cmd/buf"
    _ "connectrpc.com/connect/cmd/protoc-gen-connect-go"
    _ "google.golang.org/protobuf/cmd/protoc-gen-go"
)
```

#### 1.3 Install tools
```bash
go mod download
go install github.com/bufbuild/buf/cmd/buf@latest
go install connectrpc.com/connect/cmd/protoc-gen-connect-go@latest
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
```

### 2. Protobuf Definition

#### 2.1 Create api/greet/v1/greet.proto
```protobuf
syntax = "proto3";

package greet.v1;

option go_package = "github.com/holos-run/tokensmith/api/greet/v1;greetv1";

// GreetService provides a simple greeting service.
service GreetService {
  // Greet returns a greeting message.
  rpc Greet(GreetRequest) returns (GreetResponse) {}
}

// GreetRequest is the request message for Greet.
message GreetRequest {
  // name is the name to greet.
  string name = 1;
}

// GreetResponse is the response message for Greet.
message GreetResponse {
  // greeting is the greeting message.
  string greeting = 1;
}
```

#### 2.2 Create buf.yaml for buf configuration
```yaml
version: v2
modules:
  - path: api
lint:
  use:
    - STANDARD
breaking:
  use:
    - FILE
```

#### 2.3 Create buf.gen.yaml for code generation
```yaml
version: v2
managed:
  enabled: true
  override:
    - file_option: go_package_prefix
      value: github.com/holos-run/tokensmith
plugins:
  - local: protoc-gen-go
    out: .
    opt:
      - paths=source_relative
  - local: protoc-gen-connect-go
    out: .
    opt:
      - paths=source_relative
```

### 3. Code Generation Setup

#### 3.1 Add //go:generate directives
Create `api/greet/v1/generate.go`:
```go
package greetv1

//go:generate buf generate --template ../../../buf.gen.yaml --path api/greet/v1
```

#### 3.2 Update .gitignore
Add generated files to .gitignore:
```
# Generated protobuf files
*.pb.go
*connect.go
api/**/v*/greetv1connect/
```

#### 3.3 Update Makefile
Add generate target:
```makefile
.PHONY: generate
generate:
	go generate ./...

.PHONY: build
build: generate
	go build -o bin/tokensmith ./cmd/tokensmith
```

### 4. Logging Setup

#### 4.1 Create internal/logging/logging.go
Centralized slog configuration with JSON output:
```go
package logging

import (
	"log/slog"
	"os"
)

// NewLogger creates a new JSON logger with the given level.
func NewLogger(level slog.Level) *slog.Logger {
	opts := &slog.HandlerOptions{
		Level: level,
	}
	handler := slog.NewJSONHandler(os.Stdout, opts)
	return slog.New(handler)
}

// SetDefault sets the default logger to JSON with the given level.
func SetDefault(level slog.Level) {
	slog.SetDefault(NewLogger(level))
}
```

### 5. Service Implementation

#### 5.1 Create internal/server/greet.go
Implement the GreetService:
```go
package server

import (
	"context"
	"fmt"
	"log/slog"

	"connectrpc.com/connect"
	greetv1 "github.com/holos-run/tokensmith/api/greet/v1"
)

// GreetServer implements the GreetService.
type GreetServer struct{}

// NewGreetServer creates a new GreetServer.
func NewGreetServer() *GreetServer {
	return &GreetServer{}
}

// Greet implements the Greet RPC method.
func (s *GreetServer) Greet(
	ctx context.Context,
	req *connect.Request[greetv1.GreetRequest],
) (*connect.Response[greetv1.GreetResponse], error) {
	name := req.Msg.Name
	if name == "" {
		name = "World"
	}

	slog.Info("processing greet request",
		slog.String("name", name))

	greeting := fmt.Sprintf("Hello, %s!", name)
	res := connect.NewResponse(&greetv1.GreetResponse{
		Greeting: greeting,
	})

	return res, nil
}
```

### 6. Cobra CLI Commands

#### 6.1 Create cmd/tokensmith/commands/root.go
```go
package commands

import (
	"log/slog"
	"os"

	"github.com/holos-run/tokensmith/internal/logging"
	"github.com/spf13/cobra"
)

var (
	logLevel string
)

// NewRootCmd creates the root command.
func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tokensmith",
		Short: "Tokensmith - Envoy External Authorizer for OIDC Token Exchange",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// Set up logging
			level := slog.LevelInfo
			switch logLevel {
			case "debug":
				level = slog.LevelDebug
			case "warn":
				level = slog.LevelWarn
			case "error":
				level = slog.LevelError
			}
			logging.SetDefault(level)
		},
	}

	cmd.PersistentFlags().StringVar(&logLevel, "log-level", "info",
		"Log level (debug, info, warn, error)")

	// Add subcommands
	cmd.AddCommand(NewServeCmd())
	cmd.AddCommand(NewGreetCmd())

	return cmd
}

// Execute runs the root command.
func Execute() {
	if err := NewRootCmd().Execute(); err != nil {
		slog.Error("command failed", slog.Any("error", err))
		os.Exit(1)
	}
}
```

#### 6.2 Create cmd/tokensmith/commands/serve.go
```go
package commands

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"connectrpc.com/connect"
	greetv1connect "github.com/holos-run/tokensmith/api/greet/v1/greetv1connect"
	"github.com/holos-run/tokensmith/internal/server"
	"github.com/spf13/cobra"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

var (
	serveAddr string
	servePort int
)

// NewServeCmd creates the serve command.
func NewServeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the gRPC server",
		RunE:  runServe,
	}

	cmd.Flags().StringVar(&serveAddr, "addr", "localhost", "Server address")
	cmd.Flags().IntVar(&servePort, "port", 8080, "Server port")

	return cmd
}

func runServe(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// Create the greet service
	greetSvc := server.NewGreetServer()
	path, handler := greetv1connect.NewGreetServiceHandler(greetSvc)

	mux := http.NewServeMux()
	mux.Handle(path, handler)

	addr := fmt.Sprintf("%s:%d", serveAddr, servePort)

	// Use h2c for HTTP/2 without TLS (suitable for development)
	srv := &http.Server{
		Addr:    addr,
		Handler: h2c.NewHandler(mux, &http2.Server{}),
	}

	slog.Info("starting grpc server",
		slog.String("addr", addr),
		slog.String("service", "GreetService"))

	// Start server in goroutine
	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// Wait for interrupt signal or error
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	case sig := <-sigCh:
		slog.Info("received signal, shutting down",
			slog.String("signal", sig.String()))
	}

	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown error: %w", err)
	}

	slog.Info("server stopped")
	return nil
}
```

#### 6.3 Create cmd/tokensmith/commands/greet.go
```go
package commands

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"connectrpc.com/connect"
	greetv1 "github.com/holos-run/tokensmith/api/greet/v1"
	greetv1connect "github.com/holos-run/tokensmith/api/greet/v1/greetv1connect"
	"github.com/spf13/cobra"
)

var (
	greetServerURL string
	greetName      string
	greetTimeout   time.Duration
)

// NewGreetCmd creates the greet command.
func NewGreetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "greet",
		Short: "Call the greet service",
		RunE:  runGreet,
	}

	cmd.Flags().StringVar(&greetServerURL, "server", "http://localhost:8080",
		"Server URL")
	cmd.Flags().StringVar(&greetName, "name", "",
		"Name to greet (default: World)")
	cmd.Flags().DurationVar(&greetTimeout, "timeout", 5*time.Second,
		"Request timeout")

	return cmd
}

func runGreet(cmd *cobra.Command, args []string) error {
	// Create client
	client := greetv1connect.NewGreetServiceClient(
		http.DefaultClient,
		greetServerURL,
	)

	// Create request
	req := connect.NewRequest(&greetv1.GreetRequest{
		Name: greetName,
	})

	ctx, cancel := context.WithTimeout(cmd.Context(), greetTimeout)
	defer cancel()

	slog.Debug("calling greet service",
		slog.String("server", greetServerURL),
		slog.String("name", greetName))

	// Call the service
	res, err := client.Greet(ctx, req)
	if err != nil {
		return fmt.Errorf("greet failed: %w", err)
	}

	// Print the result to stdout
	fmt.Println(res.Msg.Greeting)

	return nil
}
```

#### 6.4 Update cmd/tokensmith/main.go
```go
package main

import (
	"github.com/holos-run/tokensmith/cmd/tokensmith/commands"
)

func main() {
	commands.Execute()
}
```

### 7. Testing

#### 7.1 Add unit test for greet service
Create `internal/server/greet_test.go`:
```go
package server

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	greetv1 "github.com/holos-run/tokensmith/api/greet/v1"
)

func TestGreetServer_Greet(t *testing.T) {
	tests := []struct {
		name     string
		reqName  string
		wantText string
	}{
		{
			name:     "with name",
			reqName:  "Alice",
			wantText: "Hello, Alice!",
		},
		{
			name:     "empty name defaults to World",
			reqName:  "",
			wantText: "Hello, World!",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := NewGreetServer()
			req := connect.NewRequest(&greetv1.GreetRequest{
				Name: tt.reqName,
			})

			res, err := srv.Greet(context.Background(), req)
			if err != nil {
				t.Fatalf("Greet() error = %v", err)
			}

			if got := res.Msg.Greeting; got != tt.wantText {
				t.Errorf("Greet() = %v, want %v", got, tt.wantText)
			}
		})
	}
}
```

### 8. gRPC Testing Script

#### 8.1 Create scripts/test-grpc.sh
Create a test script that uses `grpcurl` to verify the service works as a standard gRPC service:

```bash
#!/usr/bin/env bash
# scripts/test-grpc.sh - Test the greet service using grpcurl

set -euo pipefail

GRPC_URL="${GRPC_URL:-localhost:8080}"

echo "Testing gRPC service at ${GRPC_URL}"
echo ""

# Test 1: List services (requires reflection)
echo "1. Listing available services..."
grpcurl -plaintext "${GRPC_URL}" list || echo "  ⚠️  Reflection not enabled (expected for now)"
echo ""

# Test 2: Call greet with name
echo "2. Testing greet with name 'Alice'..."
grpcurl -plaintext \
  -d '{"name": "Alice"}' \
  "${GRPC_URL}" \
  greet.v1.GreetService/Greet
echo ""

# Test 3: Call greet without name (should default to World)
echo "3. Testing greet without name (should default to 'World')..."
grpcurl -plaintext \
  -d '{}' \
  "${GRPC_URL}" \
  greet.v1.GreetService/Greet
echo ""

echo "✅ All gRPC tests passed!"
```

Make the script executable:
```bash
chmod +x scripts/test-grpc.sh
```

#### 8.2 Add grpcurl installation instructions
Document how to install `grpcurl` for testing:

**macOS:**
```bash
brew install grpcurl
```

**Linux:**
```bash
go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest
```

**Usage:**
```bash
# Start the server in one terminal
./bin/tokensmith serve

# Run the test script in another terminal
./scripts/test-grpc.sh
```

### 9. Documentation Updates

#### 9.1 Update README.md
Add section about the greet service and new commands:
```markdown
## Quick Start

### Start the server
```bash
make build
./bin/tokensmith serve --port 8080
```

### Call the greet service

#### Using the built-in client
```bash
./bin/tokensmith greet --name Alice
# Output: Hello, Alice!
```

#### Using grpcurl (standard gRPC tool)
```bash
# Install grpcurl first (macOS)
brew install grpcurl

# Test the service
grpcurl -plaintext -d '{"name": "Alice"}' localhost:8080 greet.v1.GreetService/Greet

# Or use the test script
./scripts/test-grpc.sh
```

### Commands
- `tokensmith serve` - Start the gRPC server
- `tokensmith greet` - Call the greet service (client)
- `--log-level` - Set log level (debug, info, warn, error)
```

## Implementation Checklist

- [ ] Add Go dependencies (connect, cobra, protobuf)
- [ ] Create tools.go for build-time tools
- [ ] Create protobuf definition (api/greet/v1/greet.proto)
- [ ] Create buf configuration (buf.yaml, buf.gen.yaml)
- [ ] Add go:generate directives
- [ ] Update .gitignore for generated files
- [ ] Update Makefile with generate target
- [ ] Run code generation
- [ ] Create logging package (internal/logging)
- [ ] Implement GreetServer (internal/server/greet.go)
- [ ] Create Cobra root command (cmd/tokensmith/commands/root.go)
- [ ] Create serve command (cmd/tokensmith/commands/serve.go)
- [ ] Create greet command (cmd/tokensmith/commands/greet.go)
- [ ] Update main.go to use Cobra
- [ ] Add unit tests for greet service
- [ ] Create scripts/test-grpc.sh for grpcurl testing
- [ ] Update README.md with usage examples
- [ ] Test end-to-end: start server, call greet with built-in client
- [ ] Test with grpcurl to verify standard gRPC compatibility
- [ ] Verify JSON logging output
- [ ] Commit changes

## Success Criteria

1. ✅ `make generate` successfully generates Connect RPC code
2. ✅ `make build` builds without errors
3. ✅ `make test` passes all tests
4. ✅ `tokensmith serve` starts the gRPC server
5. ✅ `tokensmith greet --name Alice` returns "Hello, Alice!"
6. ✅ `grpcurl -plaintext -d '{"name": "Alice"}' localhost:8080 greet.v1.GreetService/Greet` works
7. ✅ `./scripts/test-grpc.sh` passes all gRPC compatibility tests
8. ✅ Logs are output in JSON format
9. ✅ `--log-level` flag controls log verbosity

## Future Considerations

- Add TLS support for production deployments
- Add metrics/observability (Prometheus, OpenTelemetry)
- Add graceful shutdown handling
- Add health check endpoint
- Consider adding reflection for easier debugging
- Add integration tests with real server
