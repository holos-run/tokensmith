//go:build tools

// Package tools tracks tool dependencies for the project.
package tools

import (
	_ "github.com/bufbuild/buf/cmd/buf"
	_ "connectrpc.com/connect/cmd/protoc-gen-connect-go"
	_ "google.golang.org/protobuf/cmd/protoc-gen-go"
)
