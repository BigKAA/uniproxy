# uniproxy

[![Go Version](https://img.shields.io/badge/go-1.25-00ADD8.svg)](https://golang.org/)
[![dephealth SDK](https://img.shields.io/badge/dephealth_SDK-v0.4.2-blue.svg)](https://github.com/BigKAA/topologymetrics)
[![License](https://img.shields.io/badge/license-Apache%202.0-green.svg)](./LICENSE)

**Universal test proxy for dependency health monitoring with dephealth SDK**

[Русская версия (README.ru.md)](./README.ru.md)

## Overview

**uniproxy** is a lightweight Go application that health-checks configured dependencies using the [dephealth SDK](https://github.com/BigKAA/topologymetrics) and exposes Prometheus metrics. It is designed as a universal test tool for validating dephealth-ui topology visualization in any environment — Docker, Kubernetes, or bare metal.

### Key Features

- Health checking for HTTP, gRPC, PostgreSQL, MySQL, Redis, AMQP, Kafka, and TCP dependencies
- Enriched Status API with detailed dependency info and recursive HTTP chain visibility
- Configuration via environment variables or YAML file (12-factor app)
- Server-side authentication for status and metrics endpoints (Basic, Bearer, API Key)
- Prometheus metrics export via dephealth SDK
- Kubernetes-native with Helm chart for instance-based deployment
- Per-dependency configuration for check intervals, timeouts, TLS, and more
- Dependency authentication: Bearer token, Basic Auth, custom HTTP headers and gRPC metadata
- Secure secret management via `_FILE` suffix pattern (Kubernetes Secrets / Docker Secrets)

## Quick Start

### Docker

```bash
# Build image
docker build -t uniproxy:0.5.0 .

# Run with an HTTP dependency
docker run -p 8080:8080 \
  -e DEPHEALTH_NAME=my-proxy \
  -e DEPHEALTH_DEPS="httpbin:http" \
  -e DEPHEALTH_HTTPBIN_URL="http://httpbin.org" \
  -e DEPHEALTH_HTTPBIN_CRITICAL=yes \
  uniproxy:0.5.0
```

### Docker Compose

The quickest way to try uniproxy with real dependencies:

```bash
docker compose -f examples/docker-compose.yaml up -d

# Check status — Redis and PostgreSQL should be healthy
curl http://localhost:8080/

# Check Prometheus metrics
curl http://localhost:8080/metrics | grep app_dependency_health

# Stop
docker compose -f examples/docker-compose.yaml down
```

### Check Endpoints

```bash
# Simple health status
curl http://localhost:8080/

# Detailed status with dependency info
curl "http://localhost:8080/?detail=true"

# Detailed status with recursive HTTP fetch (depth=2)
curl "http://localhost:8080/?detail=true&depth=2"

# Liveness / readiness probes
curl http://localhost:8080/healthz
curl http://localhost:8080/readyz

# Prometheus metrics
curl http://localhost:8080/metrics | grep app_dependency
```

### Kubernetes (Helm)

```bash
helm install my-proxy charts/uniproxy \
  --set config.name=my-proxy \
  -n my-namespace --create-namespace
```

## Configuration

uniproxy supports two configuration methods:

1. **Environment variables** — traditional 12-factor approach (always works)
2. **YAML configuration file** — structured config with env var overrides

Environment variables always take priority over YAML values.

### YAML Configuration

Set `CONFIG_FILE` to the path of a YAML file to use structured configuration:

```bash
docker run -p 8080:8080 \
  -e CONFIG_FILE=/config/config.yaml \
  -v ./config.yaml:/config/config.yaml:ro \
  uniproxy:0.5.0
```

Example YAML file:

```yaml
name: my-proxy
listenAddr: ":8080"
checkInterval: "15s"
fetchTimeout: "3s"

log:
  format: json
  level: info

auth:
  method: bearer
  token: "my-secret-token"
  metrics:
    method: none

dependencies:
  - name: backend
    type: http
    url: "http://backend.svc:8080"
    critical: true
    healthPath: "/"
  - name: cache
    type: redis
    host: redis.svc
    port: "6379"
    critical: false
```

See [examples/config.yaml](./examples/config.yaml) for a full example with all fields.

**Priority rules:**
- `CONFIG_FILE` env var → load YAML as base config
- Environment variables → override YAML values
- `DEPHEALTH_DEPS` env var → **replaces** all YAML dependencies (no merging)
- Per-dependency env vars → overlay on existing (YAML-loaded) dependencies

### Global Variables

| Variable | Required | Default | Description |
|----------|:--------:|:-------:|-------------|
| `CONFIG_FILE` | No | — | Path to YAML configuration file |
| `DEPHEALTH_NAME` | Yes | — | Application name (used in metrics and status response) |
| `DEPHEALTH_DEPS` | No | — | Comma-separated dependency list: `name1:type1,name2:type2` |
| `LISTEN_ADDR` | No | `:8080` | HTTP server listen address |
| `LOG_FORMAT` | No | `text` | Log output format: `text` or `json` |
| `LOG_LEVEL` | No | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `LOG_TIME_FORMAT` | No | `rfc3339nano` | Timestamp format: `rfc3339`, `rfc3339nano`, `unix`, `unixmilli` |
| `LOG_ADD_SOURCE` | No | `false` | Include source file:line in log output (`true`/`false`) |
| `LOG_TIME_KEY` | No | `time` | JSON key for timestamp (only effective with `LOG_FORMAT=json`) |
| `LOG_LEVEL_KEY` | No | `level` | JSON key for log level (only effective with `LOG_FORMAT=json`) |
| `LOG_MESSAGE_KEY` | No | `msg` | JSON key for message (only effective with `LOG_FORMAT=json`) |
| `LOG_SOURCE_KEY` | No | `source` | JSON key for source location (only effective with `LOG_FORMAT=json`) |
| `DEPHEALTH_CHECK_INTERVAL` | No | `10` | Health check interval in seconds |
| `DEPHEALTH_TIMEOUT` | No | SDK default | Global health check timeout in seconds |
| `DEPHEALTH_FETCH_TIMEOUT` | No | `5` | Timeout for recursive HTTP detail fetch in seconds |

### Per-Dependency Variables

For each dependency listed in `DEPHEALTH_DEPS`, configure it using environment variables with the prefix `DEPHEALTH_<NAME>_`, where `<NAME>` is the dependency name converted to uppercase with hyphens replaced by underscores (e.g., `my-backend` becomes `MY_BACKEND`).

| Variable | Required | Description |
|----------|:--------:|-------------|
| `DEPHEALTH_<NAME>_URL` | Yes* | Connection URL |
| `DEPHEALTH_<NAME>_HOST` | Yes* | Target host (alternative to URL) |
| `DEPHEALTH_<NAME>_PORT` | Yes* | Target port (required with HOST) |
| `DEPHEALTH_<NAME>_CRITICAL` | Yes | Critical dependency flag (`yes`/`no`) |
| `DEPHEALTH_<NAME>_CHECK_INTERVAL` | No | Per-dependency check interval (seconds) |
| `DEPHEALTH_<NAME>_TIMEOUT` | No | Per-dependency timeout (seconds) |
| `DEPHEALTH_<NAME>_HEALTH_PATH` | No | HTTP health check path |
| `DEPHEALTH_<NAME>_TLS` | No | Enable TLS (`yes`/`no`, HTTP/gRPC) |
| `DEPHEALTH_<NAME>_TLS_SKIP_VERIFY` | No | Skip TLS verification (`yes`/`no`) |
| `DEPHEALTH_<NAME>_GRPC_SERVICE_NAME` | No | gRPC service name for health check |
| `DEPHEALTH_<NAME>_POSTGRES_QUERY` | No | Custom PostgreSQL health check query |
| `DEPHEALTH_<NAME>_MYSQL_QUERY` | No | Custom MySQL health check query |
| `DEPHEALTH_<NAME>_REDIS_PASSWORD` | No | Redis authentication password |
| `DEPHEALTH_<NAME>_REDIS_DB` | No | Redis database number |
| `DEPHEALTH_<NAME>_AMQP_URL` | No | AMQP connection URL |

*Either `URL` or `HOST` + `PORT` is required.

### Server Authentication

uniproxy supports server-side authentication to protect its own endpoints. Authentication is configured per zone — the status API (`/`) and Prometheus metrics (`/metrics`) can have independent auth settings. Health probes (`/healthz`, `/readyz`) are always open.

#### Server Auth Methods

| Method | Description |
|--------|-------------|
| `none` | No authentication (default) |
| `basic` | HTTP Basic Auth (`Authorization: Basic <base64>`) |
| `bearer` | Bearer token (`Authorization: Bearer <token>`) |
| `apikey` | API key via `X-API-Key` header |

#### Server Auth Variables

| Variable | Description |
|----------|-------------|
| `AUTH_METHOD` | Global auth method: `none`, `basic`, `bearer`, `apikey` |
| `AUTH_USER` | Global Basic Auth username |
| `AUTH_PASS` | Global Basic Auth password |
| `AUTH_PASS_FILE` | Read password from file |
| `AUTH_TOKEN` | Global bearer token |
| `AUTH_TOKEN_FILE` | Read token from file |
| `AUTH_API_KEY` | Global API key |
| `AUTH_API_KEY_FILE` | Read API key from file |

#### Per-Zone Overrides

Each zone (`status`, `metrics`) can override the global auth settings:

| Variable | Description |
|----------|-------------|
| `AUTH_STATUS_METHOD` | Override method for `/` endpoint |
| `AUTH_STATUS_USER` | Override username for `/` |
| `AUTH_STATUS_PASS` | Override password for `/` |
| `AUTH_STATUS_TOKEN` | Override token for `/` |
| `AUTH_STATUS_API_KEY` | Override API key for `/` |
| `AUTH_METRICS_METHOD` | Override method for `/metrics` |
| `AUTH_METRICS_USER` | Override username for `/metrics` |
| `AUTH_METRICS_PASS` | Override password for `/metrics` |
| `AUTH_METRICS_TOKEN` | Override token for `/metrics` |
| `AUTH_METRICS_API_KEY` | Override API key for `/metrics` |

All `_PASS`, `_TOKEN`, and `_API_KEY` variables support the `_FILE` suffix pattern.

#### Server Auth Examples

```bash
# Protect status API with bearer token, leave metrics open
docker run -p 8080:8080 \
  -e DEPHEALTH_NAME=my-proxy \
  -e DEPHEALTH_DEPS="httpbin:http" \
  -e DEPHEALTH_HTTPBIN_URL="http://httpbin.org" \
  -e DEPHEALTH_HTTPBIN_CRITICAL=yes \
  -e AUTH_METHOD=bearer \
  -e AUTH_TOKEN=my-secret-token \
  -e AUTH_METRICS_METHOD=none \
  uniproxy:0.5.0

# Test access
curl http://localhost:8080/                                        # 401
curl -H "Authorization: Bearer my-secret-token" http://localhost:8080/  # 200
curl http://localhost:8080/metrics                                 # 200 (override: none)
curl http://localhost:8080/healthz                                 # 200 (always open)
```

### Dependency Authentication

uniproxy supports authentication for HTTP and gRPC dependencies. Auth can be configured globally or per-dependency; per-dependency settings override global ones entirely.

#### Dependency Auth Methods

| Method | HTTP | gRPC | Description |
|--------|:----:|:----:|-------------|
| Bearer Token | Yes | Yes | `Authorization: Bearer <token>` header / gRPC call credential |
| Basic Auth | Yes | Yes | `Authorization: Basic <base64>` header / gRPC call credential |
| Custom Headers | Yes | — | Arbitrary HTTP headers (e.g., API keys) |
| Custom Metadata | — | Yes | Arbitrary gRPC metadata key-value pairs |

Only **one auth method** per dependency is allowed. Setting both bearer token and basic auth is an error.

#### Global Auth Variables

Applied to all dependencies that don't have per-dependency auth configured.

| Variable | Description |
|----------|-------------|
| `DEPHEALTH_BEARER_TOKEN` | Global bearer token |
| `DEPHEALTH_BEARER_TOKEN_FILE` | Read bearer token from file (mutually exclusive with above) |
| `DEPHEALTH_BASIC_USER` | Global Basic Auth username |
| `DEPHEALTH_BASIC_PASS` | Global Basic Auth password |
| `DEPHEALTH_BASIC_PASS_FILE` | Read password from file (mutually exclusive with above) |
| `DEPHEALTH_HEADERS` | Global custom HTTP headers (JSON: `{"Key":"Value"}`) |
| `DEPHEALTH_METADATA` | Global custom gRPC metadata (JSON: `{"key":"value"}`) |

Global headers are only applied to HTTP dependencies; global metadata is only applied to gRPC dependencies.

#### Per-Dependency Auth Variables

| Variable | Description |
|----------|-------------|
| `DEPHEALTH_<NAME>_BEARER_TOKEN` | Bearer token for this dependency |
| `DEPHEALTH_<NAME>_BEARER_TOKEN_FILE` | Read bearer token from file |
| `DEPHEALTH_<NAME>_BASIC_USER` | Basic Auth username |
| `DEPHEALTH_<NAME>_BASIC_PASS` | Basic Auth password |
| `DEPHEALTH_<NAME>_BASIC_PASS_FILE` | Read password from file |
| `DEPHEALTH_<NAME>_HEADERS` | Custom HTTP headers (JSON string) |
| `DEPHEALTH_<NAME>_METADATA` | Custom gRPC metadata (JSON string) |

#### `_FILE` Suffix Pattern

For any secret variable (`BEARER_TOKEN`, `BASIC_PASS`), you can append `_FILE` to read the value from a file instead of an environment variable. This is the recommended approach for Kubernetes Secrets and Docker Secrets:

```bash
# Mount K8s Secret as a file and reference it
-e DEPHEALTH_API_BEARER_TOKEN_FILE=/run/secrets/api-token
```

Rules:
- Setting both `VAR` and `VAR_FILE` is an error
- File content is trimmed of leading/trailing whitespace
- File must exist and be readable

#### Validation Rules

1. **One method per dependency**: bearer token, basic auth, headers, or metadata — pick one
2. **Complete basic auth**: both `BASIC_USER` and `BASIC_PASS` must be set together
3. **Type check**: `HEADERS` is only valid for HTTP; `METADATA` is only valid for gRPC
4. **No VAR + VAR_FILE**: cannot set both inline value and file reference

#### Auth Example

```bash
docker run -p 8080:8080 \
  -e DEPHEALTH_NAME=auth-proxy \
  -e DEPHEALTH_DEPS="secure-api:http,grpc-svc:grpc" \
  -e DEPHEALTH_SECURE_API_URL="https://api.example.com" \
  -e DEPHEALTH_SECURE_API_CRITICAL=yes \
  -e DEPHEALTH_SECURE_API_BEARER_TOKEN="eyJhbGciOi..." \
  -e DEPHEALTH_GRPC_SVC_HOST=grpc.example.com \
  -e DEPHEALTH_GRPC_SVC_PORT=443 \
  -e DEPHEALTH_GRPC_SVC_CRITICAL=yes \
  -e DEPHEALTH_GRPC_SVC_BASIC_USER=admin \
  -e DEPHEALTH_GRPC_SVC_BASIC_PASS=secret \
  uniproxy:0.5.0
```

### Supported Dependency Types

`http`, `grpc`, `tcp`, `postgres`, `mysql`, `redis`, `amqp`, `kafka`

### Configuration Example

```bash
docker run -p 8080:8080 \
  -e DEPHEALTH_NAME=frontend \
  -e DEPHEALTH_CHECK_INTERVAL=15 \
  -e DEPHEALTH_FETCH_TIMEOUT=3 \
  -e DEPHEALTH_DEPS="backend:http,cache:redis,db:postgres" \
  -e DEPHEALTH_BACKEND_URL="http://backend.svc:8080" \
  -e DEPHEALTH_BACKEND_CRITICAL=yes \
  -e DEPHEALTH_BACKEND_HEALTH_PATH="/" \
  -e DEPHEALTH_CACHE_HOST=redis.svc \
  -e DEPHEALTH_CACHE_PORT=6379 \
  -e DEPHEALTH_CACHE_CRITICAL=no \
  -e DEPHEALTH_DB_URL="postgres://user:pass@pg.svc:5432/mydb" \
  -e DEPHEALTH_DB_CRITICAL=yes \
  uniproxy:0.5.0
```

## API Endpoints

### `GET /` — Simple Status

Returns basic health status for backward compatibility.

```json
{
  "name": "frontend",
  "podName": "frontend-7b9f4-xk2lm",
  "namespace": "production",
  "health": {
    "backend:backend.svc:8080": true,
    "cache:redis.svc:6379": true,
    "db:pg.svc:5432": false
  }
}
```

### `GET /?detail=true` — Enriched Status

Returns detailed dependency information from the SDK's `HealthDetails()` API.

**Query parameters:**

| Parameter | Default | Description |
|-----------|:-------:|-------------|
| `detail` | — | Set to `true` to enable enriched response |
| `depth` | `1` | Recursion depth for HTTP dependency fetch (0–10) |

```json
{
  "name": "frontend",
  "podName": "frontend-7b9f4-xk2lm",
  "namespace": "production",
  "dependencies": {
    "backend:backend.svc:8080": {
      "healthy": true,
      "status": "ok",
      "detail": "200_ok",
      "latency_ms": 12.5,
      "type": "http",
      "name": "backend",
      "host": "backend.svc",
      "port": "8080",
      "critical": true,
      "last_checked_at": "2026-02-15T10:30:45Z",
      "labels": {},
      "response": {
        "name": "backend",
        "podName": "backend-5c8d2-m9n3p",
        "namespace": "production",
        "dependencies": {}
      }
    },
    "db:pg.svc:5432": {
      "healthy": false,
      "status": "connection_error",
      "detail": "connection_refused",
      "latency_ms": 1230.5,
      "type": "postgres",
      "name": "db",
      "host": "pg.svc",
      "port": "5432",
      "critical": true,
      "last_checked_at": "2026-02-15T10:30:44Z",
      "labels": {}
    }
  }
}
```

**How recursive fetch works:**

- For HTTP-type dependencies with `depth > 0`, uniproxy makes an HTTP request to the dependency at `http://<host>:<port>/?detail=true&depth=N-1`
- The response is included in the `response` field of the dependency
- Non-HTTP dependencies never have a `response` field
- If the downstream is unreachable, `response` is omitted
- `depth=0` disables recursive fetch entirely
- `DEPHEALTH_FETCH_TIMEOUT` controls the timeout for all parallel fetches

**Status categories:** `ok`, `timeout`, `connection_error`, `dns_error`, `auth_error`, `tls_error`, `unhealthy`, `error`, `unknown`

### `GET /healthz` — Liveness Probe

Always returns `200 OK` with body `ok`.

### `GET /readyz` — Readiness Probe

Always returns `200 OK` with body `ok`.

### `GET /metrics` — Prometheus Metrics

dephealth SDK exports the following Prometheus metrics:

| Metric | Type | Description |
|--------|------|-------------|
| `app_dependency_health` | Gauge | Health status: 1 = healthy, 0 = unhealthy |
| `app_dependency_latency_seconds` | Histogram | Health check latency in seconds. Buckets: 1ms, 5ms, 10ms, 50ms, 100ms, 500ms, 1s, 5s |
| `app_dependency_status` | Gauge | Category of the last check result (enum pattern — exactly one status value is set to 1, the rest to 0) |
| `app_dependency_status_detail` | Gauge | Detailed reason of the last check result (state-set pattern — always 1 with detail label) |

**Base labels** (all metrics): `name`, `dependency`, `type`, `host`, `port`, `critical`

Additional labels per metric:
- `app_dependency_status` adds label **`status`** with possible values: `ok`, `timeout`, `connection_error`, `dns_error`, `auth_error`, `tls_error`, `unhealthy`, `error`
- `app_dependency_status_detail` adds label **`detail`** with a human-readable reason string

## Project Structure

```
uniproxy/
├── main.go                      # Entry point, SDK initialization
├── internal/
│   ├── auth/                    # Server-side auth middleware (Basic/Bearer/APIKey)
│   ├── config/                  # Env var + YAML config parsing
│   ├── logging/                 # Structured logging setup
│   └── server/                  # HTTP handlers, recursive fetch
├── charts/
│   └── uniproxy/                # Standard Helm chart (single-instance)
│       ├── Chart.yaml
│       ├── values.yaml
│       └── templates/           # Deployment, Service, Ingress, HTTPRoute, ConfigMap
├── deploy/
│   └── helm/
│       └── uniproxy/            # Legacy multi-instance Helm chart
├── examples/
│   ├── config.yaml              # Full YAML configuration example
│   └── docker-compose.yaml      # Docker Compose quick start (uniproxy + Redis + PostgreSQL)
├── Dockerfile                   # Multi-stage Docker build
├── go.mod
├── LICENSE
└── NOTICE
```

## Helm Deployment

The standard Helm chart (`charts/uniproxy/`) provides a single-instance deployment with full support for Service types, Ingress, Gateway API, server auth, and YAML config.

```bash
# Install with minimal config
helm install my-proxy charts/uniproxy \
  --set config.name=my-proxy \
  -n monitoring --create-namespace

# Install with values file
helm install my-proxy charts/uniproxy \
  -f my-values.yaml \
  -n monitoring --create-namespace

# Upgrade
helm upgrade my-proxy charts/uniproxy -f my-values.yaml -n monitoring

# Debug template rendering
helm template my-proxy charts/uniproxy -f my-values.yaml
```

### Service Types

**ClusterIP** (default):

```yaml
service:
  type: ClusterIP
  port: 8080
```

**NodePort**:

```yaml
service:
  type: NodePort
  port: 8080
  nodePort: 30080
```

**LoadBalancer**:

```yaml
service:
  type: LoadBalancer
  port: 8080
  loadBalancerIP: "192.168.1.100"
```

### Ingress

```yaml
ingress:
  enabled: true
  className: nginx
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
  hosts:
    - host: uniproxy.example.com
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: uniproxy-tls
      hosts:
        - uniproxy.example.com
```

### Gateway API

```yaml
gateway:
  enabled: true
  parentRefs:
    - name: my-gateway
      namespace: gateway-ns
      sectionName: https
  hostnames:
    - uniproxy.example.com
```

### Helm Values

| Value | Default | Description |
|-------|:-------:|-------------|
| `replicaCount` | `1` | Number of pod replicas |
| `image.repository` | `harbor.kryukov.lan/library/uniproxy` | Image repository |
| `image.tag` | `""` (appVersion) | Image tag |
| `config.name` | `""` (release name) | `DEPHEALTH_NAME` — application name |
| `config.listenAddr` | `":8080"` | `LISTEN_ADDR` |
| `config.checkInterval` | `"10"` | `DEPHEALTH_CHECK_INTERVAL` |
| `config.timeout` | `""` | `DEPHEALTH_TIMEOUT` |
| `config.fetchTimeout` | `"5"` | `DEPHEALTH_FETCH_TIMEOUT` |
| `config.deps` | `""` | `DEPHEALTH_DEPS` (e.g. `"backend:http,cache:redis"`) |
| `log.format` | `""` | `LOG_FORMAT`: `text` or `json` |
| `log.level` | `""` | `LOG_LEVEL`: `debug`, `info`, `warn`, `error` |
| `configFile.enabled` | `false` | Mount YAML config via ConfigMap |
| `configFile.content` | — | YAML config content |
| `serverAuth.method` | `"none"` | Server auth: `none`, `basic`, `bearer`, `apikey` |
| `serverAuth.existingSecret` | — | K8s Secret name for credentials |
| `serverAuth.status` | — | Per-zone override for `/` endpoint |
| `serverAuth.metrics` | — | Per-zone override for `/metrics` endpoint |
| `extraEnv` | `[]` | Additional environment variables |
| `service.type` | `ClusterIP` | Service type: `ClusterIP`, `NodePort`, `LoadBalancer` |
| `service.port` | `8080` | Service port |
| `ingress.enabled` | `false` | Enable Ingress resource |
| `gateway.enabled` | `false` | Enable HTTPRoute (Gateway API) |

See [`charts/uniproxy/values.yaml`](./charts/uniproxy/values.yaml) for all available values.

### Legacy Multi-Instance Chart

The chart at `deploy/helm/uniproxy/` supports deploying multiple uniproxy instances from a single Helm release. See [deploy/helm/uniproxy/values.yaml](./deploy/helm/uniproxy/values.yaml) for details.

## Testing

```bash
# Run all tests
go test ./...

# With coverage
go test -cover ./...

# Specific package
go test -v ./internal/server
```

## Use Cases

See [Use Cases and Examples](./docs/use-cases.md) for detailed scenarios including:

- Recursive chain diagnostics without Prometheus (single `curl` → full dependency tree)
- Kubernetes microservice chains, Docker Compose, bare metal / VM deployments
- Mixed environments (K8s + VMs) with cross-boundary visibility
- Sidecar for legacy applications without SDK integration
- Network policy / firewall testing, CI/CD health gates
- Multi-cluster monitoring, DB migration readiness, disaster recovery verification
- Authentication: bearer tokens, K8s Secrets, custom API key headers

## Integration with dephealth-ui

uniproxy is designed to work with [dephealth-ui](https://github.com/BigKAA/dephealth-ui) for topology visualization.

1. Deploy uniproxy instances in your Kubernetes cluster
2. Configure Prometheus to scrape uniproxy pods
3. Point dephealth-ui to your Prometheus instance
4. Use `?detail=true&depth=N` for deep dependency chain inspection

## License

Apache License 2.0 — see [LICENSE](./LICENSE) for details.

## Related Projects

- [dephealth SDK](https://github.com/BigKAA/topologymetrics) — Health checking and metrics library
- [dephealth-ui](https://github.com/BigKAA/dephealth-ui) — Web UI for topology visualization
