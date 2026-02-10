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

### Container Registry
Harbor: `https://harbor.kryukov.lan`
- Public storage: `harbor.kryukov.lan/library` (use for local images)
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

### Linting

```bash
# Go linting (if golangci-lint is available)
golangci-lint run

# Helm chart linting
helm lint ./deploy/helm/uniproxy
```

## Architecture & Code Structure

### High-Level Architecture

```
┌─────────────┐
│   main.go   │  Entry point
└──────┬──────┘
       │
       ├─> internal/config      Parse env vars (DEPHEALTH_NAME, DEPHEALTH_DEPS, etc.)
       │                        Build Config struct with dependencies
       │
       ├─> dephealth SDK        Initialize health checker with parsed dependencies
       │   (external)           Start background health checks (interval-based)
       │                        Register Prometheus metrics
       │
       └─> internal/server      HTTP server (Chi router)
                                Endpoints: /, /healthz, /readyz, /metrics
                                Expose SDK metrics via /metrics
```

### Key Components

**main.go**
- Loads configuration from environment variables via `internal/config`
- Initializes dephealth SDK with dependency configurations
- Starts background health checks (SDK handles periodic checking)
- Starts HTTP server with Chi router
- Handles graceful shutdown on SIGINT/SIGTERM

**internal/config/config.go**
- Parses environment variables into `Config` struct
- Required vars: `DEPHEALTH_NAME`, `DEPHEALTH_DEPS`
- Optional: `LISTEN_ADDR`, `LOG_LEVEL`, `DEPHEALTH_CHECK_INTERVAL`
- Per-dependency vars: `DEPHEALTH_<NAME>_URL` or `DEPHEALTH_<NAME>_HOST` + `DEPHEALTH_<NAME>_PORT`
- Dependency types: `http`, `redis`, `postgres`, `grpc`

**internal/server/server.go**
- HTTP server using `go-chi/chi` router
- Routes:
  - `GET /` — JSON status (name, podName, namespace, health map)
  - `GET /healthz` — Liveness probe (always returns 200 OK)
  - `GET /readyz` — Readiness probe (always returns 200 OK)
  - `GET /metrics` — Prometheus metrics (from SDK)

**External Dependency: dephealth SDK**
- `github.com/BigKAA/topologymetrics/sdk-go/dephealth`
- Handles all health checking logic (HTTP, gRPC, database connections, etc.)
- Exports Prometheus metrics:
  - `app_dependency_health` (gauge: 1=UP, 0=DOWN)
  - `app_dependency_latency_seconds` (histogram)
- Built-in checker factories registered via `_ "github.com/BigKAA/topologymetrics/sdk-go/dephealth/checks"`

### Configuration Model

uniproxy uses a **declarative configuration** approach:
1. User defines dependencies in environment variables (JSON format or individual vars)
2. `internal/config` parses these into structured `Config`
3. `main.go` builds `dephealth.Option` slice from config
4. SDK creates health checkers for each dependency
5. SDK runs checks in background and updates Prometheus metrics

This allows **instance-based deployment**: multiple uniproxy instances with different dependency configs can run in the same namespace.

### Deployment Model (Helm)

Helm chart is designed for **instance-based deployment**:
- Each Helm release = one uniproxy instance
- Instance-specific config in `deploy/helm/uniproxy/instances/<name>.yaml`
- Common defaults in `values.yaml`
- Supports multiple instances per namespace with different dependency configurations

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
1. Create feature branch from `master`
2. Make changes and commit
3. Merge to `master` with `--no-ff` or via PR
4. Delete feature branch
5. Tag releases with `vX.Y.Z` (semver)

### Quick Fixes
Small fixes (typos, minor tweaks) can be committed directly to `master`.

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
docker run -e LOG_LEVEL=debug -e DEPHEALTH_NAME=test ... uniproxy:dev
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
- **"unsupported dependency type"**: Check `DEPHEALTH_DEPS` format (name:type)
- **Health check always DOWN**: Verify connectivity from container/pod to target host
- **Metrics not updating**: Check `DEPHEALTH_CHECK_INTERVAL` and SDK logs
