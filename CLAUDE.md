# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**uniproxy** is a universal test proxy for dependency health monitoring. It uses the [dephealth SDK](https://github.com/BigKAA/topologymetrics) to health-check configured dependencies (HTTP, gRPC, PostgreSQL, Redis, etc.) and expose Prometheus metrics. Primary use case: testing dephealth-ui topology visualization in Kubernetes environments.

## Communication Requirements

- **Russian language** for all communication and discussion
- **English** for all code, comments, and documentation
- Ask the user if uncertain rather than making assumptions

## Development Environment

**CRITICAL**: All development, debugging, and testing MUST be done using Docker containers or Kubernetes.

**CRITICAL**: Docker images must be built as multi-platform for `linux/amd64` and `linux/arm64` using `docker buildx build --platform linux/amd64,linux/arm64`.

### Available Tools
- `kubectl` — configured for test Kubernetes cluster
- `helm` — for Helm chart operations
- `docker` — container operations

### Test Kubernetes Cluster
- Gateway API installed (no Ingress controller)
- MetalLB enabled (LoadBalancer services supported)
- cert-manager with ClusterIssuer: `dev-ca-issuer`
- Test domains: `test1.kryukov.lan`, `test2.kryukov.lan`, `test3.kryukov.lan` → `192.168.218.180` (Gateway API)
- Local DNS: `192.168.218.9`

### Container Registries

**Release registry (Yandex Container Registry):**
- `container-registry.cloud.yandex.net/crpklna5l8v5m7c0ipst` — release images
- Authentication via `yc` credential helper (configured in `~/.docker/config.json`)
- Use for all documentation examples and Helm chart defaults

**Development registry (Harbor):**
- `harbor.kryukov.lan/library` — dev/test images (homelab only)
- Docker Hub proxy: `harbor.kryukov.lan/docker`
- MCR proxy: `harbor.kryukov.lan/mcr`
- Admin credentials: `admin` / `password`

When adding test domains, ask the user to manually add them to `/etc/hosts`.

## Common Commands

### Build & Run Locally (Docker)

```bash
# Build Docker image
docker build -t uniproxy:dev .

# Run with test configuration
docker run -p 8080:8080 -p 9090:9090 \
  -e DEPHEALTH_NAME=test-proxy \
  -e DEPHEALTH_GROUP=test \
  -e DEPHEALTH_DEPS="httpbin:http" \
  -e DEPHEALTH_HTTPBIN_URL="http://httpbin.org" \
  -e DEPHEALTH_HTTPBIN_CRITICAL="yes" \
  uniproxy:dev

# Check health endpoint
curl http://localhost:8080/
curl http://localhost:8080/healthz

# Check Prometheus metrics
curl http://localhost:9090/metrics | grep app_dependency
```

### Testing

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run specific test
go test -v ./internal/config -run TestLoad

# Run tests in verbose mode
go test -v ./...
```

### Kubernetes Deployment (Helm)

```bash
# Install instance
helm install uniproxy-01 ./deploy/helm/uniproxy \
  -f ./deploy/helm/uniproxy/instances/ns1-homelab.yaml \
  -n uniproxy-ns1 --create-namespace

# Upgrade instance
helm upgrade uniproxy-01 ./deploy/helm/uniproxy \
  -f ./deploy/helm/uniproxy/instances/ns1-homelab.yaml \
  -n uniproxy-ns1

# Uninstall instance
helm uninstall uniproxy-01 -n uniproxy-ns1

# Debug template rendering
helm template uniproxy-01 ./deploy/helm/uniproxy \
  -f ./deploy/helm/uniproxy/instances/ns1-homelab.yaml

# Check deployed resources
kubectl get all -n uniproxy-ns1
kubectl logs -n uniproxy-ns1 deployment/uniproxy-01
```

### Linting & Code Quality

All quality checks run via Docker (no local tool installation required). See `Makefile` for details.

```bash
# Pull all required Docker images (first time)
make pull

# Static analysis (golangci-lint with .golangci.yml config)
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

# Run ALL checks (lint + test + audit + deadcode + helm-lint + hadolint)
make check-all

# Build / test
make build
make test
make test-coverage

# Cleanup Docker volume caches
make clean

# Show all available targets
make help
```

## Architecture & Code Structure

### High-Level Architecture

```
┌─────────────┐
│   main.go   │  Entry point
└──────┬──────┘
       │
       ├─> internal/config      Parse env vars + YAML config (CONFIG_FILE)
       │                        Build Config struct with dependencies, auth, logging
       │
       ├─> internal/logging     Structured logging setup (text/json, configurable keys)
       │
       ├─> internal/auth        Server-side auth middleware (Basic/Bearer/APIKey)
       │                        Zone-based: status (/), metrics (/metrics)
       │
       ├─> dephealth SDK        Initialize health checker with parsed dependencies
       │   (external)           Start background health checks (interval-based)
       │                        Register Prometheus metrics (4 metrics)
       │
       └─> internal/server      HTTP server (Chi router)
                                Endpoints: /, /healthz, /readyz, /metrics
                                Detail mode: /?detail=true&depth=N (recursive HTTP fetch)
                                Expose SDK metrics via /metrics
```

### Key Components

**main.go**
- Loads configuration from environment variables / YAML via `internal/config`
- Initializes structured logging via `internal/logging`
- Initializes dephealth SDK with dependency configurations
- Starts background health checks (SDK handles periodic checking)
- Starts HTTP server with Chi router and auth middleware
- Handles graceful shutdown on SIGINT/SIGTERM

**internal/auth/**
- Server-side authentication middleware (Basic Auth, Bearer token, API Key)
- Zone-based: independent auth settings for status (`/`) and metrics (`/metrics`) endpoints
- Health probes (`/healthz`, `/readyz`) always open

**internal/logging/**
- Structured logging setup with `slog`
- Supports text and JSON output formats
- Configurable JSON keys (`LOG_TIME_KEY`, `LOG_LEVEL_KEY`, `LOG_MESSAGE_KEY`, `LOG_SOURCE_KEY`)

**internal/config/config.go**
- Parses environment variables and optional YAML config (`CONFIG_FILE`) into `Config` struct
- Required vars: `DEPHEALTH_NAME`, `DEPHEALTH_GROUP`
- Optional: `LISTEN_ADDR`, `LOG_FORMAT`, `LOG_LEVEL`, `LOG_TIME_FORMAT`, `LOG_ADD_SOURCE`, `LOG_*_KEY`, `DEPHEALTH_CHECK_INTERVAL`, `DEPHEALTH_TIMEOUT`, `DEPHEALTH_FETCH_TIMEOUT`, `DEPHEALTH_ISENTRY`
- Per-dependency vars: `DEPHEALTH_<NAME>_URL` or `DEPHEALTH_<NAME>_HOST` + `DEPHEALTH_<NAME>_PORT`
- Server auth: `AUTH_METHOD`, `AUTH_USER`, `AUTH_PASS`, `AUTH_TOKEN`, `AUTH_API_KEY` + per-zone overrides (`AUTH_STATUS_*`, `AUTH_METRICS_*`)
- Dependency auth: `DEPHEALTH_BEARER_TOKEN`, `DEPHEALTH_BASIC_USER/PASS`, `DEPHEALTH_HEADERS`, `DEPHEALTH_METADATA` (global and per-dependency)
- `_FILE` suffix pattern for secrets (e.g., `DEPHEALTH_BEARER_TOKEN_FILE`)
- Dependency types: `http`, `redis`, `postgres`, `grpc`, `tcp`, `mysql`, `amqp`, `kafka`, `ldap`
- YAML config priority: env vars always override YAML values; `DEPHEALTH_DEPS` replaces all YAML dependencies

**internal/server/server.go**
- HTTP server using `go-chi/chi` router with zone-based auth middleware
- Routes:
  - `GET /` — JSON status (name, podName, namespace, health map). Supports `?detail=true` for enriched response with `HealthDetails()` and `?depth=N` for recursive HTTP chain fetch (0–10, default 1). Auth zone: `status`
  - `GET /healthz` — Liveness probe (always returns 200 OK, no auth)
  - `GET /readyz` — Readiness probe (always returns 200 OK, no auth)
  - `GET /metrics` — Prometheus metrics via `promhttp.Handler()`. Auth zone: `metrics`

**External Dependency: dephealth SDK**
- `github.com/BigKAA/topologymetrics/sdk-go/dephealth` v0.8.0
- Handles all health checking logic (HTTP, gRPC, database connections, etc.)
- Exports 4 Prometheus metrics:
  - `app_dependency_health` (gauge: 1=healthy, 0=unhealthy)
  - `app_dependency_latency_seconds` (histogram, buckets: 1ms–5s)
  - `app_dependency_status` (gauge, enum pattern: one status value set to 1, rest to 0; label `status` with values: ok, timeout, connection_error, dns_error, auth_error, tls_error, unhealthy, error)
  - `app_dependency_status_detail` (gauge, info pattern: always 1 with `detail` label containing human-readable reason)
- Base labels (all metrics): `name`, `group`, `dependency`, `type`, `host`, `port`, `critical`
- Custom labels supported via `WithLabel()` / `DEPHEALTH_<DEP>_LABEL_<KEY>` env vars
- Built-in checker factories registered via `_ "github.com/BigKAA/topologymetrics/sdk-go/dephealth/checks"`

### Configuration Model

uniproxy uses a **declarative configuration** approach with two methods:
1. **Environment variables** (always works) or **YAML config file** (`CONFIG_FILE`), env vars always override YAML
2. `internal/config` parses these into structured `Config` (dependencies, auth, logging, etc.)
3. `main.go` builds `dephealth.Option` slice from config
4. SDK creates health checkers for each dependency
5. SDK runs checks in background and updates Prometheus metrics (4 metrics)

Optional global `isentry` label (`DEPHEALTH_ISENTRY=yes`) marks entry-point applications in topology visualization by adding `isentry=yes` to all dependency metrics.

Server-side authentication protects status (`/`) and metrics (`/metrics`) endpoints independently via zone-based auth config. Dependency-side authentication supports bearer tokens, basic auth, custom headers/metadata for HTTP/gRPC dependencies.

This allows **instance-based deployment**: multiple uniproxy instances with different dependency configs can run in the same namespace.

### Deployment Model (Helm)

Two Helm charts available:
- **Standard chart** (`charts/uniproxy/`): Single-instance per release, full support for Service types, Ingress, Gateway API, server auth, YAML config via ConfigMap
- **Legacy chart** (`deploy/helm/uniproxy/`): Multi-instance per release, instance-specific config in `instances/<name>.yaml`

Both support instance-based deployment with different dependency configurations per namespace.

## Development Plans

**MANDATORY**: Use standardized development plan template from `.templates/DEVELOPMENT_PLAN_TEMPLATE.md`

- Template: `.templates/DEVELOPMENT_PLAN_TEMPLATE.md` — base template with instructions
- Example: `.templates/DEVELOPMENT_PLAN_EXAMPLE.md` — filled example
- Guide: `.templates/TEMPLATE_GUIDE.md` — detailed usage guide
- Plans storage: `plans/` directory

### Plan Requirements
- Use semver versioning for plan changes
- Update "Текущий статус" section when switching tasks
- Mark checkboxes as work progresses
- Each phase must fit in AI context window
- Include explicit dependencies between phases and subtasks

### Workflow
```bash
cp .templates/DEVELOPMENT_PLAN_TEMPLATE.md plans/feature_name.md
# Fill plan, then start development
```

## Git Workflow

**MANDATORY**: Follow [GIT-WORKFLOW.md](./GIT-WORKFLOW.md) for all code changes.

### Branch Naming
- `feature/<description>` — new functionality
- `bugfix/<description>` — bug fixes
- `docs/<description>` — documentation
- `refactor/<description>` — refactoring
- `test/<description>` — tests
- `hotfix/<description>` — critical production fixes

### Commit Message Format (Conventional Commits)
```
<type>(<scope>): <subject>

[optional body]
```

Types: `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`

### Workflow Steps
1. Create feature branch from `main`
2. Make changes and commit
3. Merge to `main` with `--no-ff` or via PR
4. Delete feature branch
5. Tag releases with `vX.Y.Z` (semver)
6. Build and push release image:
   `docker buildx build --platform linux/amd64,linux/arm64 -t container-registry.cloud.yandex.net/crpklna5l8v5m7c0ipst/uniproxy:vX.Y.Z --push .`
7. Create GitHub release with `gh release create` — include a **Docker Image** section with pull command:
   `docker pull container-registry.cloud.yandex.net/crpklna5l8v5m7c0ipst/uniproxy:vX.Y.Z`
   and supported platforms: `linux/amd64`, `linux/arm64`

### Quick Fixes
Small fixes (typos, minor tweaks) can be committed directly to `main`.

## Related Projects

- **dephealth SDK**: `~/Projects/personal/topologymetrics/topologymetrics` — local SDK development
- **dephealth-ui**: Web UI for topology visualization (external)

When working on dephealth SDK integration, the SDK source is available locally for reference.

## Important Constraints

1. **No local execution**: Always use Docker or Kubernetes for running/testing
2. **Env var configuration**: All runtime config via environment variables (12-factor app)
3. **Health check responsibility**: SDK handles all health checking logic; uniproxy only configures and exposes metrics
4. **Instance-based model**: Designed for multiple independent instances with different configs
5. **Conventional Commits**: Strictly follow commit message format
6. **Russian/English split**: Communication in Russian, code/docs in English

## Debugging Tips

### Check Configuration Parsing
```bash
# Run with debug logging
docker run -e LOG_LEVEL=debug -e DEPHEALTH_NAME=test -e DEPHEALTH_GROUP=test ... uniproxy:dev
```

### Verify Health Checks
```bash
# Watch metrics in real-time
watch -n 1 'curl -s http://localhost:9090/metrics | grep app_dependency_health'
```

### Kubernetes Troubleshooting
```bash
# Check pod logs
kubectl logs -f -n <namespace> deployment/<release-name>

# Check environment variables in pod
kubectl exec -n <namespace> deployment/<release-name> -- env | grep DEPHEALTH

# Port-forward for local access
kubectl port-forward -n <namespace> deployment/<release-name> 8080:8080 9090:9090
```

### Common Issues
- **"DEPHEALTH_NAME is required"**: Missing required env var
- **"DEPHEALTH_GROUP is required"**: Missing required env var
- **"unsupported dependency type"**: Check `DEPHEALTH_DEPS` format (name:type)
- **Health check always DOWN**: Verify connectivity from container/pod to target host
- **LDAP health always DOWN**: Verify network connectivity to LDAP server, check bind credentials for `simple_bind`, verify StartTLS settings
- **Metrics not updating**: Check `DEPHEALTH_CHECK_INTERVAL` and SDK logs
