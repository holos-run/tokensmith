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
