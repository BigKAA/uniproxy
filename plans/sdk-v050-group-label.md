# Plan: Update to dephealth SDK v0.5.0 — `group` label support

## Metadata

- **Plan version**: 1.0.0
- **Created**: 2026-02-18
- **Last updated**: 2026-02-18
- **Status**: Complete
- **Target release**: uniproxy v0.6.0

---

## Version History

- **v1.0.0** (2026-02-18): Initial plan

---

## Current Status

- **Active phase**: Complete
- **Active subtask**: —
- **Last updated**: 2026-02-18
- **Note**: All 5 phases complete. Smoke test passed. Ready for git tag & release.

---

## Summary

dephealth SDK v0.5.0 introduces a **mandatory `group` parameter** to `dephealth.New()` and adds
the `group` label to all 4 Prometheus metrics. This is a **breaking change** in the SDK API.

### SDK v0.5.0 Changes (reference)

| Change | Details |
|--------|---------|
| `dephealth.New()` signature | `New(name, opts...)` → `New(name, group, opts...)` |
| New required label | `group` added to all 4 metrics |
| Label order | `name, group, dependency, type, host, port, critical` |
| `Scheduler.Add()` | Removed (not used by uniproxy — no impact) |
| `group` validation | 1-63 chars, lowercase + digits + hyphens |
| `reservedLabels` | `group` added |

### Metric example (before → after)

```
# v0.4.2
app_dependency_health{name="my-proxy",dependency="pg",type="postgres",host="pg.svc",port="5432",critical="yes"} 1

# v0.5.0
app_dependency_health{name="my-proxy",group="backend",dependency="pg",type="postgres",host="pg.svc",port="5432",critical="yes"} 1
```

---

## Table of Contents

- [ ] [Phase 1: Config layer — add `group` field](#phase-1-config-layer--add-group-field)
- [ ] [Phase 2: SDK integration — upgrade and wire `group`](#phase-2-sdk-integration--upgrade-and-wire-group)
- [ ] [Phase 3: Helm charts — add `group` to both charts](#phase-3-helm-charts--add-group-to-both-charts)
- [ ] [Phase 4: Documentation — full update](#phase-4-documentation--full-update)
- [ ] [Phase 5: Build, test, release](#phase-5-build-test-release)

---

## Phase 1: Config layer — add `group` field

**Dependencies**: None
**Status**: Pending

### Description

Add the `Group` field to `Config` struct with full env var + YAML support.
The field follows exactly the same pattern as the existing `Name` field:
required, env overrides YAML, validated on startup.

### Design

#### 1.1 `Config` struct change

**File**: `internal/config/config.go`, line 14

```go
// BEFORE
Config struct {
    Name          string
    ListenAddr    string
    ...

// AFTER
Config struct {
    Name          string
    Group         string    // ← NEW: logical group for metrics label
    ListenAddr    string
    ...
```

#### 1.2 `yamlConfig` struct change

**File**: `internal/config/yaml.go`, line 13

```go
// BEFORE
yamlConfig struct {
    Name          string         `yaml:"name"`
    ListenAddr    string         `yaml:"listenAddr"`
    ...

// AFTER
yamlConfig struct {
    Name          string         `yaml:"name"`
    Group         string         `yaml:"group"`    // ← NEW
    ListenAddr    string         `yaml:"listenAddr"`
    ...
```

#### 1.3 `convertYAMLToConfig` change

**File**: `internal/config/yaml.go`, line 103

```go
// Add Group to struct initialization:
cfg := &Config{
    Name:       yc.Name,
    Group:      yc.Group,    // ← NEW
    ListenAddr: yc.ListenAddr,
}
```

#### 1.4 `applyEnvOverrides` change

**File**: `internal/config/config.go`, line 179

Insert after the `DEPHEALTH_NAME` block (after line ~193):

```go
// Application group.
if v := os.Getenv("DEPHEALTH_GROUP"); v != "" {
    cfg.Group = v
}
```

#### 1.5 `validate` change

**File**: `internal/config/config.go`, line 313

Insert after the `Name` validation (after line ~315):

```go
if cfg.Group == "" {
    return fmt.Errorf("DEPHEALTH_GROUP is required")
}
```

### Subtasks

- [ ] **1.1 Add `Group` to Config and yamlConfig structs**
  - **Dependencies**: None
  - **Description**: Add `Group string` field to both structs, update `convertYAMLToConfig`
  - **Modifies**:
    - `internal/config/config.go` (Config struct)
    - `internal/config/yaml.go` (yamlConfig struct, convertYAMLToConfig)

- [ ] **1.2 Add `DEPHEALTH_GROUP` env override and validation**
  - **Dependencies**: 1.1
  - **Description**: Handle DEPHEALTH_GROUP in `applyEnvOverrides`, validate in `validate()`
  - **Modifies**:
    - `internal/config/config.go` (applyEnvOverrides, validate)

- [ ] **1.3 Update config tests**
  - **Dependencies**: 1.2
  - **Description**: Add tests for Group parsing (env, YAML, missing → error). Update all
    existing tests that call `Load()` — they must now set `DEPHEALTH_GROUP` env var.
  - **Modifies**:
    - `internal/config/config_test.go`

### Completion Criteria Phase 1

- [ ] All subtasks completed (1.1, 1.2, 1.3)
- [ ] `go test ./internal/config/...` passes
- [ ] Missing `DEPHEALTH_GROUP` → clear error message
- [ ] Env override of YAML group works correctly

---

## Phase 2: SDK integration — upgrade and wire `group`

**Dependencies**: Phase 1
**Status**: Pending

### Description

Update `go.mod` to SDK v0.5.0, change `dephealth.New()` call in `main.go` to pass `cfg.Group`.

### Design

#### 2.1 `go.mod` update

```
github.com/BigKAA/topologymetrics/sdk-go v0.4.2
→
github.com/BigKAA/topologymetrics/sdk-go v0.5.0
```

#### 2.2 `main.go` change

**File**: `main.go`, line 52

```go
// BEFORE
dh, err := dephealth.New(cfg.Name, opts...)

// AFTER
dh, err := dephealth.New(cfg.Name, cfg.Group, opts...)
```

#### 2.3 `main.go` log statement update

**File**: `main.go`, line 35

```go
// BEFORE
slog.Info("config loaded",
    "name", cfg.Name,
    "listen", cfg.ListenAddr,
    ...

// AFTER
slog.Info("config loaded",
    "name", cfg.Name,
    "group", cfg.Group,    // ← NEW
    "listen", cfg.ListenAddr,
    ...
```

### Subtasks

- [ ] **2.1 Upgrade SDK dependency**
  - **Dependencies**: None
  - **Description**: `go get github.com/BigKAA/topologymetrics/sdk-go@v0.5.0` + `go mod tidy`
  - **Modifies**:
    - `go.mod`
    - `go.sum`

- [ ] **2.2 Wire `cfg.Group` in main.go**
  - **Dependencies**: 2.1, Phase 1
  - **Description**: Pass `cfg.Group` to `dephealth.New()`, add to startup log
  - **Modifies**:
    - `main.go`

- [ ] **2.3 Update server tests if needed**
  - **Dependencies**: 2.2
  - **Description**: If `server_test.go` creates mock `dephealth` instances, update those calls
    to match the new SDK API. Verify `go test ./...` passes entirely.
  - **Modifies**:
    - `internal/server/server_test.go` (if applicable)

### Completion Criteria Phase 2

- [ ] All subtasks completed (2.1, 2.2, 2.3)
- [ ] `go test ./...` passes (all packages)
- [ ] `go vet ./...` clean
- [ ] No compilation errors

---

## Phase 3: Helm charts — add `group` to both charts

**Dependencies**: Phase 2
**Status**: Pending

### Description

Add `group` configuration to both Helm charts. The pattern mirrors how `name` (or
`DEPHEALTH_NAME`) is already handled in each chart.

### Design

#### Multi-instance chart (`deploy/helm/uniproxy/`)

**values.yaml**: No global `group` — it's per-instance (same as `name`).
Instance files (e.g., `instances/ns1-homelab.yaml`) will add `group` per instance.

**deployment.yml**: Add after `DEPHEALTH_NAME` env var (line ~36):

```yaml
- name: DEPHEALTH_GROUP
  value: {{ .group | quote }}
```

**Chart.yaml**: Bump version `0.5.0` → `0.6.0`, appVersion `0.5.0` → `0.6.0`.

#### Single-instance chart (`charts/uniproxy/`)

**values.yaml**: Add `group` field next to `name` (line ~22):

```yaml
config:
  # DEPHEALTH_NAME (required). Defaults to release name if empty.
  name: ""
  # DEPHEALTH_GROUP (required). Logical group for metrics label.
  group: ""
```

**deployment.yaml**: Add after `DEPHEALTH_NAME` (line ~43):

```yaml
- name: DEPHEALTH_GROUP
  value: {{ .Values.config.group | quote }}
```

**Chart.yaml**: Bump version `0.1.0` → `0.2.0`, appVersion `0.5.0` → `0.6.0`.

### Subtasks

- [ ] **3.1 Update multi-instance chart**
  - **Dependencies**: None
  - **Description**: Add DEPHEALTH_GROUP to deployment.yml, bump Chart.yaml version
  - **Modifies**:
    - `deploy/helm/uniproxy/templates/deployment.yml`
    - `deploy/helm/uniproxy/Chart.yaml`

- [ ] **3.2 Update single-instance chart**
  - **Dependencies**: None
  - **Description**: Add `group` to values.yaml, DEPHEALTH_GROUP to deployment.yaml, bump Chart.yaml
  - **Modifies**:
    - `charts/uniproxy/values.yaml`
    - `charts/uniproxy/templates/deployment.yaml`
    - `charts/uniproxy/Chart.yaml`

- [ ] **3.3 Update instance files**
  - **Dependencies**: 3.1
  - **Description**: Add `group` field to existing instance files under
    `deploy/helm/uniproxy/instances/`
  - **Modifies**:
    - `deploy/helm/uniproxy/instances/*.yaml`

- [ ] **3.4 Lint both charts**
  - **Dependencies**: 3.1, 3.2
  - **Description**: `helm lint` for both charts, `helm template` smoke test
  - **Creates**: N/A (validation only)

### Completion Criteria Phase 3

- [ ] All subtasks completed (3.1, 3.2, 3.3, 3.4)
- [ ] `helm lint ./deploy/helm/uniproxy` passes
- [ ] `helm lint ./charts/uniproxy` passes
- [ ] `helm template` renders DEPHEALTH_GROUP in both charts

---

## Phase 4: Documentation — full update

**Dependencies**: Phase 2
**Status**: Pending

### Description

Update all documentation to reflect the new `group` field: configuration examples,
metric label lists, YAML examples, and compatibility notes.

### Design

#### Key changes across all docs:

1. **Configuration section**: Add `DEPHEALTH_GROUP` / `group` to env var table and YAML example
2. **Metrics section**: Update "Base labels" from
   `name, dependency, type, host, port, critical` →
   `name, group, dependency, type, host, port, critical`
3. **Metric examples**: Add `group="..."` label to all sample metrics
4. **Quick start examples**: Add `-e DEPHEALTH_GROUP=...` to docker run commands
5. **Compatibility note**: Document SDK v0.5.0 breaking changes

### Subtasks

- [ ] **4.1 Update README.md (EN)**
  - **Dependencies**: None
  - **Description**: Update configuration tables, metric labels (line 464), examples,
    add DEPHEALTH_GROUP to env var documentation, add compatibility note for v0.6.0
  - **Modifies**:
    - `README.md`

- [ ] **4.2 Update README.ru.md (RU)**
  - **Dependencies**: 4.1 (use EN as reference)
  - **Description**: Mirror all changes from README.md in Russian
  - **Modifies**:
    - `README.ru.md`

- [ ] **4.3 Update examples/config.yaml**
  - **Dependencies**: None
  - **Description**: Add `group: my-group` field after `name:` (line 11),
    update docker run example to v0.6.0, add comment explaining group purpose
  - **Modifies**:
    - `examples/config.yaml`

- [ ] **4.4 Update examples/docker-compose.yaml**
  - **Dependencies**: None
  - **Description**: Add `DEPHEALTH_GROUP` env var, update image tag to v0.6.0
  - **Modifies**:
    - `examples/docker-compose.yaml`

- [ ] **4.5 Update docs/use-cases.md + docs/use-cases.ru.md**
  - **Dependencies**: None
  - **Description**: Add `group` label to any metric examples in use-case docs
  - **Modifies**:
    - `docs/use-cases.md`
    - `docs/use-cases.ru.md`

- [ ] **4.6 Update CLAUDE.md**
  - **Dependencies**: None
  - **Description**: Update env var list, metrics labels section, docker run examples,
    architecture description. Add `DEPHEALTH_GROUP` to list of required vars.
  - **Modifies**:
    - `CLAUDE.md`

### Completion Criteria Phase 4

- [ ] All subtasks completed (4.1–4.6)
- [ ] All documentation references correct label set `name, group, dependency, ...`
- [ ] All docker run examples include DEPHEALTH_GROUP
- [ ] YAML example includes `group:` field
- [ ] Compatibility note present in README EN/RU

---

## Phase 5: Build, test, release

**Dependencies**: Phase 1, Phase 2, Phase 3, Phase 4
**Status**: Pending

### Description

Build Docker image, run full test suite, create git tag, and publish release.

### Subtasks

- [ ] **5.1 Run full test suite**
  - **Dependencies**: None
  - **Description**: `go test -cover ./...` — all packages, verify coverage levels
  - **Creates**: Test results

- [ ] **5.2 Build Docker image**
  - **Dependencies**: 5.1
  - **Description**: Multi-arch build for amd64+arm64:
    ```bash
    docker buildx build --builder multiarch --platform linux/amd64,linux/arm64 \
      -t harbor.kryukov.lan/library/uniproxy:v0.6.0 \
      -t harbor.kryukov.lan/library/uniproxy:latest --push .
    ```
  - **Creates**: Docker image `harbor.kryukov.lan/library/uniproxy:v0.6.0`

- [ ] **5.3 Docker smoke test**
  - **Dependencies**: 5.2
  - **Description**: Run container with DEPHEALTH_GROUP set, verify:
    - Startup log includes `group` field
    - `/metrics` output contains `group` label
    - Missing DEPHEALTH_GROUP → error
  - **Creates**: N/A (validation only)

- [ ] **5.4 Git tag and release**
  - **Dependencies**: 5.3
  - **Description**: Create `v0.6.0` tag, push, create GitHub release with changelog
  - **Creates**: Git tag `v0.6.0`, GitHub release

### Completion Criteria Phase 5

- [ ] All subtasks completed (5.1–5.4)
- [ ] `go test -cover ./...` passes with coverage >= v0.5.0 levels
- [ ] Docker image built and pushed (multi-arch)
- [ ] Smoke test passed (group in metrics, group in logs, error on missing)
- [ ] Git tag v0.6.0 created
- [ ] GitHub release published

---

## Notes

### Breaking Changes (for users migrating from v0.5.0)

1. **`DEPHEALTH_GROUP` is now required** — application will not start without it
2. **All Prometheus metrics** now include `group` label — PromQL queries, dashboards, and
   alert rules targeting uniproxy metrics must be updated
3. **YAML config** now supports `group:` field (env var `DEPHEALTH_GROUP` overrides it)
4. **SDK v0.5.0** removed `Scheduler.Add()` — no impact on uniproxy users

### Migration Guide

```bash
# v0.5.0 → v0.6.0: Add DEPHEALTH_GROUP env var
docker run -e DEPHEALTH_NAME=my-proxy \
           -e DEPHEALTH_GROUP=my-team \   # ← NEW (required)
           -e DEPHEALTH_DEPS="backend:http" \
           ...
```

### Files Changed Summary

| Phase | Files |
|-------|-------|
| Phase 1 | `internal/config/config.go`, `internal/config/yaml.go`, `internal/config/config_test.go` |
| Phase 2 | `go.mod`, `go.sum`, `main.go`, `internal/server/server_test.go` (if needed) |
| Phase 3 | `deploy/helm/uniproxy/{Chart.yaml,templates/deployment.yml}`, `charts/uniproxy/{Chart.yaml,values.yaml,templates/deployment.yaml}`, `deploy/helm/uniproxy/instances/*.yaml` |
| Phase 4 | `README.md`, `README.ru.md`, `examples/config.yaml`, `examples/docker-compose.yaml`, `docs/use-cases.md`, `docs/use-cases.ru.md`, `CLAUDE.md` |
| Phase 5 | Docker image, git tag, GitHub release |

---

**Plan ready for execution.**
