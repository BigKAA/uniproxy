# uniproxy

[![Go Version](https://img.shields.io/badge/go-1.25-00ADD8.svg)](https://golang.org/)
[![License](https://img.shields.io/badge/license-Apache%202.0-green.svg)](./LICENSE)

**Universal test proxy for dependency health monitoring with dephealth SDK**

## Overview

**uniproxy** is a lightweight Go application that health-checks configured dependencies using the [dephealth SDK](https://github.com/BigKAA/topologymetrics) and exposes Prometheus metrics. It's designed as a universal test tool for validating dephealth-ui topology visualization in Kubernetes environments.

### Key Features

- ✅ Universal dependency health checking (HTTP, gRPC, PostgreSQL, Redis, etc.)
- ✅ Configuration via environment variables (12-factor app)
- ✅ Prometheus metrics export via dephealth SDK
- ✅ Multi-architecture Docker images (amd64, arm64)
- ✅ Kubernetes-native with Helm chart
- ✅ Instance-based deployment (multiple uniproxy instances with different configs)

## Quick Start

### Prerequisites

- Go 1.25+
- Docker (for containerized deployment)
- Kubernetes cluster (for Helm deployment)

### Local Development

```bash
# Clone repository
git clone https://github.com/BigKAA/uniproxy.git
cd uniproxy

# Download dependencies
go mod download

# Run locally
export UNIPROXY_NAME=test-proxy
export UNIPROXY_NAMESPACE=default
export UNIPROXY_LISTEN_ADDR=:8080
export UNIPROXY_METRICS_ADDR=:9090
export UNIPROXY_CHECK_INTERVAL=10s
export UNIPROXY_DEPENDENCIES='[{"name":"postgres","type":"postgres","host":"localhost","port":5432,"critical":true}]'

go run main.go
```

### Docker

```bash
# Build image
docker build -t uniproxy:latest .

# Run container
docker run -p 8080:8080 -p 9090:9090 \
  -e UNIPROXY_NAME=test-proxy \
  -e UNIPROXY_NAMESPACE=default \
  -e UNIPROXY_DEPENDENCIES='[{"name":"test-dep","type":"http","host":"httpbin.org","port":80}]' \
  uniproxy:latest
```

### Kubernetes (Helm)

```bash
# Install with Helm
helm install uniproxy-01 ./deploy/helm/uniproxy \
  -f ./deploy/helm/uniproxy/instances/example.yaml \
  -n uniproxy --create-namespace
```

## Configuration

### Environment Variables

| Variable | Required | Description | Example |
|----------|:--------:|-------------|---------|
| `UNIPROXY_NAME` | Yes | Service name (used in metrics) | `uniproxy-01` |
| `UNIPROXY_NAMESPACE` | Yes | Kubernetes namespace or logical group | `production` |
| `UNIPROXY_LISTEN_ADDR` | No | HTTP server listen address | `:8080` (default) |
| `UNIPROXY_METRICS_ADDR` | No | Prometheus metrics address | `:9090` (default) |
| `UNIPROXY_CHECK_INTERVAL` | No | Health check interval | `10s` (default) |
| `UNIPROXY_DEPENDENCIES` | Yes | JSON array of dependency configs | See below |
| `UNIPROXY_LOG_LEVEL` | No | Log level (info/debug) | `info` (default) |

### Dependency Configuration

`UNIPROXY_DEPENDENCIES` must be a JSON array of dependency objects:

```json
[
  {
    "name": "postgres-main",
    "type": "postgres",
    "host": "pg-master.db.svc.cluster.local",
    "port": 5432,
    "critical": true,
    "role": "primary",
    "username": "user",
    "password": "pass",
    "database": "mydb"
  },
  {
    "name": "redis-cache",
    "type": "redis",
    "host": "redis.cache.svc.cluster.local",
    "port": 6379,
    "critical": false
  },
  {
    "name": "auth-service",
    "type": "http",
    "host": "auth.svc.cluster.local",
    "port": 8080,
    "critical": true,
    "path": "/health"
  }
]
```

**Supported Dependency Types:**
- `http` / `https`
- `grpc`
- `tcp`
- `postgres`
- `mysql`
- `redis`
- `mongodb`
- `amqp`
- `kafka`

## Exposed Metrics

uniproxy exposes standard dephealth SDK metrics on `:9090/metrics`:

### `app_dependency_health` (Gauge)
Health status (1=UP, 0=DOWN) for each dependency endpoint.

**Labels:**
- `name` — service name (from `UNIPROXY_NAME`)
- `namespace` — namespace (from `UNIPROXY_NAMESPACE`)
- `dependency` — dependency name
- `type` — connection type
- `host` — target host
- `port` — target port
- `critical` — criticality flag (yes/no)

### `app_dependency_latency_seconds` (Histogram)
Health check latency in seconds with buckets: `0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0, 5.0`

## Project Structure

```
uniproxy/
├── main.go                    # Application entry point
├── internal/
│   ├── config/               # Environment configuration
│   └── server/               # HTTP server
├── deploy/
│   └── helm/
│       └── uniproxy/         # Helm chart
│           ├── templates/    # K8s manifests
│           ├── instances/    # Instance configurations
│           └── values.yaml   # Default values
├── Dockerfile                # Multi-arch Docker build
├── go.mod                    # Go dependencies
├── README.md                 # This file
├── AGENTS.md                 # AI agent guidelines
└── GIT-WORKFLOW.md           # Git workflow documentation
```

## Deployment

### Helm Chart

The Helm chart supports instance-based deployment, allowing multiple uniproxy instances with different dependency configurations in the same namespace.

**Example:**
```bash
# Deploy instance 1 (checks postgres + redis)
helm install uniproxy-01 ./deploy/helm/uniproxy \
  -f ./deploy/helm/uniproxy/instances/ns1-example.yaml \
  -n uniproxy-ns1 --create-namespace

# Deploy instance 2 (checks different services)
helm install uniproxy-02 ./deploy/helm/uniproxy \
  -f ./deploy/helm/uniproxy/instances/ns2-example.yaml \
  -n uniproxy-ns2 --create-namespace
```

See [deploy/helm/uniproxy/README.md](./deploy/helm/uniproxy/README.md) for details.

## Development

### Build

```bash
# Build binary
go build -o uniproxy main.go

# Run tests
go test ./...

# Build Docker image
docker build -t uniproxy:dev .
```

### Testing

Create a test configuration:

```bash
export UNIPROXY_NAME=test
export UNIPROXY_NAMESPACE=default
export UNIPROXY_DEPENDENCIES='[
  {"name":"httpbin","type":"http","host":"httpbin.org","port":80,"path":"/status/200"},
  {"name":"google","type":"http","host":"www.google.com","port":443,"path":"/"}
]'

go run main.go
```

Check metrics:
```bash
curl http://localhost:9090/metrics | grep app_dependency
```

## Integration with dephealth-ui

uniproxy is designed to work with [dephealth-ui](https://github.com/BigKAA/dephealth-ui) for topology visualization.

**Workflow:**
1. Deploy uniproxy instances in your Kubernetes cluster
2. Configure Prometheus to scrape uniproxy pods
3. Point dephealth-ui to your Prometheus instance
4. View real-time topology in dephealth-ui web interface

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes using [Conventional Commits](https://www.conventionalcommits.org/)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

See [GIT-WORKFLOW.md](./GIT-WORKFLOW.md) for detailed workflow.

## License

Apache License 2.0 - see [LICENSE](./LICENSE) for details.

## Related Projects

- [dephealth-ui](https://github.com/BigKAA/dephealth-ui) — Web UI for topology visualization
- [dephealth SDK](https://github.com/BigKAA/topologymetrics) — Instrumentation library

---

**Built with ❤️ for microservices observability testing**
