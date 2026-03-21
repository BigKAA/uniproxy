# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.7.3] - 2026-03-21

### Added

- **Graceful shutdown** with configurable timeout (30s default)
  - Stops health check scheduling on SIGINT/SIGTERM
  - Waits for active HTTP requests to complete
  - Configurable via `SHUTDOWN_TIMEOUT` environment variable

- **Circuit breaker** for downstream HTTP fetches
  - Prevents cascading failures when downstream services are unhealthy
  - Configurable max failures (default: 5), timeout (default: 60s), half-open limit (default: 3)
  - Metrics: `uniproxy_circuit_breaker_state`, `uniproxy_circuit_breaker_requests_total`

- **HTTPS downstream support** for recursive HTTP fetch
  - Automatic scheme detection: port 443 → HTTPS, other ports → HTTP
  - TLS verification can be skipped per dependency via `TLS_SKIP_VERIFY`

- **HTTP Transport tuning** for recursive fetches
  - Configurable connection pooling: max idle connections, per-host limits, idle timeout
  - Environment variables: `HTTP_TRANSPORT_MAX_IDLE_CONNS`, `HTTP_TRANSPORT_MAX_IDLE_CONNS_PER_HOST`, `HTTP_TRANSPORT_IDLE_CONN_TIMEOUT`
  - Metrics: `uniproxy_http_pool_idle_connections`, `uniproxy_http_pool_requests_total`

- **Resilience settings configuration** via YAML
  - New `shutdownTimeout`, `circuitBreaker`, and `httpTransport` config sections
  - Full YAML example in `examples/config.yaml`

### Changed

- Improved documentation for AI coding agents (AGENTS.md)

### Fixed

- N/A

### Security

- N/A

## [0.7.2] - 2026-03-09

### Added

- N/A

### Changed

- N/A

### Fixed

- N/A

### Security

- N/A

## [0.7.1] - 2026-02-09

### Added

- `hostHeader` support for HTTP dependencies behind ingress/proxy routing
- `grpcAuthority` support for gRPC services behind proxy
- Documentation updates and examples

[0.7.3]: https://github.com/BigKAA/uniproxy/compare/v0.7.1...v0.7.3
[0.7.2]: https://github.com/BigKAA/uniproxy/compare/v0.7.1...v0.7.2
[0.7.1]: https://github.com/BigKAA/uniproxy/releases/v0.7.1
