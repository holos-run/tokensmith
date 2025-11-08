package token

import (
	"testing"
)

func TestParseServiceAccountIdentity(t *testing.T) {
	tests := []struct {
		name        string
		username    string
		uid         string
		expectError bool
		expected    *ServiceAccountIdentity
	}{
		{
			name:        "valid service account",
			username:    "system:serviceaccount:default:my-service",
			uid:         "12345",
			expectError: false,
			expected: &ServiceAccountIdentity{
				Namespace: "default",
				Name:      "my-service",
				UID:       "12345",
				Username:  "system:serviceaccount:default:my-service",
			},
		},
		{
			name:        "valid service account with hyphens",
			username:    "system:serviceaccount:app-prod:eso-sa",
			uid:         "67890",
			expectError: false,
			expected: &ServiceAccountIdentity{
				Namespace: "app-prod",
				Name:      "eso-sa",
				UID:       "67890",
				Username:  "system:serviceaccount:app-prod:eso-sa",
			},
		},
		{
			name:        "not a service account",
			username:    "system:node:my-node",
			uid:         "11111",
			expectError: true,
			expected:    nil,
		},
		{
			name:        "missing namespace",
			username:    "system:serviceaccount::my-service",
			uid:         "22222",
			expectError: true,
			expected:    nil,
		},
		{
			name:        "missing name",
			username:    "system:serviceaccount:default:",
			uid:         "33333",
			expectError: true,
			expected:    nil,
		},
		{
			name:        "malformed - only one part",
			username:    "system:serviceaccount:default",
			uid:         "44444",
			expectError: true,
			expected:    nil,
		},
		{
			name:        "regular user",
			username:    "alice@example.com",
			uid:         "55555",
			expectError: true,
			expected:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseServiceAccountIdentity(tt.username, tt.uid)

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

			if result.Namespace != tt.expected.Namespace {
				t.Errorf("namespace mismatch: got %q, want %q", result.Namespace, tt.expected.Namespace)
			}

			if result.Name != tt.expected.Name {
				t.Errorf("name mismatch: got %q, want %q", result.Name, tt.expected.Name)
			}

			if result.UID != tt.expected.UID {
				t.Errorf("uid mismatch: got %q, want %q", result.UID, tt.expected.UID)
			}

			if result.Username != tt.expected.Username {
				t.Errorf("username mismatch: got %q, want %q", result.Username, tt.expected.Username)
			}
		})
	}
}
