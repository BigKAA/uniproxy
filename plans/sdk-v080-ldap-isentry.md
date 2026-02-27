# Plan: SDK v0.8.0 Upgrade — LDAP Checker & isentry Label

## Metadata

- **Plan version**: 1.1.0
- **Created**: 2026-02-27
- **Last updated**: 2026-02-27
- **Status**: Pending

---

## Version History

- **v1.1.0** (2026-02-27): Added Phase 5 — Documentation
- **v1.0.0** (2026-02-27): Initial plan

---

## Current Status

- **Active phase**: Phase 4
- **Active subtask**: 4.1
- **Last updated**: 2026-02-27
- **Note**: Phase 3 completed — Helm chart updated with isentry label and LDAP env vars support (inline + Secret-based)

---

## Table of Contents

- [x] [Phase 1: SDK Upgrade & isentry Label](#phase-1-sdk-upgrade--isentry-label)
- [x] [Phase 2: LDAP Checker Support](#phase-2-ldap-checker-support)
- [x] [Phase 3: Helm Chart Update](#phase-3-helm-chart-update)
- [ ] [Phase 4: Build & Test in Docker/K8s](#phase-4-build--test-in-dockerk8s)
- [ ] [Phase 5: Documentation](#phase-5-documentation)

---

## Phase 1: SDK Upgrade & isentry Label

**Dependencies**: None
**Status**: Completed

### Description

Upgrade dephealth SDK from v0.6.0 to v0.8.0 and implement the optional `isentry` label.
The `isentry` label is a global flag: when `DEPHEALTH_ISENTRY=yes` is set, `isentry=yes`
is added to all dependency metrics via `dephealth.WithLabel("isentry", "yes")`.

### Design

#### isentry in Config

Add field `IsEntry bool` to `Config` struct. Parse from `DEPHEALTH_ISENTRY` env var
(same boolean parsing: yes/no/true/false/1/0). If not set — default `false`.

In `main.go:buildDependencyOption()`, when `cfg.IsEntry` is true, append
`dephealth.WithLabel("isentry", "yes")` to every dependency's option list.

The `isentry` value is always `"yes"` when enabled — it's a flag, not a freeform value.

#### YAML support

Add `isEntry` field to `yamlConfig` and map to `Config.IsEntry`.

#### Data Flow

```
DEPHEALTH_ISENTRY=yes
     │
     ▼
config.Load() → Config{IsEntry: true}
     │
     ▼
main.buildDependencyOption() for each dep:
  depOpts = append(depOpts, dephealth.WithLabel("isentry", "yes"))
     │
     ▼
SDK registers label on all metrics:
  app_dependency_health{..., isentry="yes"} 1
```

### Subtasks

- [ ] **1.1 Upgrade SDK in go.mod**
  - **Dependencies**: None
  - **Description**: Update `github.com/BigKAA/topologymetrics/sdk-go` from v0.6.0 to v0.8.0 in go.mod. Run `go mod tidy` to update go.sum. Verify existing code compiles without changes.
  - **Modifies**:
    - `go.mod`
    - `go.sum`

- [ ] **1.2 Add isentry to config parsing**
  - **Dependencies**: 1.1
  - **Description**: Add `IsEntry bool` field to `Config` struct. In `applyEnvOverrides()`, read `DEPHEALTH_ISENTRY` env var using existing `parseBool()`. Add `IsEntry` field to `yamlConfig` and `convertYAMLToConfig()`.
  - **Modifies**:
    - `internal/config/config.go` — add `IsEntry bool` to `Config`, parse in `applyEnvOverrides()`
    - `internal/config/yaml.go` — add `IsEntry *bool` to `yamlConfig`, map in `convertYAMLToConfig()`

- [ ] **1.3 Apply isentry label in main.go**
  - **Dependencies**: 1.2
  - **Description**: In `buildDependencyOption()`, if `cfg.IsEntry` is true, append `dephealth.WithLabel("isentry", "yes")` to dependency options. Pass `cfg` (or `cfg.IsEntry`) to `buildDependencyOption` — currently it receives only `config.Dependency`, need to also pass the isentry flag.
  - **Modifies**:
    - `main.go` — change `buildDependencyOption()` signature to accept `isEntry bool`, add `WithLabel` call
  - **Design detail**: Change signature from `buildDependencyOption(dep config.Dependency)` to `buildDependencyOption(dep config.Dependency, isEntry bool)`. In `buildOptions`, pass `cfg.IsEntry` to each call.

- [ ] **1.4 Unit tests for isentry**
  - **Dependencies**: 1.2
  - **Description**: Add tests to `config_test.go` for isentry parsing: (a) DEPHEALTH_ISENTRY=yes → IsEntry=true, (b) DEPHEALTH_ISENTRY=no → IsEntry=false, (c) not set → IsEntry=false, (d) invalid value → error.
  - **Modifies**:
    - `internal/config/config_test.go`

### Completion Criteria Phase 1

- [ ] All subtasks completed (1.1–1.4)
- [ ] `go test ./...` passes
- [ ] `go build` succeeds with SDK v0.8.0
- [ ] isentry label correctly parsed and applied

---

## Phase 2: LDAP Checker Support

**Dependencies**: Phase 1
**Status**: Pending

### Description

Add LDAP as a supported dependency type. Users configure LDAP dependencies via env vars
following the same pattern as other types. All 4 check methods supported:
`anonymous_bind`, `simple_bind`, `root_dse` (default), `search`.

### Design

#### New Fields in Dependency Struct

```go
// LDAP-specific.
LDAPCheckMethod   string  // "anonymous_bind", "simple_bind", "root_dse", "search"
LDAPBindDN        string  // DN for simple_bind
LDAPBindPassword  string  // password for simple_bind
LDAPBaseDN        string  // base DN for search
LDAPSearchFilter  string  // LDAP filter for search (default: (objectClass=*))
LDAPSearchScope   string  // "base", "one", "sub" (default: base)
LDAPStartTLS      *bool   // StartTLS for ldap:// connections
LDAPTLSSkipVerify *bool   // skip TLS certificate verification
```

#### Environment Variables

For dependency `myldap:ldap`:

| Variable | Required | Description |
|---|---|---|
| `DEPHEALTH_MYLDAP_URL` | URL or HOST+PORT | `ldap://host:389` or `ldaps://host:636` |
| `DEPHEALTH_MYLDAP_HOST` + `_PORT` | URL or HOST+PORT | Alternative to URL |
| `DEPHEALTH_MYLDAP_CRITICAL` | Yes | yes/no |
| `DEPHEALTH_MYLDAP_LDAP_CHECK_METHOD` | No (default: root_dse) | Check method |
| `DEPHEALTH_MYLDAP_LDAP_BIND_DN` | For simple_bind | Bind DN |
| `DEPHEALTH_MYLDAP_LDAP_BIND_PASSWORD` | For simple_bind | Bind password (supports _FILE) |
| `DEPHEALTH_MYLDAP_LDAP_BASE_DN` | For search | Base DN |
| `DEPHEALTH_MYLDAP_LDAP_SEARCH_FILTER` | No | LDAP filter |
| `DEPHEALTH_MYLDAP_LDAP_SEARCH_SCOPE` | No | base/one/sub |
| `DEPHEALTH_MYLDAP_LDAP_START_TLS` | No | true/false |
| `DEPHEALTH_MYLDAP_LDAP_TLS_SKIP_VERIFY` | No | true/false |

#### SDK Mapping

```go
// In buildDependencyOption, case "ldap":
dephealth.WithLDAPCheckMethod(dep.LDAPCheckMethod)
dephealth.WithLDAPBindDN(dep.LDAPBindDN)
dephealth.WithLDAPBindPassword(dep.LDAPBindPassword)
dephealth.WithLDAPBaseDN(dep.LDAPBaseDN)
dephealth.WithLDAPSearchFilter(dep.LDAPSearchFilter)
dephealth.WithLDAPSearchScope(dep.LDAPSearchScope)
dephealth.WithLDAPStartTLS(bool)
dephealth.WithLDAPTLSSkipVerify(bool)

// Factory:
dephealth.LDAP(dep.Name, depOpts...)
```

### Subtasks

- [ ] **2.1 Add LDAP fields to Dependency and parsing**
  - **Dependencies**: None
  - **Description**: Add LDAP fields to `Dependency` struct. Add `"ldap"` to allowed types in `parseDeps()`. Add `case "ldap":` to `parseSingleDep()` that reads all LDAP-specific env vars. Use `resolveSecret()` for `LDAP_BIND_PASSWORD` to support `_FILE` variant.
  - **Modifies**:
    - `internal/config/config.go` — `Dependency` struct, `parseDeps()` switch, `parseSingleDep()` switch

- [ ] **2.2 Add LDAP fields to YAML config**
  - **Dependencies**: 2.1
  - **Description**: Add LDAP fields to `yamlDep` struct with proper YAML tags. Map them in `convertYAMLDep()`.
  - **Modifies**:
    - `internal/config/yaml.go` — `yamlDep` struct, `convertYAMLDep()`

- [ ] **2.3 Add LDAP to buildDependencyOption in main.go**
  - **Dependencies**: 2.1
  - **Description**: Add `case "ldap":` to both switch statements in `buildDependencyOption()`. Map LDAP config fields to SDK dependency options. Add `dephealth.LDAP()` factory call.
  - **Modifies**:
    - `main.go` — `buildDependencyOption()`

- [ ] **2.4 Unit tests for LDAP config parsing**
  - **Dependencies**: 2.1, 2.2
  - **Description**: Add tests for LDAP dependency parsing: (a) minimal config (URL + critical + root_dse default), (b) simple_bind with bind_dn/password, (c) search with base_dn/filter/scope, (d) HOST+PORT instead of URL, (e) StartTLS and TLSSkipVerify, (f) _FILE variant for bind password, (g) invalid check method → error, (h) invalid search scope → error.
  - **Modifies**:
    - `internal/config/config_test.go`
    - `internal/config/yaml_test.go`

### Completion Criteria Phase 2

- [ ] All subtasks completed (2.1–2.4)
- [ ] `go test ./...` passes
- [ ] LDAP type accepted in DEPHEALTH_DEPS
- [ ] All LDAP env vars parsed correctly
- [ ] YAML config supports LDAP dependencies

---

## Phase 3: Helm Chart Update

**Dependencies**: Phase 2
**Status**: Completed

### Description

Update Helm chart to support LDAP dependencies and the isentry label.

### Design

#### values.yaml Changes

Add new top-level field:
```yaml
# Optional label added to all dependency metrics.
# When set to "yes", isentry=yes label is added to all metrics.
# Useful for marking entry-point applications in topology visualization.
isentry: ""
```

No changes needed for LDAP in values.yaml — LDAP is configured per-connection
in instance files, and the deployment template needs new env var mappings.

#### deployment.yml Changes

1. Add `DEPHEALTH_ISENTRY` env var (conditional on `$.Values.isentry`):
```yaml
{{- if $.Values.isentry }}
- name: DEPHEALTH_ISENTRY
  value: {{ $.Values.isentry | quote }}
{{- end }}
```

2. Add LDAP-specific env vars in the connection loop (after existing amqpURL block):
```yaml
{{- if .ldapCheckMethod }}
- name: {{ $envPrefix }}_LDAP_CHECK_METHOD
  value: {{ .ldapCheckMethod | quote }}
{{- end }}
{{- if .ldapBindDN }}
- name: {{ $envPrefix }}_LDAP_BIND_DN
  value: {{ .ldapBindDN | quote }}
{{- end }}
... (all LDAP fields)
```

3. Support LDAP bind password from Kubernetes Secret:
```yaml
{{- if and .auth.existingSecret .auth.ldapBindPasswordKey }}
- name: {{ $envPrefix }}_LDAP_BIND_PASSWORD
  valueFrom:
    secretKeyRef:
      name: {{ .auth.existingSecret }}
      key: {{ .auth.ldapBindPasswordKey }}
{{- end }}
```

### Subtasks

- [ ] **3.1 Add isentry to values.yaml and deployment template**
  - **Dependencies**: None
  - **Description**: Add `isentry: ""` to values.yaml with documentation comment. Add DEPHEALTH_ISENTRY env var to deployment.yml (inside the instance range, before the connections block).
  - **Modifies**:
    - `deploy/helm/uniproxy/values.yaml`
    - `deploy/helm/uniproxy/templates/deployment.yml`

- [ ] **3.2 Add LDAP env vars to deployment template**
  - **Dependencies**: None
  - **Description**: Add LDAP-specific env var blocks to the connections loop in deployment.yml: ldapCheckMethod, ldapBindDN, ldapBindPassword, ldapBaseDN, ldapSearchFilter, ldapSearchScope, ldapStartTLS, ldapTLSSkipVerify. Add Secret-based ldapBindPassword support in the auth section.
  - **Modifies**:
    - `deploy/helm/uniproxy/templates/deployment.yml`

- [ ] **3.3 Helm lint and template validation**
  - **Dependencies**: 3.1, 3.2
  - **Description**: Run `helm lint` on the chart. Run `helm template` with an instance file that includes LDAP connections and isentry. Verify generated env vars are correct.
  - **Validates**:
    - `helm lint ./deploy/helm/uniproxy`
    - `helm template` with test values

### Completion Criteria Phase 3

- [ ] All subtasks completed (3.1–3.3)
- [ ] `helm lint` passes
- [ ] `helm template` generates correct LDAP env vars
- [ ] `helm template` generates DEPHEALTH_ISENTRY when set

---

## Phase 4: Build & Test in Docker/K8s

**Dependencies**: Phase 1, Phase 2, Phase 3
**Status**: Pending

### Description

Build Docker image and test the complete solution in Docker first, then in Kubernetes.

### Subtasks

- [ ] **4.1 Build Docker image**
  - **Dependencies**: None
  - **Description**: Build Docker image with `docker build -t uniproxy:dev .`. Verify it starts correctly with a minimal HTTP dependency config.
  - **Creates**:
    - Docker image `uniproxy:dev`

- [ ] **4.2 Test isentry label in Docker**
  - **Dependencies**: 4.1
  - **Description**: Run container with `DEPHEALTH_ISENTRY=yes` and an HTTP dependency. Verify that `/metrics` output contains `isentry="yes"` label on all `app_dependency_*` metrics.
  - **Validates**:
    - `curl localhost:9090/metrics | grep isentry`

- [ ] **4.3 Test LDAP dependency in Docker (optional)**
  - **Dependencies**: 4.1
  - **Description**: If an LDAP server is available for testing, run container with LDAP dependency configured. Verify metrics are generated with `type="ldap"`. If no LDAP server available — verify at minimum that the application starts without error with LDAP configured (it will show health=DOWN but should not crash).
  - **Validates**:
    - Application starts with LDAP dependency
    - Metrics contain `type="ldap"` gauge

- [ ] **4.4 Deploy to Kubernetes**
  - **Dependencies**: 4.1
  - **Description**: Deploy to test cluster using Helm with isentry enabled. Verify pods start and metrics are correct.
  - **Validates**:
    - `helm install/upgrade` succeeds
    - Pods in Running state
    - Metrics contain isentry label

### Completion Criteria Phase 4

- [ ] All subtasks completed (4.1–4.4)
- [ ] Docker image builds successfully
- [ ] isentry label visible in Prometheus metrics
- [ ] Application handles LDAP dependency type without crashing
- [ ] Kubernetes deployment works with updated Helm chart

---

## Phase 5: Documentation

**Dependencies**: Phase 2, Phase 3
**Status**: Pending

### Description

Update all project documentation to reflect the new LDAP dependency type, isentry label,
and SDK version upgrade. Documentation exists in two languages (EN/RU) and must be updated
in both.

### Subtasks

- [ ] **5.1 Update README.md and README.ru.md**
  - **Dependencies**: None
  - **Description**: Update both README files:
    - SDK version badge: `v0.4.2` → `v0.8.0`
    - Key Features list: add LDAP to the dependency types line (`HTTP, gRPC, PostgreSQL, MySQL, Redis, AMQP, Kafka, TCP` → add `LDAP`)
    - Add `isentry` label to features list (optional global label for entry-point marking)
    - Add LDAP configuration example to the Environment Variables reference section
    - Add `DEPHEALTH_ISENTRY` to the global env vars table
    - Update Docker run examples version tag if needed
  - **Modifies**:
    - `README.md`
    - `README.ru.md`

- [ ] **5.2 Update CLAUDE.md**
  - **Dependencies**: None
  - **Description**: Update CLAUDE.md project instructions:
    - Architecture section: add LDAP to the dependency types comment in `Dependency` struct description
    - Configuration Model section: mention isentry as global optional label
    - Common Commands: update Docker run example if needed
    - Debugging Tips / Common Issues: add LDAP-related troubleshooting (e.g., "LDAP health always DOWN: verify network connectivity to LDAP server, check bind credentials for simple_bind")
  - **Modifies**:
    - `CLAUDE.md`

- [ ] **5.3 Update Helm chart documentation**
  - **Dependencies**: None
  - **Description**: Update Helm values.yaml inline comments:
    - Add documentation comment for `isentry` field (already added in Phase 3, verify completeness)
    - Add LDAP connection example in the instances comment block
    - Update instance example files if needed (add commented-out LDAP example to `instances/ns1-homelab.yaml`)
  - **Modifies**:
    - `deploy/helm/uniproxy/values.yaml` (verify comments)
    - `deploy/helm/uniproxy/instances/ns1-homelab.yaml` (optional: add commented LDAP example)

- [ ] **5.4 Update use-cases documentation**
  - **Dependencies**: None
  - **Description**: Review and update `docs/use-cases.md` and `docs/use-cases.ru.md` if they reference supported dependency types. Add LDAP use case if appropriate (e.g., "Testing connectivity to corporate LDAP/Active Directory").
  - **Modifies**:
    - `docs/use-cases.md`
    - `docs/use-cases.ru.md`

### Completion Criteria Phase 5

- [ ] All subtasks completed (5.1–5.4)
- [ ] SDK version badge updated in both READMEs
- [ ] LDAP listed in all dependency type enumerations across docs
- [ ] isentry label documented with usage example
- [ ] Helm chart inline documentation complete
- [ ] CLAUDE.md reflects current architecture

---

## Notes

- **SDK compatibility**: v0.6.0 → v0.8.0 has no breaking changes. Only additive: LDAP checker (v0.8.0), dynamic endpoints (v0.7.0).
- **LDAP password security**: `LDAP_BIND_PASSWORD` supports `_FILE` variant via `resolveSecret()` for mounting from Kubernetes Secrets.
- **isentry semantics**: It's a boolean flag (present or absent), not a key-value pair. When `DEPHEALTH_ISENTRY=yes`, the label `isentry="yes"` is added. Users in dephealth-ui will use this to identify graph entry-point vertices.
- **LDAP default port**: SDK handles `ldap://` (port 389) and `ldaps://` (port 636) automatically from URL scheme.
