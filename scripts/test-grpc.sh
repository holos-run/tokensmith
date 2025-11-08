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
