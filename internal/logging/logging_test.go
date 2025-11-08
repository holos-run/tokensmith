package logging

import (
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
