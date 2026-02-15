# Plan: Configurable JSON Logging

## Metadata

- **Version**: 1.0.0
- **Created**: 2026-02-15
- **Last updated**: 2026-02-15
- **Status**: Pending

---

## Version History

- **v1.0.0** (2026-02-15): Initial plan based on requirements and design spec

---

## Current Status

- **Active phase**: Phase 1
- **Active subtask**: 1.1
- **Last updated**: 2026-02-15
- **Note**: Ready to implement. Design in `plans/configurable_logging.md`.

---

## Table of Contents

- [ ] [Phase 1: Config and logging package](#phase-1-config-and-logging-package)
- [ ] [Phase 2: Integration and tests](#phase-2-integration-and-tests)
- [ ] [Phase 3: Build and verify](#phase-3-build-and-verify)

---

## Context

### Problem

uniproxy hardcodes text-format logging (`slog.NewTextHandler`). In Kubernetes and other production environments, JSON structured logging is required for log aggregators (Loki, ELK, Fluentd). Different aggregators expect different key names and time formats.

### Solution

Add 8 environment variables for configurable logging: format (text/json), level, time format, source location, and custom JSON key names. Implemented via standard `slog.HandlerOptions.ReplaceAttr`.

### Key decisions

| Decision | Choice |
|---|---|
| Config struct location | `internal/config.LogConfig` (data-only, no imports) |
| Logger factory location | `internal/logging.NewLogger(config.LogConfig)` |
| Dependency direction | `logging` imports `config` (not reverse) |
| Output destination | Always `os.Stderr` (not configurable, 12-factor) |
| Validation | Strict in `config.Load()`, lenient in `logging.NewLogger()` |
| ReplaceAttr scope | JSON handler only (text handler untouched) |
| Default format | `text` (backward compatible) |

### Design reference

Full design with code samples: `plans/configurable_logging.md` (Design section)

---

## Phase 1: Config and logging package

**Dependencies**: None
**Status**: Pending

### Description

Create the `LogConfig` struct and `loadLogConfig()` parser in the config package. Create the new `internal/logging` package with `NewLogger()` factory. This phase produces all new code but doesn't wire it into `main.go` yet.

### Subtasks

- [ ] **1.1 Add LogConfig struct and loadLogConfig() to config package**
  - **Dependencies**: None
  - **Description**:
    1. Add `LogConfig` struct with 8 fields to `internal/config/config.go`
    2. Replace `LogLevel string` field in `Config` with `Log LogConfig`
    3. Add `loadLogConfig()` function that parses 8 env vars with validation:
       - `LOG_FORMAT`: "text"/"json" (default: "text")
       - `LOG_LEVEL`: "debug"/"info"/"warn"/"error" via `slog.Level.UnmarshalText()` (default: "info")
       - `LOG_TIME_FORMAT`: "rfc3339"/"rfc3339nano"/"unix"/"unixmilli" (default: "rfc3339nano")
       - `LOG_ADD_SOURCE`: via `parseBool()` (default: false)
       - `LOG_TIME_KEY`, `LOG_LEVEL_KEY`, `LOG_MESSAGE_KEY`, `LOG_SOURCE_KEY`: any string (default: "")
    4. Call `loadLogConfig()` in `Load()`, remove old `LogLevel` initialization
    5. Add import `"log/slog"` for level validation
  - **Modifies**:
    - `internal/config/config.go`

- [ ] **1.2 Create internal/logging package**
  - **Dependencies**: 1.1
  - **Description**: Create `internal/logging/logging.go` with:
    1. `NewLogger(cfg config.LogConfig) *slog.Logger` — factory function
    2. `parseLevel(s string) slog.Level` — unexported, lenient (falls back to Info)
    3. `buildReplaceAttr(cfg config.LogConfig) func([]string, slog.Attr) slog.Attr` — unexported
    - For `cfg.Format == "json"`: JSONHandler with ReplaceAttr for time format + key renames
    - For `cfg.Format == "text"` (or default): TextHandler, no ReplaceAttr
    - Output always `os.Stderr`
    - See design spec for full code
  - **Creates**:
    - `internal/logging/logging.go`

- [ ] **1.3 Add config tests for LogConfig**
  - **Dependencies**: 1.1
  - **Description**: Add tests to `internal/config/config_test.go`:
    - `TestLoad_LogConfig_Defaults` — no LOG_* vars → text, info, rfc3339nano, false, empty keys
    - `TestLoad_LogConfig_AllVars` — all 8 vars set → parsed correctly
    - `TestLoad_LogConfig_InvalidFormat` — LOG_FORMAT=xml → error
    - `TestLoad_LogConfig_InvalidLevel` — LOG_LEVEL=trace → error
    - `TestLoad_LogConfig_InvalidTimeFormat` — LOG_TIME_FORMAT=iso8601 → error
    - `TestLoad_LogConfig_InvalidAddSource` — LOG_ADD_SOURCE=maybe → error
    - `TestLoad_LogConfig_BackwardCompat` — LOG_LEVEL=debug → works (existing usage)
  - **Modifies**:
    - `internal/config/config_test.go`

- [ ] **1.4 Add logging tests**
  - **Dependencies**: 1.2
  - **Description**: Create `internal/logging/logging_test.go`:
    - Use `NewTestLogger(cfg, &buf)` helper that writes to `bytes.Buffer` instead of stderr
    - Or export a `NewLoggerWithWriter(cfg, w)` for testing (preferred — avoids test-only code)

    Actually, better approach: make `NewLogger` accept `io.Writer` as parameter, and main.go passes `os.Stderr`. This is cleaner and more testable.

    Wait — this changes the public API. Alternative: export `NewHandler(cfg) slog.Handler` that returns the handler, and `NewLogger` wraps it with `slog.New`. Tests can then use `NewHandler` with a custom writer... but handlers are bound to writers at creation.

    Simplest approach: add unexported `newHandler(cfg, w io.Writer) slog.Handler` and have `NewLogger` call it with `os.Stderr`. Tests use `newHandler` directly.

    Tests:
    - `TestNewHandler_DefaultConfig` — empty LogConfig → text output, info level
    - `TestNewHandler_JSONFormat` — Format="json" → valid JSON line
    - `TestNewHandler_TextFormat` — Format="text" → key=value format
    - `TestNewHandler_LevelFiltering` — Level="warn" → info message not written
    - `TestNewHandler_JSONTimeFormat_RFC3339` — verify no nanoseconds
    - `TestNewHandler_JSONTimeFormat_Unix` — verify integer timestamp
    - `TestNewHandler_JSONTimeFormat_UnixMilli` — verify integer ms timestamp
    - `TestNewHandler_JSONCustomKeys` — verify renamed keys in JSON
    - `TestNewHandler_AddSource` — verify source field present
    - `TestNewHandler_CustomKeys_TextIgnored` — text format ignores key renames
  - **Creates**:
    - `internal/logging/logging_test.go`

### Completion criteria

- [ ] All subtasks completed (1.1, 1.2, 1.3, 1.4)
- [ ] `go build ./...` compiles without errors
- [ ] `go test ./internal/config/...` passes (including new tests)
- [ ] `go test ./internal/logging/...` passes
- [ ] LogConfig defaults match backward-compatible behavior

---

## Phase 2: Integration and tests

**Dependencies**: Phase 1
**Status**: Pending

### Description

Wire the new logging package into `main.go`, replacing the inline logger setup. Update existing tests if any reference `cfg.LogLevel`. Run full test suite.

### Subtasks

- [ ] **2.1 Update main.go to use logging.NewLogger()**
  - **Dependencies**: Phase 1
  - **Description**:
    1. Replace lines 30-36 (inline logger setup) with:
       ```go
       logger := logging.NewLogger(cfg.Log)
       slog.SetDefault(logger)
       ```
    2. Add import `"github.com/BigKAA/uniproxy/internal/logging"`
    3. Remove unused imports: `"log/slog"` handler-related if no longer needed
    4. Keep `slog.Error/Info` calls unchanged (they use default logger)
    5. Pass `logger` to `buildOptions` → `dephealth.WithLogger(logger)` (already works)
    6. Update `cfg.LogLevel` reference in the log output line (line 38) if it exists
  - **Modifies**:
    - `main.go`

- [ ] **2.2 Fix existing config tests**
  - **Dependencies**: 2.1
  - **Description**: Update any existing tests that reference `cfg.LogLevel` to use `cfg.Log.Level`. Search for `LogLevel` in test files and update.
  - **Modifies**:
    - `internal/config/config_test.go` (if needed)

- [ ] **2.3 Run full test suite**
  - **Dependencies**: 2.1, 2.2
  - **Description**: Run `go test ./...` and verify all tests pass. No regressions.
  - **Creates**:
    - Test results

### Completion criteria

- [ ] All subtasks completed (2.1, 2.2, 2.3)
- [ ] `main.go` uses `logging.NewLogger(cfg.Log)` instead of inline setup
- [ ] `go test ./...` — all tests pass
- [ ] No references to `cfg.LogLevel` remain in codebase

---

## Phase 3: Build and verify

**Dependencies**: Phase 2
**Status**: Pending

### Description

Build Docker image and verify logging works correctly in both text and JSON modes.

### Subtasks

- [ ] **3.1 Build Docker image**
  - **Dependencies**: None
  - **Description**: Build `uniproxy:dev` image. Verify it starts and logs in default (text) format.
  - **Creates**:
    - Docker image

- [ ] **3.2 Verify JSON logging**
  - **Dependencies**: 3.1
  - **Description**: Run container with `LOG_FORMAT=json` and verify:
    1. Output is valid JSON
    2. Default keys: `time`, `level`, `msg`
    3. With `LOG_TIME_FORMAT=rfc3339` — no nanoseconds in time
    4. With `LOG_TIME_KEY=@timestamp LOG_MESSAGE_KEY=message` — custom keys appear
    5. With `LOG_LEVEL=debug` — debug messages visible
    6. With `LOG_ADD_SOURCE=yes` — source info present
  - **Creates**:
    - Verification results

- [ ] **3.3 Verify backward compatibility**
  - **Dependencies**: 3.1
  - **Description**: Run container WITHOUT any new env vars. Verify:
    1. Text format output (same as before)
    2. `LOG_LEVEL=debug` still works
    3. All endpoints respond correctly (`/`, `/?detail=true`, `/healthz`, `/metrics`)
  - **Creates**:
    - Verification results

### Completion criteria

- [ ] All subtasks completed (3.1, 3.2, 3.3)
- [ ] Docker image builds successfully
- [ ] JSON logging works with all configuration options
- [ ] Backward compatibility confirmed — no behavior change without new env vars
- [ ] All endpoints unaffected by logging changes

---

## Implementation Workflow

```
Phase 1 ─── 1.1 LogConfig + loadLogConfig() ──┐
         ├── 1.2 internal/logging package ─────┤
         ├── 1.3 Config tests ─────────────────┤▶ go test ./internal/...
         └── 1.4 Logging tests ────────────────┘
                    │
Phase 2 ─── 2.1 Wire main.go ─────────────────┐
         ├── 2.2 Fix existing tests ───────────┤▶ go test ./...
         └── 2.3 Full test suite ──────────────┘
                    │
Phase 3 ─── 3.1 Docker build ─────────────────┐
         ├── 3.2 Verify JSON logging ──────────┤▶ Manual verification
         └── 3.3 Verify backward compat ───────┘
```

## Notes

- `internal/logging` imports `internal/config` — this is the only new cross-package dependency
- `ReplaceAttr` performance: one switch statement per log entry per built-in key — negligible overhead
- `slog.Level.UnmarshalText()` supports "DEBUG", "INFO", "WARN", "ERROR" plus offsets like "INFO+2"
- Validation is strict in config (fail fast at startup), lenient in logging (never block)
- Design allows future extension: custom levels, log rotation, file output — without changing the interface
