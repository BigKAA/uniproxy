// Package logging provides configurable slog.Logger construction.
package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/BigKAA/uniproxy/internal/config"
)

// NewLogger creates a configured *slog.Logger from LogConfig.
// Output is always os.Stderr.
func NewLogger(cfg config.LogConfig) *slog.Logger {
	return slog.New(newHandler(cfg, os.Stderr))
}

// newHandler creates a slog.Handler writing to w.
// Exported via NewLogger; used directly in tests.
func newHandler(cfg config.LogConfig, w io.Writer) slog.Handler {
	level := parseLevel(cfg.Level)

	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: cfg.AddSource,
	}

	switch cfg.Format {
	case "json":
		opts.ReplaceAttr = buildReplaceAttr(cfg)
		return slog.NewJSONHandler(w, opts)
	default: // "text"
		return slog.NewTextHandler(w, opts)
	}
}

// parseLevel converts a string to slog.Level. Defaults to Info on error.
func parseLevel(s string) slog.Level {
	var level slog.Level
	if err := level.UnmarshalText([]byte(strings.ToUpper(s))); err != nil {
		return slog.LevelInfo
	}
	return level
}

// buildReplaceAttr creates a ReplaceAttr function for the JSON handler
// that customizes time format and key names.
func buildReplaceAttr(cfg config.LogConfig) func([]string, slog.Attr) slog.Attr {
	return func(groups []string, a slog.Attr) slog.Attr {
		if len(groups) != 0 {
			return a // only customize top-level built-in keys
		}

		switch a.Key {
		case slog.TimeKey:
			if cfg.TimeKey != "" {
				a.Key = cfg.TimeKey
			}
			if t, ok := a.Value.Any().(time.Time); ok {
				switch cfg.TimeFormat {
				case "rfc3339":
					a.Value = slog.StringValue(t.Format(time.RFC3339))
				case "unix":
					a = slog.Int64(a.Key, t.Unix())
				case "unixmilli":
					a = slog.Int64(a.Key, t.UnixMilli())
				// "rfc3339nano" = slog default, no transform needed
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
