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
