package authz

import (
	"testing"

	envoy_auth "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	"google.golang.org/grpc/codes"
)

func TestExtractBearerToken(t *testing.T) {
	tests := []struct {
		name        string
		headers     map[string]string
		expectError bool
		expected    string
	}{
		{
			name: "valid bearer token",
			headers: map[string]string{
				"authorization": "Bearer my-token-here",
			},
			expectError: false,
			expected:    "my-token-here",
		},
		{
			name: "valid bearer token with long value",
			headers: map[string]string{
				"authorization": "Bearer eyJhbGciOiJSUzI1NiIsImtpZCI6IjEyMyJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.signature",
			},
			expectError: false,
			expected:    "eyJhbGciOiJSUzI1NiIsImtpZCI6IjEyMyJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.signature",
		},
		{
			name:        "missing authorization header",
			headers:     map[string]string{},
			expectError: true,
			expected:    "",
		},
		{
			name: "not a bearer token",
			headers: map[string]string{
				"authorization": "Basic dXNlcjpwYXNz",
			},
			expectError: true,
			expected:    "",
		},
		{
			name: "empty bearer token",
			headers: map[string]string{
				"authorization": "Bearer ",
			},
			expectError: true,
			expected:    "",
		},
		{
			name: "missing Bearer prefix",
			headers: map[string]string{
				"authorization": "my-token-here",
			},
			expectError: true,
			expected:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a CheckRequest with the test headers
			req := &envoy_auth.CheckRequest{
				Attributes: &envoy_auth.AttributeContext{
					Request: &envoy_auth.AttributeContext_Request{
						Http: &envoy_auth.AttributeContext_HttpRequest{
							Headers: tt.headers,
						},
					},
				},
			}

			result, err := extractBearerToken(req)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("token mismatch: got %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestGetPath(t *testing.T) {
	tests := []struct {
		name     string
		req      *envoy_auth.CheckRequest
		expected string
	}{
		{
			name: "valid path",
			req: &envoy_auth.CheckRequest{
				Attributes: &envoy_auth.AttributeContext{
					Request: &envoy_auth.AttributeContext_Request{
						Http: &envoy_auth.AttributeContext_HttpRequest{
							Path: "/api/v1/namespaces/default/secrets",
						},
					},
				},
			},
			expected: "/api/v1/namespaces/default/secrets",
		},
		{
			name:     "nil request",
			req:      &envoy_auth.CheckRequest{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getPath(tt.req)
			if result != tt.expected {
				t.Errorf("path mismatch: got %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestGetMethod(t *testing.T) {
	tests := []struct {
		name     string
		req      *envoy_auth.CheckRequest
		expected string
	}{
		{
			name: "GET method",
			req: &envoy_auth.CheckRequest{
				Attributes: &envoy_auth.AttributeContext{
					Request: &envoy_auth.AttributeContext_Request{
						Http: &envoy_auth.AttributeContext_HttpRequest{
							Method: "GET",
						},
					},
				},
			},
			expected: "GET",
		},
		{
			name: "POST method",
			req: &envoy_auth.CheckRequest{
				Attributes: &envoy_auth.AttributeContext{
					Request: &envoy_auth.AttributeContext_Request{
						Http: &envoy_auth.AttributeContext_HttpRequest{
							Method: "POST",
						},
					},
				},
			},
			expected: "POST",
		},
		{
			name:     "nil request",
			req:      &envoy_auth.CheckRequest{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getMethod(tt.req)
			if result != tt.expected {
				t.Errorf("method mismatch: got %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestHttpStatusFromGRPCCode(t *testing.T) {
	tests := []struct {
		name     string
		code     codes.Code
		expected int32
	}{
		{name: "OK", code: codes.OK, expected: 200},
		{name: "Unauthenticated", code: codes.Unauthenticated, expected: 401},
		{name: "PermissionDenied", code: codes.PermissionDenied, expected: 403},
		{name: "NotFound", code: codes.NotFound, expected: 404},
		{name: "Internal", code: codes.Internal, expected: 500},
		{name: "Unavailable", code: codes.Unavailable, expected: 503},
		{name: "Unknown", code: codes.Code(999), expected: 500},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := httpStatusFromGRPCCode(tt.code)
			if result != tt.expected {
				t.Errorf("status mismatch: got %d, want %d", result, tt.expected)
			}
		})
	}
}
