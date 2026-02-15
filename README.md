# uniproxy

[![Go Version](https://img.shields.io/badge/go-1.25-00ADD8.svg)](https://golang.org/)
[![dephealth SDK](https://img.shields.io/badge/dephealth_SDK-v0.4.1-blue.svg)](https://github.com/BigKAA/topologymetrics)
[![License](https://img.shields.io/badge/license-Apache%202.0-green.svg)](./LICENSE)

**Universal test proxy for dependency health monitoring with dephealth SDK**

[Русская версия (README.ru.md)](./README.ru.md)

## Overview

**uniproxy** is a lightweight Go application that health-checks configured dependencies using the [dephealth SDK](https://github.com/BigKAA/topologymetrics) and exposes Prometheus metrics. It is designed as a universal test tool for validating dephealth-ui topology visualization in any environment — Docker, Kubernetes, or bare metal.

### Key Features

- Health checking for HTTP, gRPC, PostgreSQL, MySQL, Redis, AMQP, Kafka, and TCP dependencies
- Enriched Status API with detailed dependency info and recursive HTTP chain visibility
- Configuration via environment variables (12-factor app)
- Prometheus metrics export via dephealth SDK
- Kubernetes-native with Helm chart for instance-based deployment
- Per-dependency configuration for check intervals, timeouts, TLS, and more

## Quick Start

### Docker

```bash
# Build image
docker build -t uniproxy:0.4.1 .

# Run with an HTTP dependency
docker run -p 8080:8080 \
  -e DEPHEALTH_NAME=my-proxy \
  -e DEPHEALTH_DEPS="httpbin:http" \
  -e DEPHEALTH_HTTPBIN_URL="http://httpbin.org" \
  -e DEPHEALTH_HTTPBIN_CRITICAL=yes \
  uniproxy:0.4.1
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
helm install uniproxy-01 ./deploy/helm/uniproxy \
  -f ./deploy/helm/uniproxy/instances/ns1-homelab.yaml \
  -n uniproxy-ns1 --create-namespace
```

## Configuration

All configuration is done via environment variables.

### Global Variables

| Variable | Required | Default | Description |
|----------|:--------:|:-------:|-------------|
| `DEPHEALTH_NAME` | Yes | — | Application name (used in metrics and status response) |
| `DEPHEALTH_DEPS` | No | — | Comma-separated dependency list: `name1:type1,name2:type2` |
| `LISTEN_ADDR` | No | `:8080` | HTTP server listen address |
| `LOG_LEVEL` | No | `info` | Log level (`info` or `debug`) |
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
  uniproxy:0.4.1
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

Standard dephealth SDK metrics:

- **`app_dependency_health`** (Gauge) — Health status: 1=UP, 0=DOWN
- **`app_dependency_latency_seconds`** (Histogram) — Check latency

Labels: `name`, `namespace`, `dependency`, `type`, `host`, `port`, `critical`

## Project Structure

```
uniproxy/
├── main.go                      # Entry point, SDK initialization
├── internal/
│   ├── config/
│   │   ├── config.go            # Env var parsing
│   │   └── config_test.go       # Config tests
│   └── server/
│       ├── server.go            # HTTP handlers, types
│       ├── server_test.go       # Server tests (20 tests)
│       ├── fetch.go             # Recursive HTTP fetch
│       └── fetch_test.go        # Fetch tests
├── deploy/
│   └── helm/
│       └── uniproxy/
│           ├── Chart.yaml       # Chart metadata (v0.4.1)
│           ├── values.yaml      # Default values
│           ├── templates/       # K8s manifest templates
│           └── instances/       # Instance configurations
├── plans/                       # Development plans
├── Dockerfile                   # Multi-stage Docker build
├── go.mod
├── LICENSE
└── NOTICE
```

## Helm Deployment

The Helm chart supports instance-based deployment — multiple uniproxy instances with different dependency configurations in the same namespace.

```bash
# Install
helm install uniproxy-01 ./deploy/helm/uniproxy \
  -f ./deploy/helm/uniproxy/instances/ns1-homelab.yaml \
  -n uniproxy-ns1 --create-namespace

# Upgrade
helm upgrade uniproxy-01 ./deploy/helm/uniproxy \
  -f ./deploy/helm/uniproxy/instances/ns1-homelab.yaml \
  -n uniproxy-ns1

# Debug template rendering
helm template uniproxy-01 ./deploy/helm/uniproxy \
  -f ./deploy/helm/uniproxy/instances/ns1-homelab.yaml
```

### Helm Values

| Value | Default | Description |
|-------|:-------:|-------------|
| `global.pushRegistry` | `""` | Container registry prefix |
| `image.name` | `uniproxy` | Image name |
| `image.tag` | `latest` | Image tag |
| `checkInterval` | `"10"` | Health check interval (seconds) |
| `timeout` | `""` | Global check timeout (seconds) |
| `fetchTimeout` | `"5"` | Recursive HTTP fetch timeout (seconds) |

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
