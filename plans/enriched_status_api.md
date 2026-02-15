# Plan: Enriched Status API (GET /?detail=true)

## Metadata

- **Version**: 2.0.0
- **Created**: 2026-02-14
- **Last updated**: 2026-02-15
- **Status**: In Progress
- **Blocked by**: ~~SDK feature request (HealthDetails API)~~ — Resolved in SDK v0.4.1

---

## Version History

- **v2.0.0** (2026-02-15): Updated plan for SDK v0.4.1 actual API (11-field EndpointStatus, custom JSON marshaling, StatusCategory type)
- **v1.0.0** (2026-02-14): Initial plan

---

## Current Status

- **Active phase**: Completed
- **Active subtask**: None
- **Last updated**: 2026-02-15
- **Note**: All 5 phases completed — unit tests (88.3%/92.3% coverage), Docker image 0.4.1 built, integration test passed, Helm chart updated to 0.4.1

---

## Table of Contents

- [x] [Phase 1: SDK upgrade and configuration](#phase-1-sdk-upgrade-and-configuration)
- [x] [Phase 2: Detail response handler](#phase-2-detail-response-handler)
- [x] [Phase 3: Recursive HTTP fetch](#phase-3-recursive-http-fetch)
- [x] [Phase 4: Build and test](#phase-4-build-and-test)
- [x] [Phase 5: Helm chart update](#phase-5-helm-chart-update)

---

## Context

### Problem

Current `GET /` returns minimal info:
```json
{"name": "...", "podName": "...", "namespace": "...", "health": {"dep:host:port": true}}
```

Users need to see **why** dependencies fail and for HTTP dependencies (other uniproxy instances) see the **full response** including downstream health.

### Solution

Add query parameter `?detail=true&depth=N` that returns enriched response:
```json
{
  "name": "my-frontend",
  "podName": "...",
  "namespace": "...",
  "dependencies": {
    "backend-api:10.0.0.50:8080": {
      "healthy": false,
      "status": "connection_error",
      "detail": "connection_refused",
      "latency_ms": 1230.5,
      "type": "http",
      "name": "backend-api",
      "host": "10.0.0.50",
      "port": "8080",
      "critical": true,
      "last_checked_at": "2026-02-15T10:30:45Z",
      "labels": {},
      "response": null
    },
    "main-db:10.0.0.100:5432": {
      "healthy": true,
      "status": "ok",
      "detail": "200_ok",
      "latency_ms": 50.0,
      "type": "postgres",
      "name": "main-db",
      "host": "10.0.0.100",
      "port": "5432",
      "critical": true,
      "last_checked_at": "2026-02-15T10:30:44Z",
      "labels": {}
    }
  }
}
```

### Key decisions

| Decision | Choice |
|---|---|
| Data source | SDK `HealthDetails()` method (available in v0.4.1) |
| Response structure | SDK `EndpointStatus` fields + `response` field for HTTP recursive fetch |
| HTTP dep response | Recursive fetch at request time |
| Backward compat | `?detail=true` query param; plain `GET /` unchanged |
| Recursion control | `?depth=N` param (default 1, min 0), passes `depth=N-1` to HTTP deps |
| Fetch timeout | Env `DEPHEALTH_FETCH_TIMEOUT` (default 5 seconds) |

### SDK v0.4.1 API (actual)

`HealthDetails()` returns `map[string]EndpointStatus` where key = `"name:host:port"`.

```go
type EndpointStatus struct {
    Healthy       *bool             `json:"healthy"`         // nil = UNKNOWN, true = UP, false = DOWN
    Status        StatusCategory    `json:"status"`          // "ok", "timeout", "connection_error", ...
    Detail        string            `json:"detail"`          // "200_ok", "connection_refused", ...
    Latency       time.Duration     `json:"-"`               // internal; serialized as latency_ms
    Type          DependencyType    `json:"type"`            // "http", "postgres", "tcp", ...
    Name          string            `json:"name"`            // dependency name
    Host          string            `json:"host"`            // hostname/IP
    Port          string            `json:"port"`            // port (string)
    Critical      bool              `json:"critical"`        // critical dependency flag
    LastCheckedAt time.Time         `json:"last_checked_at"` // ISO 8601 or null
    Labels        map[string]string `json:"labels"`          // custom labels, always {}
}
// Custom MarshalJSON: latency → latency_ms (float64 ms), zero time → null, labels → {}
// Helper: LatencyMillis() float64
```

`StatusCategory` values: `ok`, `timeout`, `connection_error`, `dns_error`, `auth_error`, `tls_error`, `unhealthy`, `error`, `unknown`.

---

## Phase 1: SDK upgrade and configuration

**Dependencies**: None (SDK v0.4.1 released)
**Status**: Completed

### Description

Update SDK dependency to v0.4.1 and add FetchTimeout configuration option.

### Subtasks

- [x] **1.1 Update go.mod to SDK v0.4.1**
  - **Dependencies**: None
  - **Description**: Update SDK dependency: `go get github.com/BigKAA/topologymetrics/sdk-go@v0.4.1`. Verify compilation. The SDK now exports `HealthDetails()`, `EndpointStatus`, `StatusCategory`, `DependencyType`.
  - **Modifies**:
    - `go.mod`
    - `go.sum`

- [x] **1.2 Add FetchTimeout to Config**
  - **Dependencies**: None
  - **Description**: Add `FetchTimeout time.Duration` field to `config.Config`. Parse from `DEPHEALTH_FETCH_TIMEOUT` env var (float seconds, default 5). Reuse existing `parseFloat` pattern from `CheckInterval`/`Timeout` parsing. Return error if value is negative.
  - **Modifies**:
    - `internal/config/config.go` (add field + parsing)
    - `internal/config/config_test.go` (add test for FetchTimeout)

### Completion criteria

- [x] All subtasks completed (1.1, 1.2)
- [x] `go test ./internal/config/...` passes
- [x] `go build ./...` compiles without errors
- [x] `dh.HealthDetails()` is callable (verified by compilation)

---

## Phase 2: Detail response handler

**Dependencies**: Phase 1
**Status**: Completed

### Description

Extend `handleRoot` to support `?detail=true` query parameter. When present, use `HealthDetails()` instead of `Health()` to build enriched response. Define server-side response types that wrap SDK's `EndpointStatus` with an additional `response` field for recursive fetch.

### Subtasks

- [x] **2.1 Define response types and extend server interface**
  - **Dependencies**: None
  - **Description**: Add types and extend HealthProvider interface:
    ```go
    // Extend the interface used by the server
    type HealthChecker interface {
        Health() map[string]bool
        HealthDetails() map[string]dephealth.EndpointStatus
    }

    type DetailStatusResponse struct {
        Name         string                         `json:"name"`
        PodName      string                         `json:"podName"`
        Namespace    string                         `json:"namespace"`
        Dependencies map[string]*DependencyDetail   `json:"dependencies"`
    }

    type DependencyDetail struct {
        dephealth.EndpointStatus                      // embed all 11 SDK fields
        Response *json.RawMessage `json:"response,omitempty"`
    }
    ```
    Note: Embedding `EndpointStatus` leverages its `MarshalJSON()` for free serialization of latency_ms, last_checked_at, labels. We need a custom `MarshalJSON` on `DependencyDetail` that merges SDK fields + response.
  - **Modifies**:
    - `internal/server/server.go` (types + interface)

- [x] **2.2 Implement detail handler logic**
  - **Dependencies**: 2.1
  - **Description**: In `handleRoot`, check `r.URL.Query().Get("detail") == "true"`. If true:
    1. Call `dh.HealthDetails()` → `map[string]EndpointStatus`
    2. Parse `depth` query param (default 1, min 0, max 10)
    3. Build `DetailStatusResponse` mapping each endpoint to `DependencyDetail`
    4. For now, set `Response: nil` (Phase 3 adds recursive fetch)
    5. Return JSON with `Content-Type: application/json`
    Add `fetchTimeout time.Duration` field to Server struct. Update `New()` constructor signature.
  - **Modifies**:
    - `internal/server/server.go` (handleRoot extension, Server struct, New signature)
    - `main.go` (pass `cfg.FetchTimeout` to `server.New()`)

- [x] **2.3 Add tests for detail response**
  - **Dependencies**: 2.2
  - **Description**: Create `internal/server/server_test.go`:
    - Test `GET /` returns old `StatusResponse` format (backward compatibility)
    - Test `GET /?detail=true` returns `DetailStatusResponse` with all 11 SDK fields + response
    - Test `depth` param parsing (default=1, explicit=0, explicit=3, invalid=default)
    - Test `detail=false` or `detail=abc` returns old format
    - Use mock implementing `HealthChecker` interface
  - **Creates**:
    - `internal/server/server_test.go`

### Completion criteria

- [x] All subtasks completed (2.1, 2.2, 2.3)
- [x] `GET /` returns old format (backward compatible)
- [x] `GET /?detail=true` returns `dependencies` map with all EndpointStatus fields + `response`
- [x] `go test ./internal/server/...` passes

---

## Phase 3: Recursive HTTP fetch

**Dependencies**: Phase 2
**Status**: Completed

### Description

For HTTP-type dependencies, when `depth > 0`, make an HTTP request to the dependency's URL and include the response JSON in the `response` field. Pass `?detail=true&depth=N-1` to support recursive chain visibility.

### Subtasks

- [x] **3.1 Implement HTTP fetch logic**
  - **Dependencies**: None
  - **Description**: Create `internal/server/fetch.go` with function:
    ```go
    func fetchHTTPResponse(ctx context.Context, host, port string, depth int, timeout time.Duration) *json.RawMessage
    ```
    - Constructs URL: `http://<host>:<port>/?detail=true&depth=<depth-1>`
    - Uses `http.Client` with timeout from context/config
    - Returns response body as `*json.RawMessage` on success
    - Returns `nil` on any error (timeout, connection refused, non-200, invalid JSON)
    - No error propagation — errors are visible through the parent endpoint's `status`/`detail` fields
  - **Creates**:
    - `internal/server/fetch.go`

- [x] **3.2 Integrate fetch into detail handler**
  - **Dependencies**: 3.1, Phase 2
  - **Description**: In detail handler, after building `DependencyDetail` entries:
    1. Collect HTTP-type dependencies where `depth > 0`
    2. Launch parallel goroutines (using `sync.WaitGroup` or `errgroup`) for each
    3. Use `host` and `port` from `EndpointStatus` directly (no URL mapping needed!)
    4. Set `DependencyDetail.Response` with fetch result
    5. Respect `fetchTimeout` as overall timeout for all parallel fetches
  - **Modifies**:
    - `internal/server/server.go` (detail handler integration)

- [x] **3.3 Add tests for HTTP fetch**
  - **Dependencies**: 3.1, 3.2
  - **Description**: Create `internal/server/fetch_test.go`:
    - Test successful fetch from httptest server simulating uniproxy
    - Test depth propagation (depth=2 passes `?depth=1` to downstream)
    - Test timeout handling (slow server → nil response)
    - Test unreachable host → nil response
    - Test non-JSON response → nil response
    - Test depth=0 → no fetch attempted
  - **Creates**:
    - `internal/server/fetch_test.go`

### Completion criteria

- [x] All subtasks completed (3.1, 3.2, 3.3)
- [x] HTTP deps with depth>0 include `response` with target's JSON
- [x] HTTP deps with depth=0 have `response: null`
- [x] Non-HTTP deps never have `response` field
- [x] Unreachable HTTP deps have `response: null`
- [x] Parallel fetch respects `DEPHEALTH_FETCH_TIMEOUT`
- [x] `go test ./internal/server/...` passes

---

## Phase 4: Build and test

**Dependencies**: Phase 3
**Status**: Completed

### Description

Build Docker image and run integration test with real dependencies.

### Subtasks

- [x] **4.1 Run unit tests with coverage**
  - **Dependencies**: None
  - **Description**: `go test -cover ./...` — all tests pass with reasonable coverage. Target: >80% for config, >70% for server.
  - **Creates**:
    - Test results

- [x] **4.2 Build Docker image**
  - **Dependencies**: 4.1
  - **Description**: Build `uniproxy:dev` image using existing Dockerfile (Harbor proxy base images). Verify image starts and responds to `GET /` and `GET /?detail=true`.
  - **Creates**:
    - Docker image `uniproxy:dev`

- [x] **4.3 Integration test with two instances**
  - **Dependencies**: 4.2
  - **Description**: Run two uniproxy containers in Docker network:
    - **Instance B**: standalone, with HTTP dependency (e.g., httpbin)
    - **Instance A**: depends on Instance B (HTTP type)
    - Test scenarios:
      1. `GET /` on A → old format, backward compatible
      2. `GET /?detail=true` on A → shows B's status with `response` containing B's detailed health
      3. `GET /?detail=true&depth=0` on A → dependencies shown but no `response` field
      4. Stop B → `GET /?detail=true` on A → B's `status` = "connection_error", `response` = null
  - **Creates**:
    - Test results

### Completion criteria

- [x] All subtasks completed (4.1, 4.2, 4.3)
- [x] All unit tests pass with coverage targets (config: 88.3%, server: 92.3%)
- [x] Docker image builds successfully (uniproxy:0.4.1)
- [x] Integration test validates recursive fetch chain
- [x] No regressions in existing functionality

---

## Phase 5: Helm chart update

**Dependencies**: Phase 4
**Status**: Completed

### Description

Update Helm chart with new env var, bump versions, validate rendering.

### Subtasks

- [x] **5.1 Add DEPHEALTH_FETCH_TIMEOUT to Helm chart**
  - **Dependencies**: None
  - **Description**: Add `fetchTimeout` to `values.yaml` (default 5). Add env var to deployment template. Comment explaining the parameter.
  - **Modifies**:
    - `deploy/helm/uniproxy/values.yaml`
    - `deploy/helm/uniproxy/templates/deployment.yaml`

- [x] **5.2 Update instance examples**
  - **Dependencies**: 5.1
  - **Description**: Add `fetchTimeout` to instance example files with a comment showing usage of `?detail=true&depth=N`.
  - **Modifies**:
    - `deploy/helm/uniproxy/instances/*.yaml`

- [x] **5.3 Bump versions**
  - **Dependencies**: 5.1
  - **Description**: Bump Chart.yaml `version` and `appVersion` to 0.5.0.
  - **Modifies**:
    - `deploy/helm/uniproxy/Chart.yaml`

- [x] **5.4 Helm lint and template test**
  - **Dependencies**: 5.1, 5.2, 5.3
  - **Description**: Run `helm lint` and `helm template` to verify chart renders correctly with new env var.
  - **Creates**:
    - Lint results

### Completion criteria

- [x] All subtasks completed (5.1, 5.2, 5.3, 5.4)
- [x] `helm lint` passes
- [x] `helm template` renders DEPHEALTH_FETCH_TIMEOUT correctly
- [x] Chart version bumped to 0.4.1 (synced with SDK version)

---

## Implementation Workflow (Execution Order)

```
Phase 1 ─── 1.1 SDK upgrade ──────────────── ▶ go build ./...
         └── 1.2 FetchTimeout config ─────── ▶ go test ./internal/config/...
                    │
Phase 2 ─── 2.1 Types + interface ───────────┐
         └── 2.2 Detail handler logic ───────┤▶ go test ./internal/server/...
         └── 2.3 Server tests ───────────────┘
                    │
Phase 3 ─── 3.1 fetch.go ───────────────────┐
         └── 3.2 Integrate fetch ────────────┤▶ go test ./internal/server/...
         └── 3.3 Fetch tests ────────────────┘
                    │
Phase 4 ─── 4.1 Unit tests + coverage ──────┐
         └── 4.2 Docker build ───────────────┤▶ Integration validation
         └── 4.3 Integration test ───────────┘
                    │
Phase 5 ─── 5.1 values.yaml + template ─────┐
         └── 5.2 Instance examples ──────────┤▶ helm lint + helm template
         └── 5.3 Version bump ───────────────┤
         └── 5.4 Lint + template test ───────┘
```

## Notes

- SDK's `EndpointStatus` has custom `MarshalJSON()` — we leverage it but need our own marshaler for `DependencyDetail` to merge SDK fields + `response`
- `EndpointStatus.Host` and `.Port` fields eliminate the need for a separate URL mapping (simplification vs. original plan)
- SDK includes UNKNOWN status (nil Healthy) for endpoints not yet checked — this correctly appears in detail response
- `response` field uses `*json.RawMessage` for transparent JSON pass-through
- Depth parameter prevents infinite recursion in circular dependency graphs (A → B → A)
- All HTTP fetches are parallel with shared timeout from `DEPHEALTH_FETCH_TIMEOUT`
