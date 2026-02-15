# Requirements: Configurable JSON Logging

## Metadata

- **Version**: 1.0.0
- **Created**: 2026-02-15
- **Status**: Requirements Defined
- **Source**: Brainstorm session

---

## Goals

uniproxy should support configurable structured logging (text/JSON) to work in diverse environments: Kubernetes, Docker, bare-metal, development machines. Users should be able to customize the log format, time representation, and JSON key names through environment variables.

---

## Functional Requirements

### FR-1: Log Format Selection

- **Env var**: `LOG_FORMAT`
- **Values**: `text`, `json`
- **Default**: `text` (backward compatible)
- **Behavior**: `text` uses `slog.TextHandler`, `json` uses `slog.JSONHandler`

### FR-2: Log Level

- **Env var**: `LOG_LEVEL`
- **Values**: `debug`, `info`, `warn`, `error` (case-insensitive)
- **Default**: `info`
- **Change from current**: Currently only distinguishes `debug` vs everything else. New behavior uses `slog.Level.UnmarshalText()` for full support.
- **Backward compat**: `debug` still works as before

### FR-3: Time Format

- **Env var**: `LOG_TIME_FORMAT`
- **Values**: `rfc3339`, `rfc3339nano`, `unix`, `unixmilli`
- **Default**: `rfc3339nano` (slog's native default)
- **Behavior**: Only affects JSON handler output. Implemented via `ReplaceAttr`.
- **Examples**:
  - `rfc3339`: `"2026-02-15T12:00:00Z"`
  - `rfc3339nano`: `"2026-02-15T12:00:00.123456789Z"` (default)
  - `unix`: `1739620800` (integer seconds)
  - `unixmilli`: `1739620800123` (integer milliseconds)

### FR-4: Source Location

- **Env var**: `LOG_ADD_SOURCE`
- **Values**: `yes`/`no`/`true`/`false`/`1`/`0` (via `parseBool`)
- **Default**: `no`
- **Behavior**: When enabled, adds file:line information to each log entry via `slog.HandlerOptions.AddSource`

### FR-5: Custom JSON Key Names

- **Env vars**:
  - `LOG_TIME_KEY` — key name for timestamp (default: `time`)
  - `LOG_LEVEL_KEY` — key name for level (default: `level`)
  - `LOG_MESSAGE_KEY` — key name for message (default: `msg`)
  - `LOG_SOURCE_KEY` — key name for source location (default: `source`)
- **Behavior**: Only affects JSON handler. Implemented via `ReplaceAttr`.
- **Use case**: ECS compatibility (`@timestamp`, `message`, `log.level`), GCP (`severity`, `message`)

---

## Non-Functional Requirements

### NFR-1: Backward Compatibility
- Without any new env vars set, behavior is identical to current (text format, info level)
- Existing `LOG_LEVEL=debug` continues to work

### NFR-2: Performance
- `ReplaceAttr` function should be minimal (switch on key, no allocations)
- No measurable performance impact for `text` format (no ReplaceAttr applied)

### NFR-3: SDK Integration
- `*slog.Logger` instance passed to `dephealth.WithLogger()` must use the same configured handler
- SDK logs follow the same format as application logs

### NFR-4: Testability
- Logger construction must be testable without environment variables
- Config struct should be the sole input to logger factory

---

## User Stories

### US-1: K8s Operator
> As a K8s operator, I want to set `LOG_FORMAT=json` so that my log aggregator (Loki/ELK/Fluentd) can parse structured log fields.

### US-2: Developer
> As a developer, I want text logs by default so that I can read them easily during local Docker development.

### US-3: ECS User
> As a user of Elastic Common Schema, I want to set `LOG_TIME_KEY=@timestamp LOG_MESSAGE_KEY=message LOG_LEVEL_KEY=log.level` so that my logs are ECS-compatible without a Logstash filter.

### US-4: Debugger
> As a developer debugging an issue, I want to set `LOG_ADD_SOURCE=yes` to see which file and line each log message comes from.

### US-5: Metrics Pipeline
> As a monitoring engineer, I want `LOG_TIME_FORMAT=unixmilli` so that my ClickHouse pipeline can use integer timestamps for efficient sorting.

---

## Acceptance Criteria

- [ ] `LOG_FORMAT=text` produces `slog.TextHandler` output (key=value pairs)
- [ ] `LOG_FORMAT=json` produces `slog.JSONHandler` output (JSON objects)
- [ ] `LOG_LEVEL` supports all four slog levels (debug, info, warn, error)
- [ ] `LOG_TIME_FORMAT` changes timestamp format in JSON output
- [ ] `LOG_ADD_SOURCE=yes` includes file:line in log output
- [ ] `LOG_TIME_KEY`, `LOG_LEVEL_KEY`, `LOG_MESSAGE_KEY`, `LOG_SOURCE_KEY` rename JSON keys
- [ ] Custom key names are ignored when `LOG_FORMAT=text`
- [ ] No env vars set = identical behavior to current (text, info level)
- [ ] All config tests pass
- [ ] Logger integration test with JSON output parsing

---

## Environment Variables Summary

| Variable | Values | Default | Tier |
|---|---|---|---|
| `LOG_LEVEL` | `debug`, `info`, `warn`, `error` | `info` | 1 |
| `LOG_FORMAT` | `text`, `json` | `text` | 1 |
| `LOG_TIME_FORMAT` | `rfc3339`, `rfc3339nano`, `unix`, `unixmilli` | `rfc3339nano` | 2 |
| `LOG_ADD_SOURCE` | `yes`/`no` | `no` | 2 |
| `LOG_TIME_KEY` | any string | `time` | 3 |
| `LOG_LEVEL_KEY` | any string | `level` | 3 |
| `LOG_MESSAGE_KEY` | any string | `msg` | 3 |
| `LOG_SOURCE_KEY` | any string | `source` | 3 |

---

## Open Questions

None — all clarified during brainstorm.

---

---

# Design: Configurable JSON Logging

## Architecture Overview

```
┌──────────────┐     ┌───────────────────┐     ┌──────────────────┐
│   main.go    │────▶│ internal/config    │     │ internal/logging │
│              │     │                    │     │                  │
│ cfg := Load()│     │ Config.Log LogConfig│     │ NewLogger(cfg)   │
│ logger :=    │────▶│                    │◀────│ → *slog.Logger   │
│  NewLogger() │     │ LogConfig{8 fields}│     │                  │
│ SetDefault() │     └───────────────────┘     └──────────────────┘
│ WithLogger() │              ▲                        │
└──────────────┘              │                        │
       │                 env vars (8)            slog.Handler
       ▼                                    ┌──────────┴──────────┐
  dephealth SDK                             │                     │
  (same logger)                       TextHandler           JSONHandler
                                      (Level,               (Level,
                                       AddSource)            AddSource,
                                                             ReplaceAttr)
                                                                  │
                                                        ┌─────────┴─────────┐
                                                        │ buildReplaceAttr  │
                                                        │  - timeFormat     │
                                                        │  - timeKey        │
                                                        │  - levelKey       │
                                                        │  - messageKey     │
                                                        │  - sourceKey      │
                                                        └───────────────────┘
```

## Component Design

### 1. `internal/config.LogConfig` (data struct)

```go
// LogConfig holds logging configuration parsed from environment variables.
type LogConfig struct {
    Format     string // "text" (default) or "json"
    Level      string // "debug", "info" (default), "warn", "error"
    TimeFormat string // "rfc3339", "rfc3339nano" (default), "unix", "unixmilli"
    AddSource  bool   // include file:line in log output (default: false)
    TimeKey    string // JSON key for timestamp (default: "time")
    LevelKey   string // JSON key for level (default: "level")
    MessageKey string // JSON key for message (default: "msg")
    SourceKey  string // JSON key for source (default: "source")
}
```

**Parsing in `Load()`:**
```go
cfg.Log = LogConfig{
    Format:     strings.ToLower(getEnv("LOG_FORMAT", "text")),
    Level:      strings.ToLower(getEnv("LOG_LEVEL", "info")),
    TimeFormat: strings.ToLower(getEnv("LOG_TIME_FORMAT", "rfc3339nano")),
    TimeKey:    getEnv("LOG_TIME_KEY", ""),    // empty = slog default
    LevelKey:   getEnv("LOG_LEVEL_KEY", ""),
    MessageKey: getEnv("LOG_MESSAGE_KEY", ""),
    SourceKey:  getEnv("LOG_SOURCE_KEY", ""),
}
// AddSource via parseBool
if v := os.Getenv("LOG_ADD_SOURCE"); v != "" {
    cfg.Log.AddSource, err = parseBool(v)
    ...
}
```

**Validation:**
- `Format`: only "text" or "json", error otherwise
- `Level`: via `slog.Level.UnmarshalText()` — invalid → error
- `TimeFormat`: only "rfc3339", "rfc3339nano", "unix", "unixmilli" — invalid → error
- Key names: no validation (any string is valid)

**Breaking change in `Config`:**
- Remove `LogLevel string` field
- Add `Log LogConfig` field
- `main.go` uses `cfg.Log.Level` instead of `cfg.LogLevel`

### 2. `internal/logging` (new package)

**File: `internal/logging/logging.go`**

```go
package logging

import (
    "log/slog"
    "os"
    "strings"
    "time"

    "github.com/BigKAA/uniproxy/internal/config"
)

// NewLogger creates a configured *slog.Logger from LogConfig.
// Output is always os.Stderr.
func NewLogger(cfg config.LogConfig) *slog.Logger {
    level := parseLevel(cfg.Level)

    opts := &slog.HandlerOptions{
        Level:     level,
        AddSource: cfg.AddSource,
    }

    var handler slog.Handler
    switch cfg.Format {
    case "json":
        opts.ReplaceAttr = buildReplaceAttr(cfg)
        handler = slog.NewJSONHandler(os.Stderr, opts)
    default: // "text"
        handler = slog.NewTextHandler(os.Stderr, opts)
    }

    return slog.New(handler)
}

// parseLevel converts string to slog.Level. Defaults to Info.
func parseLevel(s string) slog.Level {
    var level slog.Level
    if err := level.UnmarshalText([]byte(strings.ToUpper(s))); err != nil {
        return slog.LevelInfo
    }
    return level
}

// buildReplaceAttr creates a ReplaceAttr function for JSON handler
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
```

**Key design decisions:**
- `parseLevel` is lenient (falls back to Info) — logging should never block startup
- `buildReplaceAttr` only runs for JSON format — text handler gets no ReplaceAttr
- Empty key strings (default) = no rename (slog defaults preserved)
- Output hardcoded to `os.Stderr` — no configurability needed (12-factor standard)

### 3. Changes to `main.go`

**Before (lines 30-36):**
```go
level := slog.LevelInfo
if cfg.LogLevel == "debug" {
    level = slog.LevelDebug
}
logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
slog.SetDefault(logger)
```

**After:**
```go
logger := logging.NewLogger(cfg.Log)
slog.SetDefault(logger)
```

5 lines → 2 lines. Import `"github.com/BigKAA/uniproxy/internal/logging"`.

### 4. Changes to `internal/config/config.go`

**Config struct:**
```go
// Before:
type Config struct {
    Name          string
    ListenAddr    string
    LogLevel      string         // ← remove
    CheckInterval time.Duration
    ...
}

// After:
type Config struct {
    Name          string
    ListenAddr    string
    Log           LogConfig      // ← replace
    CheckInterval time.Duration
    ...
}
```

**Load() function:**
```go
// Before:
cfg := &Config{
    ListenAddr: getEnv("LISTEN_ADDR", ":8080"),
    LogLevel:   getEnv("LOG_LEVEL", "info"),
}

// After:
cfg := &Config{
    ListenAddr: getEnv("LISTEN_ADDR", ":8080"),
}
// Parse log config
cfg.Log, err = loadLogConfig()
if err != nil {
    return nil, err
}
```

**New helper:**
```go
func loadLogConfig() (LogConfig, error) {
    lc := LogConfig{
        Format:     strings.ToLower(getEnv("LOG_FORMAT", "text")),
        Level:      strings.ToLower(getEnv("LOG_LEVEL", "info")),
        TimeFormat: strings.ToLower(getEnv("LOG_TIME_FORMAT", "rfc3339nano")),
        TimeKey:    os.Getenv("LOG_TIME_KEY"),
        LevelKey:   os.Getenv("LOG_LEVEL_KEY"),
        MessageKey: os.Getenv("LOG_MESSAGE_KEY"),
        SourceKey:  os.Getenv("LOG_SOURCE_KEY"),
    }

    // Validate format
    switch lc.Format {
    case "text", "json":
    default:
        return lc, fmt.Errorf("invalid LOG_FORMAT %q (expected text/json)", lc.Format)
    }

    // Validate level
    var level slog.Level
    if err := level.UnmarshalText([]byte(strings.ToUpper(lc.Level))); err != nil {
        return lc, fmt.Errorf("invalid LOG_LEVEL %q (expected debug/info/warn/error)", lc.Level)
    }

    // Validate time format
    switch lc.TimeFormat {
    case "rfc3339", "rfc3339nano", "unix", "unixmilli":
    default:
        return lc, fmt.Errorf("invalid LOG_TIME_FORMAT %q (expected rfc3339/rfc3339nano/unix/unixmilli)", lc.TimeFormat)
    }

    // AddSource (optional)
    if v := os.Getenv("LOG_ADD_SOURCE"); v != "" {
        b, err := parseBool(v)
        if err != nil {
            return lc, fmt.Errorf("invalid LOG_ADD_SOURCE: %w", err)
        }
        lc.AddSource = b
    }

    return lc, nil
}
```

## Test Design

### `internal/logging/logging_test.go`

| Test | What it verifies |
|---|---|
| `TestNewLogger_DefaultConfig` | Empty config → text handler, info level |
| `TestNewLogger_JSONFormat` | Format="json" → output is valid JSON |
| `TestNewLogger_TextFormat` | Format="text" → output is key=value |
| `TestNewLogger_LevelFiltering` | Level="warn" filters out info/debug |
| `TestNewLogger_JSONTimeFormat_RFC3339` | TimeFormat="rfc3339" → no nanoseconds |
| `TestNewLogger_JSONTimeFormat_Unix` | TimeFormat="unix" → integer seconds |
| `TestNewLogger_JSONTimeFormat_UnixMilli` | TimeFormat="unixmilli" → integer ms |
| `TestNewLogger_JSONCustomKeys` | Custom key names appear in JSON output |
| `TestNewLogger_AddSource` | AddSource=true → "source" field present |
| `TestNewLogger_CustomKeys_IgnoredForText` | Text format ignores key renames |

**Test pattern** (capture slog output):
```go
var buf bytes.Buffer
cfg := config.LogConfig{Format: "json", Level: "info"}
// Construct handler manually pointing to buf instead of os.Stderr
opts := &slog.HandlerOptions{Level: parseLevel(cfg.Level)}
handler := slog.NewJSONHandler(&buf, opts)
logger := slog.New(handler)
logger.Info("test message", "key", "value")
// Assert buf.String() contains expected JSON
```

### `internal/config/config_test.go` (additions)

| Test | What it verifies |
|---|---|
| `TestLoad_LogConfig_Defaults` | No LOG_* vars → defaults (text, info, rfc3339nano, no source, empty keys) |
| `TestLoad_LogConfig_AllVars` | All 8 vars set → parsed correctly |
| `TestLoad_LogConfig_InvalidFormat` | LOG_FORMAT=xml → error |
| `TestLoad_LogConfig_InvalidLevel` | LOG_LEVEL=trace → error |
| `TestLoad_LogConfig_InvalidTimeFormat` | LOG_TIME_FORMAT=iso8601 → error |
| `TestLoad_LogConfig_InvalidAddSource` | LOG_ADD_SOURCE=maybe → error |
| `TestLoad_LogConfig_BackwardCompat` | LOG_LEVEL=debug (existing usage) → works |

## File Change Summary

| File | Action | Description |
|---|---|---|
| `internal/config/config.go` | Modify | Add `LogConfig` struct, replace `LogLevel` with `Log`, add `loadLogConfig()` |
| `internal/config/config_test.go` | Modify | Add 7 tests for LogConfig parsing |
| `internal/logging/logging.go` | Create | `NewLogger()`, `parseLevel()`, `buildReplaceAttr()` |
| `internal/logging/logging_test.go` | Create | 10 tests for logger construction |
| `main.go` | Modify | Replace inline logger setup with `logging.NewLogger()`, add import |

## Next Steps

Create implementation plan using template, then `/sc:implement`.
