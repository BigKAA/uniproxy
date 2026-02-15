package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/BigKAA/uniproxy/internal/config"
)

func TestNewHandler_DefaultConfig(t *testing.T) {
	var buf bytes.Buffer
	h := newHandler(config.LogConfig{}, &buf)
	logger := slog.New(h)

	logger.Info("hello")
	out := buf.String()
	if !strings.Contains(out, "hello") {
		t.Errorf("expected output to contain 'hello', got %q", out)
	}
	// Default is text handler — should have key=value format.
	if strings.HasPrefix(out, "{") {
		t.Error("expected text format, got JSON")
	}
}

func TestNewHandler_JSONFormat(t *testing.T) {
	var buf bytes.Buffer
	h := newHandler(config.LogConfig{Format: "json", Level: "info"}, &buf)
	logger := slog.New(h)

	logger.Info("test message", "key", "value")
	out := buf.String()

	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, out)
	}
	if m["msg"] != "test message" {
		t.Errorf("msg = %v, want %q", m["msg"], "test message")
	}
	if m["key"] != "value" {
		t.Errorf("key = %v, want %q", m["key"], "value")
	}
}

func TestNewHandler_TextFormat(t *testing.T) {
	var buf bytes.Buffer
	h := newHandler(config.LogConfig{Format: "text", Level: "info"}, &buf)
	logger := slog.New(h)

	logger.Info("hello world")
	out := buf.String()
	if !strings.Contains(out, "msg=\"hello world\"") && !strings.Contains(out, "msg=hello") {
		t.Errorf("expected text key=value format, got %q", out)
	}
}

func TestNewHandler_LevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	h := newHandler(config.LogConfig{Format: "text", Level: "warn"}, &buf)
	logger := slog.New(h)

	logger.Info("should not appear")
	if buf.Len() != 0 {
		t.Errorf("info message should be filtered at warn level, got %q", buf.String())
	}

	logger.Warn("should appear")
	if buf.Len() == 0 {
		t.Error("warn message should not be filtered at warn level")
	}
}

func TestNewHandler_JSONTimeFormat_RFC3339(t *testing.T) {
	var buf bytes.Buffer
	h := newHandler(config.LogConfig{Format: "json", Level: "info", TimeFormat: "rfc3339"}, &buf)
	logger := slog.New(h)

	logger.Info("test")

	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	ts, ok := m["time"].(string)
	if !ok {
		t.Fatalf("time field is not a string: %v", m["time"])
	}
	// RFC3339 has no nanoseconds — ends with "Z" or "+HH:MM", no "." before it.
	if strings.Contains(ts, ".") {
		t.Errorf("expected rfc3339 (no nanoseconds), got %q", ts)
	}
}

func TestNewHandler_JSONTimeFormat_Unix(t *testing.T) {
	var buf bytes.Buffer
	h := newHandler(config.LogConfig{Format: "json", Level: "info", TimeFormat: "unix"}, &buf)
	logger := slog.New(h)

	logger.Info("test")

	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	ts, ok := m["time"].(float64)
	if !ok {
		t.Fatalf("time field should be a number, got %T: %v", m["time"], m["time"])
	}
	if ts < 1e9 {
		t.Errorf("unix timestamp seems too small: %v", ts)
	}
}

func TestNewHandler_JSONTimeFormat_UnixMilli(t *testing.T) {
	var buf bytes.Buffer
	h := newHandler(config.LogConfig{Format: "json", Level: "info", TimeFormat: "unixmilli"}, &buf)
	logger := slog.New(h)

	logger.Info("test")

	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	ts, ok := m["time"].(float64)
	if !ok {
		t.Fatalf("time field should be a number, got %T: %v", m["time"], m["time"])
	}
	if ts < 1e12 {
		t.Errorf("unixmilli timestamp seems too small: %v", ts)
	}
}

func TestNewHandler_JSONCustomKeys(t *testing.T) {
	var buf bytes.Buffer
	cfg := config.LogConfig{
		Format:     "json",
		Level:      "info",
		TimeKey:    "@timestamp",
		LevelKey:   "severity",
		MessageKey: "message",
	}
	h := newHandler(cfg, &buf)
	logger := slog.New(h)

	logger.Info("custom keys test")

	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, ok := m["@timestamp"]; !ok {
		t.Error("expected @timestamp key in JSON output")
	}
	if _, ok := m["time"]; ok {
		t.Error("default 'time' key should be renamed")
	}
	if m["severity"] == nil {
		t.Error("expected severity key in JSON output")
	}
	if m["message"] != "custom keys test" {
		t.Errorf("message = %v, want %q", m["message"], "custom keys test")
	}
	if _, ok := m["msg"]; ok {
		t.Error("default 'msg' key should be renamed")
	}
}

func TestNewHandler_AddSource(t *testing.T) {
	var buf bytes.Buffer
	cfg := config.LogConfig{
		Format:    "json",
		Level:     "info",
		AddSource: true,
	}
	h := newHandler(cfg, &buf)
	logger := slog.New(h)

	logger.Info("source test")

	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, ok := m["source"]; !ok {
		t.Error("expected 'source' field when AddSource is true")
	}
}

func TestNewHandler_CustomKeys_TextIgnored(t *testing.T) {
	var buf bytes.Buffer
	cfg := config.LogConfig{
		Format:     "text",
		Level:      "info",
		TimeKey:    "@timestamp",
		LevelKey:   "severity",
		MessageKey: "message",
	}
	h := newHandler(cfg, &buf)
	logger := slog.New(h)

	logger.Info("text test")
	out := buf.String()

	// Text handler should not rename keys — standard "msg=" should be present.
	if strings.Contains(out, "@timestamp") || strings.Contains(out, "severity") {
		t.Errorf("text format should ignore custom keys, got %q", out)
	}
}
