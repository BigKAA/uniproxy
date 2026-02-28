// Package logging provides configurable structured logging based on the standard
// library's slog package. It supports text and JSON output formats, configurable
// log levels, multiple timestamp formats, and custom JSON key names.
//
// All log output is written to os.Stderr to keep stdout clean for application output.
package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/BigKAA/uniproxy/internal/config"
)

// NewLogger creates a configured *slog.Logger from the given LogConfig.
// The logger always writes to os.Stderr. The returned logger should be set as
// the default via slog.SetDefault() for consistent logging across the application.
func NewLogger(cfg config.LogConfig) *slog.Logger {
	return slog.New(newHandler(cfg, os.Stderr))
}

// newHandler creates a slog.Handler based on the configured format (text or JSON).
// For JSON format, it applies custom attribute replacements (key names, time format).
// For text format, it uses the default slog.TextHandler with configured level and source.
//
// This function is also used directly in tests with a custom io.Writer.
func newHandler(cfg config.LogConfig, w io.Writer) slog.Handler {
	level := parseLevel(cfg.Level)

	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: cfg.AddSource,
	}

	switch cfg.Format {
	case "json":
		// JSON handler gets custom attribute replacer for key names and time formatting.
		opts.ReplaceAttr = buildReplaceAttr(cfg)
		return slog.NewJSONHandler(w, opts)
	default: // "text"
		return slog.NewTextHandler(w, opts)
	}
}

// parseLevel converts a log level string to the corresponding slog.Level.
// Uses slog's built-in level parsing which supports "DEBUG", "INFO", "WARN", "ERROR".
// Returns slog.LevelInfo as a safe default if the string is unrecognized.
func parseLevel(s string) slog.Level {
	var level slog.Level
	if err := level.UnmarshalText([]byte(strings.ToUpper(s))); err != nil {
		return slog.LevelInfo
	}
	return level
}

// buildReplaceAttr creates a ReplaceAttr function for the slog JSON handler.
// This function is called for each attribute during JSON serialization and allows:
//   - Customizing the timestamp format (rfc3339, unix, unixmilli; rfc3339nano is the slog default).
//   - Renaming built-in JSON keys (time, level, msg, source) via LOG_*_KEY env vars.
//
// Only top-level built-in keys are customized; nested/group attributes pass through unchanged.
func buildReplaceAttr(cfg config.LogConfig) func([]string, slog.Attr) slog.Attr {
	return func(groups []string, a slog.Attr) slog.Attr {
		// Only customize top-level built-in keys, not nested group attributes.
		if len(groups) != 0 {
			return a
		}

		switch a.Key {
		case slog.TimeKey:
			// Rename the time key if configured (e.g. "time" -> "timestamp").
			if cfg.TimeKey != "" {
				a.Key = cfg.TimeKey
			}
			// Apply custom time format if the value is a time.Time.
			if t, ok := a.Value.Any().(time.Time); ok {
				switch cfg.TimeFormat {
				case "rfc3339":
					a.Value = slog.StringValue(t.Format(time.RFC3339))
				case "unix":
					// Unix timestamp as integer seconds.
					a = slog.Int64(a.Key, t.Unix())
				case "unixmilli":
					// Unix timestamp as integer milliseconds.
					a = slog.Int64(a.Key, t.UnixMilli())
				// "rfc3339nano" is the slog default — no transformation needed.
				}
			}
		case slog.LevelKey:
			if cfg.LevelKey != "" {
				a.Key = cfg.LevelKey
			}
		case slog.MessageKey:
			if cfg.MessageKey != "" {
				a.Key = cfg.MessageKey
			}
		case slog.SourceKey:
			if cfg.SourceKey != "" {
				a.Key = cfg.SourceKey
			}
		}
		return a
	}
}
