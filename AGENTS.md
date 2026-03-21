# AGENTS.md — Guide for AI Coding Agents

This file provides guidance for AI agents working on the uniproxy codebase.

## Project Overview

**uniproxy** is a Go application that health-checks dependencies using the [dephealth SDK](https://github.com/BigKAA/topologymetrics) (`github.com/BigKAA/topologymetrics/sdk-go/dephealth` v0.8.2) and exposes Prometheus metrics. Go 1.25+.

## Communication Requirements

- **Russian language** for all communication and discussion
- **English** for all code, comments, and documentation
- Ask the user if uncertain rather than making assumptions

## Build, Test, and Quality Commands

All commands run via Docker (no local tool installation required):

```bash
# Pull required Docker images (first time)
make pull

# Build (compilation)
make build

# Run all tests with race detection
make test

# Run tests with coverage
make test-coverage

# Run a single test
go test -v ./internal/config -run TestLoad

# Static analysis (golangci-lint)
make lint

# Security check (gosec only)
make security

# Dependency vulnerability scan (govulncheck)
make audit

# Dead code detection
make deadcode

# Code formatting (goimports + gofmt)
make fmt

# Helm chart validation
make helm-lint

# Dockerfile best practices (hadolint)
make hadolint

# Run ALL checks
make check-all
```

## Code Style Guidelines

### General Principles

- **Language**: Code and comments in English
- **Discussion**: Russian language
- **Documentation**: Package-level and public function docs required
- **Errors**: Always handle errors explicitly; never ignore with `_`

### Imports

- Use `goimports` for import organization (local imports grouped first, then stdlib, then external)
- Local prefix: `github.com/BigKAA/uniproxy`
- Format: `goimports -w -local github.com/BigKAA/uniproxy .`

### Naming Conventions

- **Variables**: `camelCase` (e.g., `depName`, `checkInterval`)
- **Constants**: `PascalCase` for exported, `camelCase` for unexported (e.g., `DefaultPort`, `defaultTimeout`)
- **Packages**: `snake_case` (e.g., `internal/auth`, `internal/config`)
- **Functions**: `PascalCase` for exported, `camelCase` for unexported
- **Interfaces**: `PascalCase` with `er` suffix (e.g., `HealthChecker`)

### Types

- Use pointer types (`*bool`, `*string`) when "not set" is meaningful (nil = default)
- Prefer `time.Duration` over raw integers for time values
- Use concrete types over interfaces unless polymorphism needed

### Error Handling

- Return errors with context: `fmt.Errorf("failed to %s: %w", action, originalErr)`
- Wrap errors at call site, not in helper functions
- Never use bare `panic()` except for unrecoverable initialization failures

### Formatting

- Run `make fmt` before committing
- Maximum line length: moderate (let gofmt handle it)
- Group related const declarations together
- Use meaningful variable names; avoid single letters except in short loops

### Testing

- Test files: `*_test.go` in same package
- Test functions: `Test<FunctionName>`
- Table-driven tests preferred for multiple cases
- Use `t.Helper()` for test helpers
- Include race detection: `go test -race`

## Architecture

```
main.go → internal/config → dephealth SDK → internal/server
                    ↓
            internal/logging
                    ↓
            internal/auth
```

- **main.go**: Entry point, SDK init, server start
- **internal/config**: Env vars + YAML parsing, validation
- **internal/logging**: Structured logging setup (slog)
- **internal/auth**: Zone-based auth middleware
- **internal/server**: HTTP handlers, recursive fetch
- **dephealth SDK**: Health checking logic, Prometheus metrics

## Common Patterns

### Config Loading

```go
cfg, err := config.Load()  // loads from YAML + env vars
if err != nil {
    slog.Error("failed to load config", "error", err)
    os.Exit(1)
}
```

### Adding a New Dependency Type

1. Add type to `config.Dependency` struct
2. Add env var parsing in `parseSingleDep()`
3. Add SDK option building in `main.go` `buildDependencyOption()`
4. Add validation in `validateAuth()` if needed

### Adding a New Config Option

1. Add field to `Config` struct in `config.go`
2. Add env var parsing in `applyEnvOverrides()`
3. Add default in `applyDefaults()`
4. Add validation in `validate()`

## Linters Enabled

From `.golangci.yml`: errcheck, govet, staticcheck, unused, ineffassign, misspell, unconvert, unparam, prealloc, gosec

## Important Files

- `go.mod`: Module definition (Go 1.25.8)
- `Makefile`: All build/lint/test commands
- `.golangci.yml`: Linter configuration
- `charts/uniproxy/`: Helm chart
- `deploy/helm/uniproxy/`: Legacy multi-instance Helm chart

## Git Workflow

- Follow Conventional Commits: `<type>(<scope>): <subject>`
- Types: `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`
- Branch naming: `feature/<description>`, `bugfix/<description>`, etc.
- Never commit directly to main; use PR or `--no-ff` merge

## Important Constraints

1. **No local execution**: Always use Docker or Kubernetes
2. **12-factor app**: All runtime config via environment variables
3. **SDK responsibility**: dephealth SDK handles health checking logic
4. **Instance-based model**: Multiple independent instances supported
