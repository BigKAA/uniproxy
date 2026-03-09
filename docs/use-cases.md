# uniproxy — Use Cases and Examples

[Русская версия (use-cases.ru.md)](./use-cases.ru.md)

## Overview

uniproxy is a universal test proxy for dependency health monitoring. Its key feature is **recursive chain visibility**: by sending a single HTTP request to the first uniproxy in a chain, you can discover the health status of the entire dependency tree — **without Prometheus, without any metrics infrastructure**.

```
curl "http://entry-point:8080/?detail=true&depth=5"
```

This single request returns a nested JSON tree with health status, latency, connection details, and downstream responses for every service in the chain.

uniproxy works in any environment: Kubernetes, Docker, bare metal, VMs, or any combination of these. It supports HTTP, gRPC, TCP, PostgreSQL, MySQL, Redis, AMQP, Kafka, and LDAP health checks.

---

## Table of Contents

1. [Recursive Chain Diagnostics (without Prometheus)](#1-recursive-chain-diagnostics-without-prometheus)
2. [Kubernetes Microservice Chain](#2-kubernetes-microservice-chain)
3. [Docker Compose for Local Development](#3-docker-compose-for-local-development)
4. [Bare Metal / VM Infrastructure](#4-bare-metal--vm-infrastructure)
5. [Mixed Environment (K8s + VMs)](#5-mixed-environment-k8s--vms)
6. [Sidecar for Legacy Applications](#6-sidecar-for-legacy-applications)
7. [Network Policy and Firewall Testing](#7-network-policy-and-firewall-testing)
8. [CI/CD Pipeline Health Gate](#8-cicd-pipeline-health-gate)
9. [Multi-Cluster Monitoring](#9-multi-cluster-monitoring)
10. [Database Migration Readiness](#10-database-migration-readiness)
11. [Disaster Recovery Verification](#11-disaster-recovery-verification)
12. [Health-Checking Services Behind Bearer Auth](#12-health-checking-services-behind-bearer-auth)
13. [Secure Credentials with Kubernetes Secrets](#13-secure-credentials-with-kubernetes-secrets)
14. [Custom API Key Headers for Third-Party Services](#14-custom-api-key-headers-for-third-party-services)
15. [LDAP / Active Directory Connectivity Testing](#15-ldap--active-directory-connectivity-testing)

---

## 1. Recursive Chain Diagnostics (without Prometheus)

### Problem

You have a chain of services and need to quickly understand which dependency is down — without setting up Prometheus, Grafana, or any monitoring stack. You just need `curl`.

### Architecture

```
                   curl
                    │
                    ▼
┌──────────┐   ┌──────────┐   ┌───────────┐   ┌─────────┐
│ frontend │──▶│ backend  │──▶│ auth-svc  │──▶│ user-db │
│ :8080    │   │ :8080    │   │ :8080     │   │ :5432   │
│ uniproxy │   │ uniproxy │   │ uniproxy  │   │ postgres│
└──────────┘   └──────────┘   └───────────┘   └─────────┘
                    │                              ▲
                    └──────────────────────────────┘
                              redis :6379
```

### Single Request — Full Tree

```bash
curl -s "http://frontend:8080/?detail=true&depth=3" | jq .
```

**Response:**

```json
{
  "name": "frontend",
  "podName": "",
  "namespace": "",
  "dependencies": {
    "backend:backend:8080": {
      "healthy": true,
      "status": "ok",
      "latency_ms": 5.2,
      "type": "http",
      "name": "backend",
      "host": "backend",
      "port": "8080",
      "critical": true,
      "response": {
        "name": "backend",
        "dependencies": {
          "auth-svc:auth-svc:8080": {
            "healthy": true,
            "status": "ok",
            "latency_ms": 3.1,
            "type": "http",
            "name": "auth-svc",
            "host": "auth-svc",
            "port": "8080",
            "critical": true,
            "response": {
              "name": "auth-svc",
              "dependencies": {
                "user-db:pg.svc:5432": {
                  "healthy": false,
                  "status": "connection_error",
                  "detail": "connection_refused",
                  "latency_ms": 3012.0,
                  "type": "postgres",
                  "name": "user-db",
                  "host": "pg.svc",
                  "port": "5432",
                  "critical": true
                }
              }
            }
          },
          "cache:redis:6379": {
            "healthy": true,
            "status": "ok",
            "latency_ms": 0.8,
            "type": "redis",
            "name": "cache",
            "host": "redis",
            "port": "6379",
            "critical": false
          }
        }
      }
    }
  }
}
```

From this single response you immediately see:
- **frontend → backend**: OK (5.2 ms)
- **backend → auth-svc**: OK (3.1 ms)
- **backend → cache (redis)**: OK (0.8 ms)
- **auth-svc → user-db (postgres)**: **DOWN** — connection refused (3012 ms timeout)

**Depth control:**
- `depth=0` — no recursive fetch, only local dependencies
- `depth=1` — fetch one level deep (default)
- `depth=5` — up to 5 levels of recursion
- `depth=10` — maximum depth

---

## 2. Kubernetes Microservice Chain

### Problem

You have a microservice application in Kubernetes and want to validate that all inter-service connections work after deployment.

### Architecture

```
Namespace: production
┌─────────────────────────────────────────────────────┐
│                                                     │
│  ┌──────────┐   ┌──────────┐   ┌──────────┐         │
│  │ gateway  │──▶│ order-svc│──▶│ payment  │         │
│  │ uniproxy │   │ uniproxy │   │ uniproxy │         │
│  │ :8080    │   │ :8080    │   │ :8080    │         │
│  └──────────┘   └──────────┘   └──────────┘         │
│       │              │              │               │
│       │              ▼              ▼               │
│       │         ┌──────────┐  ┌──────────┐          │
│       │         │ postgres │  │  stripe  │ (ext)    │
│       │         │  :5432   │  │ api.com  │          │
│       │         └──────────┘  └──────────┘          │
│       ▼                                             │
│  ┌──────────┐                                       │
│  │  redis   │                                       │
│  │  :6379   │                                       │
│  └──────────┘                                       │
└─────────────────────────────────────────────────────┘
```

### Helm Values: gateway

```yaml
# instances/gateway.yaml
name: gateway
dependencies:
  - name: order-svc
    type: http
    url: "http://order-svc.production.svc:8080"
    critical: "yes"
    healthPath: "/"
  - name: session-cache
    type: redis
    host: redis.production.svc
    port: "6379"
    critical: "no"
```

### Helm Values: order-svc

```yaml
# instances/order-svc.yaml
name: order-svc
dependencies:
  - name: payment
    type: http
    url: "http://payment.production.svc:8080"
    critical: "yes"
    healthPath: "/"
  - name: orders-db
    type: postgres
    url: "postgres://user:pass@postgres.production.svc:5432/orders"
    critical: "yes"
```

### Deploy and Check

```bash
# Deploy all instances
helm install gateway ./deploy/helm/uniproxy \
  -f instances/gateway.yaml -n production

helm install order-svc ./deploy/helm/uniproxy \
  -f instances/order-svc.yaml -n production

helm install payment ./deploy/helm/uniproxy \
  -f instances/payment.yaml -n production

# Port-forward to gateway and check entire chain
kubectl port-forward -n production svc/gateway 8080:8080

curl -s "http://localhost:8080/?detail=true&depth=3" | jq .
```

---

## 3. Docker Compose for Local Development

### Problem

You want to quickly validate connectivity between services during local development without installing monitoring tools.

### docker-compose.yml

```yaml
services:
  # --- Infrastructure ---
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: app
      POSTGRES_PASSWORD: secret
      POSTGRES_DB: mydb
    ports:
      - "5432:5432"

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"

  # --- Application chain ---
  frontend-proxy:
    image: uniproxy:dev
    environment:
      DEPHEALTH_NAME: frontend
      DEPHEALTH_GROUP: my-group
      DEPHEALTH_DEPS: "api:http,cache:redis"
      DEPHEALTH_API_URL: "http://api-proxy:8080"
      DEPHEALTH_API_CRITICAL: "yes"
      DEPHEALTH_CACHE_HOST: redis
      DEPHEALTH_CACHE_PORT: "6379"
      DEPHEALTH_CACHE_CRITICAL: "no"
    ports:
      - "8080:8080"

  api-proxy:
    image: uniproxy:dev
    environment:
      DEPHEALTH_NAME: api
      DEPHEALTH_GROUP: my-group
      DEPHEALTH_DEPS: "db:postgres,cache:redis"
      DEPHEALTH_DB_URL: "postgres://app:secret@postgres:5432/mydb"
      DEPHEALTH_DB_CRITICAL: "yes"
      DEPHEALTH_CACHE_HOST: redis
      DEPHEALTH_CACHE_PORT: "6379"
      DEPHEALTH_CACHE_CRITICAL: "no"
```

### Check

```bash
docker compose up -d

# One command to check the entire chain
curl -s "http://localhost:8080/?detail=true&depth=2" | jq .

# Quick health check (simple format)
curl -s http://localhost:8080/
# {"name":"frontend","podName":"","namespace":"","health":{"api:api-proxy:8080":true,"cache:redis:6379":true}}
```

### Useful Scripts

```bash
# Watch connectivity in real-time
watch -n 2 'curl -s "http://localhost:8080/?detail=true&depth=2" | jq ".dependencies | to_entries[] | {name: .value.name, healthy: .value.healthy, latency_ms: .value.latency_ms}"'

# Check only unhealthy dependencies in the chain
curl -s "http://localhost:8080/?detail=true&depth=3" | \
  jq '.. | objects | select(.healthy == false) | {name, host, port, status, detail}'
```

---

## 4. Bare Metal / VM Infrastructure

### Problem

You have traditional infrastructure (no containers, no Kubernetes) and need to monitor connectivity between servers and services.

### Architecture

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│  web-server-01  │     │  app-server-01  │     │   db-server-01  │
│  192.168.1.10   │────▶│  192.168.1.20   │────▶│  192.168.1.30   │
│  uniproxy:8080  │     │  uniproxy:8080  │     │  PostgreSQL:5432│
└─────────────────┘     └─────────────────┘     │  Redis:6379     │
                              │                 └─────────────────┘
                              ▼
                        ┌─────────────────┐
                        │  mq-server-01   │
                        │  192.168.1.40   │
                        │  RabbitMQ:5672  │
                        └─────────────────┘
```

### Installation (systemd)

Download the binary and create a systemd unit:

```bash
# On web-server-01
cat > /etc/systemd/system/uniproxy.service <<EOF
[Unit]
Description=uniproxy health check proxy
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/uniproxy
Environment=DEPHEALTH_NAME=web-server-01
Environment=DEPHEALTH_GROUP=infrastructure
Environment=DEPHEALTH_DEPS=app-server:http
Environment=DEPHEALTH_APP_SERVER_HOST=192.168.1.20
Environment=DEPHEALTH_APP_SERVER_PORT=8080
Environment=DEPHEALTH_APP_SERVER_CRITICAL=yes
Restart=always

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now uniproxy
```

```bash
# On app-server-01
cat > /etc/systemd/system/uniproxy.service <<EOF
[Unit]
Description=uniproxy health check proxy
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/uniproxy
Environment=DEPHEALTH_NAME=app-server-01
Environment=DEPHEALTH_GROUP=infrastructure
Environment=DEPHEALTH_DEPS=database:postgres,cache:redis,message-queue:amqp
Environment=DEPHEALTH_DATABASE_URL=postgres://user:pass@192.168.1.30:5432/appdb
Environment=DEPHEALTH_DATABASE_CRITICAL=yes
Environment=DEPHEALTH_CACHE_HOST=192.168.1.30
Environment=DEPHEALTH_CACHE_PORT=6379
Environment=DEPHEALTH_CACHE_CRITICAL=no
Environment=DEPHEALTH_MESSAGE_QUEUE_HOST=192.168.1.40
Environment=DEPHEALTH_MESSAGE_QUEUE_PORT=5672
Environment=DEPHEALTH_MESSAGE_QUEUE_CRITICAL=yes
Restart=always

[Install]
WantedBy=multi-user.target
EOF
```

### Check from Anywhere

```bash
# From your workstation — check entire infrastructure
curl -s "http://192.168.1.10:8080/?detail=true&depth=2" | jq .

# Simple connectivity check
curl -s http://192.168.1.10:8080/
# {"name":"web-server-01","health":{"app-server:192.168.1.20:8080":true}}

# Check specific server
curl -s "http://192.168.1.20:8080/?detail=true" | jq .
```

---

## 5. Mixed Environment (K8s + VMs)

### Problem

Part of your infrastructure runs in Kubernetes, part on bare metal VMs. You need end-to-end visibility across both environments.

### Architecture

```
     Kubernetes cluster                    Bare metal
┌──────────────────────┐          ┌──────────────────────┐
│  ┌──────────┐        │          │                      │
│  │ frontend │        │          │  ┌──────────────┐    │
│  │ uniproxy │──────────────────▶│  │ legacy-api   │    │
│  │ :8080    │        │          │  │ 10.0.5.100   │    │
│  └──────────┘        │          │  │ uniproxy:8080│    │
│       │              │          │  └──────┬───────┘    │
│       ▼              │          │         │            │
│  ┌──────────┐        │          │         ▼            │
│  │ redis    │        │          │  ┌──────────────┐    │
│  │ :6379    │        │          │  │ oracle-db    │    │
│  └──────────┘        │          │  │ 10.0.5.200   │    │
│                      │          │  │ TCP :1521    │    │
└──────────────────────┘          │  └──────────────┘    │
                                  └──────────────────────┘
```

### Configuration

**Kubernetes side** (Helm values):

```yaml
name: frontend
dependencies:
  - name: legacy-api
    type: http
    host: "10.0.5.100"
    port: "8080"
    critical: "yes"
  - name: cache
    type: redis
    host: redis.default.svc
    port: "6379"
    critical: "no"
```

**VM side** (environment variables):

```bash
DEPHEALTH_NAME=legacy-api
DEPHEALTH_GROUP=infrastructure
DEPHEALTH_DEPS="oracle:tcp"
DEPHEALTH_ORACLE_HOST=10.0.5.200
DEPHEALTH_ORACLE_PORT=1521
DEPHEALTH_ORACLE_CRITICAL=yes
```

### Cross-Environment Check

```bash
# From K8s — check entire chain including bare metal
kubectl port-forward svc/frontend 8080:8080
curl -s "http://localhost:8080/?detail=true&depth=3" | jq .
```

The response will include the K8s Redis status **and** recursively fetch the legacy-api's status, which shows Oracle DB connectivity from the bare metal server.

---

## 6. Sidecar for Legacy Applications

### Problem

You have applications where you **cannot integrate the dephealth SDK** (closed source, different language, legacy code). You need to monitor their dependencies without modifying the application.

### Architecture

```
Pod / Host
┌────────────────────────────────────────┐
│                                        │
│  ┌──────────────┐  ┌───────────────┐   │
│  │ legacy-app   │  │   uniproxy    │   │
│  │ (Java/.NET/  │  │   (sidecar)   │   │
│  │  PHP/etc.)   │  │   :8080       │   │
│  │              │  │               │   │
│  │ Uses:        │  │ Monitors:     │   │
│  │ - DB :5432   │  │ - DB :5432    │   │
│  │ - Redis :6379│  │ - Redis :6379 │   │
│  │ - API :443   │  │ - API :443    │   │
│  └──────────────┘  └───────────────┘   │
│                                        │
└────────────────────────────────────────┘
```

The sidecar uniproxy monitors the **same dependencies** that the application uses, providing health visibility without touching the application code.

### Kubernetes Pod Spec

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: legacy-app
spec:
  containers:
    - name: app
      image: legacy-app:latest
      ports:
        - containerPort: 80

    - name: health-sidecar
      image: uniproxy:0.7.1
      ports:
        - containerPort: 8080
      env:
        - name: DEPHEALTH_NAME
          value: "legacy-app"
        - name: DEPHEALTH_GROUP
          value: "production"
        - name: DEPHEALTH_DEPS
          value: "main-db:postgres,session-cache:redis,payment-api:http"
        - name: DEPHEALTH_MAIN_DB_URL
          value: "postgres://user:pass@postgres.svc:5432/legacy"
        - name: DEPHEALTH_MAIN_DB_CRITICAL
          value: "yes"
        - name: DEPHEALTH_SESSION_CACHE_HOST
          value: "redis.svc"
        - name: DEPHEALTH_SESSION_CACHE_PORT
          value: "6379"
        - name: DEPHEALTH_SESSION_CACHE_CRITICAL
          value: "no"
        - name: DEPHEALTH_PAYMENT_API_URL
          value: "https://payment-api.svc:443"
        - name: DEPHEALTH_PAYMENT_API_CRITICAL
          value: "yes"
        - name: DEPHEALTH_PAYMENT_API_TLS
          value: "yes"
```

### Docker Compose Sidecar

```yaml
services:
  legacy-app:
    image: legacy-app:latest
    ports:
      - "80:80"

  health-sidecar:
    image: uniproxy:dev
    network_mode: "service:legacy-app"  # shares network namespace
    environment:
      DEPHEALTH_NAME: legacy-app
      DEPHEALTH_GROUP: production
      DEPHEALTH_DEPS: "db:postgres,cache:redis"
      DEPHEALTH_DB_URL: "postgres://user:pass@db-host:5432/app"
      DEPHEALTH_DB_CRITICAL: "yes"
      DEPHEALTH_CACHE_HOST: redis-host
      DEPHEALTH_CACHE_PORT: "6379"
      DEPHEALTH_CACHE_CRITICAL: "no"
```

### Check Sidecar Health

```bash
# The sidecar shares the pod/host network
curl -s "http://legacy-app:8080/?detail=true" | jq .
```

### Benefits

- **Zero application changes** — no SDK integration required
- **Language agnostic** — works with Java, .NET, PHP, Python, Node.js, or any other stack
- **Same dependency view** — monitors exactly the same endpoints the application connects to
- **Prometheus metrics** — sidecar exposes `app_dependency_health`, `app_dependency_latency_seconds`, `app_dependency_status`, and `app_dependency_status_detail`

---

## 7. Network Policy and Firewall Testing

### Problem

After applying Kubernetes NetworkPolicies or firewall rules, you need to verify that allowed connections work and blocked connections are actually blocked.

### Architecture

```
Namespace: frontend-ns          Namespace: backend-ns
┌────────────────────┐         ┌────────────────────┐
│  ┌──────────┐      │  allow  │  ┌──────────┐      │
│  │ probe-01 │──────────────────▶│ backend  │      │
│  │ uniproxy │      │         │  │ :8080    │      │
│  └──────────┘      │         │  └──────────┘      │
│       │            │         │       │            │
│       │  deny      │         │       │ allow      │
│       ▼            │         │       ▼            │
│  ┌──────────┐      │         │  ┌──────────┐      │
│  │ blocked  │──────X         │  │ database │      │
│  │ (should  │      │         │  │ :5432    │      │
│  │  fail)   │      │         │  └──────────┘      │
│  └──────────┘      │         │                    │
└────────────────────┘         └────────────────────┘
```

### Test Config

```yaml
# probe-01: tests both allowed and blocked connections
name: netpol-probe
dependencies:
  - name: backend-allowed
    type: http
    host: backend.backend-ns.svc
    port: "8080"
    critical: "yes"
  - name: db-should-be-blocked
    type: tcp
    host: database.backend-ns.svc
    port: "5432"
    critical: "no"
```

### Verify

```bash
curl -s "http://probe-01:8080/?detail=true" | jq '.dependencies | to_entries[] | {name: .value.name, healthy: .value.healthy, status: .value.status}'
```

Expected:
```json
{"name": "backend-allowed", "healthy": true, "status": "ok"}
{"name": "db-should-be-blocked", "healthy": false, "status": "connection_error"}
```

If `db-should-be-blocked` is healthy — your NetworkPolicy has a gap!

---

## 8. CI/CD Pipeline Health Gate

### Problem

Before deploying a new version, verify that all downstream dependencies are available and healthy.

### Pipeline Script

```bash
#!/bin/bash
# pre-deploy-check.sh — Run before deployment

ENTRY_POINT="http://gateway.staging.svc:8080"
DEPTH=3

echo "Checking dependency chain (depth=$DEPTH)..."

RESPONSE=$(curl -sf "$ENTRY_POINT/?detail=true&depth=$DEPTH")
if [ $? -ne 0 ]; then
    echo "FAIL: Cannot reach entry point $ENTRY_POINT"
    exit 1
fi

# Check for any unhealthy critical dependencies in the entire tree
UNHEALTHY=$(echo "$RESPONSE" | jq -r '
  [.. | objects | select(.healthy == false and .critical == true)]
  | length
')

if [ "$UNHEALTHY" -gt 0 ]; then
    echo "FAIL: $UNHEALTHY critical dependencies are unhealthy:"
    echo "$RESPONSE" | jq '.. | objects | select(.healthy == false and .critical == true) | {name, host, port, status, detail}'
    exit 1
fi

echo "PASS: All critical dependencies are healthy"
exit 0
```

### GitLab CI Example

```yaml
stages:
  - test
  - health-check
  - deploy

dependency-check:
  stage: health-check
  image: curlimages/curl:latest
  script:
    - |
      RESPONSE=$(curl -sf "http://gateway.staging:8080/?detail=true&depth=3")
      UNHEALTHY=$(echo "$RESPONSE" | jq '[.. | objects | select(.healthy == false and .critical == true)] | length')
      if [ "$UNHEALTHY" -gt 0 ]; then
        echo "Critical dependencies are unhealthy!"
        echo "$RESPONSE" | jq '.. | objects | select(.healthy == false and .critical == true)'
        exit 1
      fi
      echo "All dependencies healthy"
  rules:
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
```

---

## 9. Multi-Cluster Monitoring

### Problem

You have services spread across multiple Kubernetes clusters or data centers and need cross-cluster dependency visibility.

### Architecture

```
     Cluster A (EU)                         Cluster B (US)
┌──────────────────────┐            ┌──────────────────────┐
│  ┌──────────┐        │   HTTP     │  ┌──────────┐        │
│  │ eu-front │────────────────────▶│  │ us-api   │        │
│  │ uniproxy │        │            │  │ uniproxy │        │
│  │ :8080    │        │            │  │ :8080    │        │
│  └──────────┘        │            │  └──────────┘        │
│       │              │            │       │              │
│       ▼              │            │       ▼              │
│  ┌──────────┐        │            │  ┌──────────┐        │
│  │ eu-db    │        │            │  │ us-db    │        │
│  │ postgres │        │            │  │ postgres │        │
│  └──────────┘        │            │  └──────────┘        │
└──────────────────────┘            └──────────────────────┘
```

### Configuration

**Cluster A — eu-front:**

```bash
DEPHEALTH_NAME=eu-front
DEPHEALTH_GROUP=eu-cluster
DEPHEALTH_DEPS="us-api:http,eu-db:postgres"
DEPHEALTH_US_API_HOST=us-api.example.com    # external endpoint
DEPHEALTH_US_API_PORT=443
DEPHEALTH_US_API_CRITICAL=yes
DEPHEALTH_US_API_TLS=yes
DEPHEALTH_EU_DB_URL=postgres://eu-db.svc:5432/app
DEPHEALTH_EU_DB_CRITICAL=yes
```

**Cluster B — us-api:**

```bash
DEPHEALTH_NAME=us-api
DEPHEALTH_GROUP=us-cluster
DEPHEALTH_DEPS="us-db:postgres"
DEPHEALTH_US_DB_URL=postgres://us-db.svc:5432/app
DEPHEALTH_US_DB_CRITICAL=yes
```

### Cross-Cluster Check

```bash
# From Cluster A — see both clusters
curl -s "http://eu-front:8080/?detail=true&depth=2" | jq .
```

The response shows EU local DB status **and** recursively fetches the US API's status, which includes the US DB connectivity.

---

## 10. Database Migration Readiness

### Problem

Before running a database migration, verify that all applications can reach the database and that the database is accepting connections.

### Architecture

```
┌──────────┐  ┌──────────┐  ┌──────────┐
│ service-1│  │ service-2│  │ service-3│
│ uniproxy │  │ uniproxy │  │ uniproxy │
└────┬─────┘  └────┬─────┘  └────┬─────┘
     │             │             │
     └──────┬──────┘─────────────┘
            ▼
      ┌──────────┐
      │ postgres │  ← migration target
      │  :5432   │
      └──────────┘
```

### Pre-Migration Check Script

```bash
#!/bin/bash
# check-db-connectivity.sh

SERVICES=("service-1:8080" "service-2:8080" "service-3:8080")
DB_NAME="main-db"
ALL_OK=true

for svc in "${SERVICES[@]}"; do
    RESPONSE=$(curl -sf "http://$svc/?detail=true" 2>/dev/null)
    if [ $? -ne 0 ]; then
        echo "WARN: Cannot reach $svc"
        ALL_OK=false
        continue
    fi

    DB_STATUS=$(echo "$RESPONSE" | jq -r ".dependencies | to_entries[] | select(.value.name == \"$DB_NAME\") | .value.healthy")
    DB_LATENCY=$(echo "$RESPONSE" | jq -r ".dependencies | to_entries[] | select(.value.name == \"$DB_NAME\") | .value.latency_ms")

    if [ "$DB_STATUS" = "true" ]; then
        echo "OK: $svc → $DB_NAME (${DB_LATENCY}ms)"
    else
        echo "FAIL: $svc → $DB_NAME is unhealthy"
        ALL_OK=false
    fi
done

if [ "$ALL_OK" = "true" ]; then
    echo ""
    echo "All services can reach the database. Safe to proceed with migration."
else
    echo ""
    echo "Some services cannot reach the database. DO NOT proceed with migration."
    exit 1
fi
```

---

## 11. Disaster Recovery Verification

### Problem

After activating a disaster recovery site, verify that all services can reach their dependencies in the new environment.

### Architecture

```
Primary Site (DOWN)              DR Site (ACTIVE)
┌──────────────────┐            ┌──────────────────┐
│  ┌────────┐      │            │  ┌────────┐      │
│  │ app    │  ✕   │            │  │ app    │  ✓   │
│  └────────┘      │            │  │ proxy  │      │
│  ┌────────┐      │            │  └───┬────┘      │
│  │ db     │  ✕   │            │      │           │
│  └────────┘      │            │  ┌───▼────┐      │
└──────────────────┘            │  │ db-dr  │  ✓   │
                                │  └────────┘      │
                                │  ┌────────┐      │
                                │  │ cache  │  ✓   │
                                │  └────────┘      │
                                └──────────────────┘
```

### DR Validation Script

```bash
#!/bin/bash
# dr-validate.sh — Run after DR failover

DR_ENTRY="http://app-proxy.dr-site:8080"

echo "=== DR Site Validation ==="
echo "Timestamp: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
echo ""

RESPONSE=$(curl -sf "$DR_ENTRY/?detail=true&depth=3")
if [ $? -ne 0 ]; then
    echo "CRITICAL: DR entry point unreachable!"
    exit 1
fi

echo "Dependency status:"
echo "$RESPONSE" | jq -r '
  .. | objects | select(.name != null and .healthy != null) |
  (if .healthy then "  ✓ " else "  ✗ " end) + .name + " (" + .host + ":" + .port + ") — " + .status
'

TOTAL=$(echo "$RESPONSE" | jq '[.. | objects | select(.healthy != null)] | length')
HEALTHY=$(echo "$RESPONSE" | jq '[.. | objects | select(.healthy == true)] | length')
CRITICAL_DOWN=$(echo "$RESPONSE" | jq '[.. | objects | select(.healthy == false and .critical == true)] | length')

echo ""
echo "Summary: $HEALTHY/$TOTAL healthy"

if [ "$CRITICAL_DOWN" -gt 0 ]; then
    echo "CRITICAL: $CRITICAL_DOWN critical dependencies are DOWN"
    exit 1
fi

echo "DR site validation: PASSED"
```

**Example output:**

```
=== DR Site Validation ===
Timestamp: 2026-02-15T12:00:00Z

Dependency status:
  ✓ api-backend (api.dr-site:8080) — ok
  ✓ database (db-dr.dr-site:5432) — ok
  ✓ cache (redis.dr-site:6379) — ok
  ✗ primary-db (db.primary-site:5432) — connection_error

Summary: 3/4 healthy
DR site validation: PASSED
```

---

## 12. Health-Checking Services Behind Bearer Auth

### Problem

Your HTTP or gRPC dependencies require authentication. Without proper credentials, health checks return `401 Unauthorized` or `403 Forbidden`, making it impossible to determine if the service is actually healthy.

### Solution

Configure uniproxy with bearer token authentication per dependency.

### Architecture

```
uniproxy ──[Authorization: Bearer xxx]──> secure-api (HTTP 200 OK)
    │
    └──[Authorization: Bearer yyy]──> internal-svc (HTTP 200 OK)
```

### Configuration

```bash
docker run -p 8080:8080 \
  -e DEPHEALTH_NAME=auth-monitor \
  -e DEPHEALTH_GROUP=monitoring \
  -e DEPHEALTH_DEPS="secure-api:http,internal-svc:http" \
  -e DEPHEALTH_SECURE_API_URL="https://api.example.com/health" \
  -e DEPHEALTH_SECURE_API_CRITICAL=yes \
  -e DEPHEALTH_SECURE_API_BEARER_TOKEN="eyJhbGciOiJSUzI1NiIs..." \
  -e DEPHEALTH_INTERNAL_SVC_URL="http://internal.svc:8080" \
  -e DEPHEALTH_INTERNAL_SVC_CRITICAL=yes \
  -e DEPHEALTH_INTERNAL_SVC_BEARER_TOKEN="internal-service-token" \
  uniproxy:0.7.1
```

### Using Global Bearer Token

If all dependencies use the same token, set it once globally:

```bash
docker run -p 8080:8080 \
  -e DEPHEALTH_NAME=auth-monitor \
  -e DEPHEALTH_GROUP=monitoring \
  -e DEPHEALTH_BEARER_TOKEN="shared-token-for-all-deps" \
  -e DEPHEALTH_DEPS="api-1:http,api-2:http" \
  -e DEPHEALTH_API_1_URL="https://api-1.example.com" \
  -e DEPHEALTH_API_1_CRITICAL=yes \
  -e DEPHEALTH_API_2_URL="https://api-2.example.com" \
  -e DEPHEALTH_API_2_CRITICAL=yes \
  uniproxy:0.7.1
```

---

## 13. Secure Credentials with Kubernetes Secrets

### Problem

Storing tokens and passwords directly in environment variables (ConfigMaps, Helm values) is a security risk. You need to use Kubernetes Secrets for sensitive auth data.

### Solution

Use the `_FILE` suffix pattern to read secrets from mounted files, or use `secretKeyRef` in Helm values.

### Architecture

```
K8s Secret "api-creds"
  └─ token: "eyJhbGci..."
  └─ password: "s3cret"
       │
       ├──[mounted as /run/secrets/token]──> uniproxy (reads via _FILE)
       └──[secretKeyRef in env]──> uniproxy (injected by kubelet)
```

### Option A: `_FILE` Pattern (Docker / K8s volume mount)

```yaml
# secret.yaml
apiVersion: v1
kind: Secret
metadata:
  name: api-creds
type: Opaque
stringData:
  token: "eyJhbGciOiJSUzI1NiIs..."
  password: "s3cret"
```

```yaml
# deployment (volume mount approach)
containers:
  - name: uniproxy
    env:
      - name: DEPHEALTH_API_BEARER_TOKEN_FILE
        value: /run/secrets/token
    volumeMounts:
      - name: secrets
        mountPath: /run/secrets
        readOnly: true
volumes:
  - name: secrets
    secret:
      secretName: api-creds
```

### Option B: Helm `existingSecret` (secretKeyRef)

```yaml
# instances/production.yaml
instances:
  - name: auth-proxy
    connections:
      - name: secure-api
        type: http
        url: "https://api.example.com"
        critical: "yes"
        auth:
          existingSecret: "api-creds"
          bearerTokenKey: "token"
      - name: db-api
        type: http
        url: "https://db-api.internal:8080"
        critical: "yes"
        auth:
          existingSecret: "api-creds"
          basicUserKey: "username"
          basicPassKey: "password"
```

This renders `valueFrom.secretKeyRef` in the deployment — no inline secrets in Helm values.

---

## 14. Custom API Key Headers for Third-Party Services

### Problem

Some third-party APIs use custom headers for authentication (e.g., `X-API-Key`, `X-Auth-Token`) instead of standard `Authorization` headers.

### Solution

Use the `HEADERS` option to send arbitrary HTTP headers with health checks.

### Configuration

```bash
docker run -p 8080:8080 \
  -e DEPHEALTH_NAME=api-monitor \
  -e DEPHEALTH_GROUP=monitoring \
  -e DEPHEALTH_DEPS="stripe:http,sendgrid:http,datadog:http" \
  -e DEPHEALTH_STRIPE_URL="https://api.stripe.com/v1" \
  -e DEPHEALTH_STRIPE_CRITICAL=yes \
  -e DEPHEALTH_STRIPE_HEADERS='{"Authorization":"Bearer sk_live_xxx"}' \
  -e DEPHEALTH_SENDGRID_URL="https://api.sendgrid.com/v3/scopes" \
  -e DEPHEALTH_SENDGRID_CRITICAL=no \
  -e DEPHEALTH_SENDGRID_HEADERS='{"Authorization":"Bearer SG.xxx"}' \
  -e DEPHEALTH_DATADOG_URL="https://api.datadoghq.com/api/v1/validate" \
  -e DEPHEALTH_DATADOG_CRITICAL=no \
  -e DEPHEALTH_DATADOG_HEADERS='{"DD-API-KEY":"abc123","DD-APPLICATION-KEY":"def456"}' \
  uniproxy:0.7.1
```

### gRPC Metadata

For gRPC services, use `METADATA` to attach custom key-value pairs:

```bash
-e DEPHEALTH_GRPC_SVC_METADATA='{"x-api-key":"key123","x-request-id":"health-check"}'
```

---

## 15. LDAP / Active Directory Connectivity Testing

### Problem

Your applications depend on a corporate LDAP or Active Directory server for authentication. You need to verify LDAP connectivity and monitor its availability alongside other dependencies.

### Architecture

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│  web-app     │────▶│  auth-svc    │────▶│  AD / LDAP   │
│  uniproxy    │     │  uniproxy    │     │  :389 / :636 │
│  :8080       │     │  :8080       │     └──────────────┘
└──────────────┘     └──────────────┘
      │
      ▼
┌──────────────┐
│  postgres    │
│  :5432       │
└──────────────┘
```

### Configuration

```bash
docker run -p 8080:8080 \
  -e DEPHEALTH_NAME=auth-svc \
  -e DEPHEALTH_GROUP=production \
  -e DEPHEALTH_DEPS="corp-ad:ldap" \
  -e DEPHEALTH_CORP_AD_URL="ldaps://ad.corp.example.com:636" \
  -e DEPHEALTH_CORP_AD_CRITICAL=yes \
  -e DEPHEALTH_CORP_AD_LDAP_CHECK_METHOD=simple_bind \
  -e DEPHEALTH_CORP_AD_LDAP_BIND_DN="cn=svc-healthcheck,ou=Service Accounts,dc=corp,dc=example,dc=com" \
  -e DEPHEALTH_CORP_AD_LDAP_BIND_PASSWORD_FILE=/run/secrets/ldap-password \
  uniproxy:latest
```

### LDAP Check Methods

| Method | Description | Use case |
|--------|-------------|----------|
| `root_dse` | Read Root DSE (no auth) | Basic connectivity check |
| `anonymous_bind` | Anonymous LDAP bind | Verify server accepts connections |
| `simple_bind` | Bind with DN + password | Validate service account credentials |
| `search` | Perform an LDAP search | Verify read access to directory |

### Search Method Example

```bash
-e DEPHEALTH_MYLDAP_LDAP_CHECK_METHOD=search \
-e DEPHEALTH_MYLDAP_LDAP_BIND_DN="cn=reader,dc=example,dc=com" \
-e DEPHEALTH_MYLDAP_LDAP_BIND_PASSWORD="secret" \
-e DEPHEALTH_MYLDAP_LDAP_BASE_DN="ou=Users,dc=example,dc=com" \
-e DEPHEALTH_MYLDAP_LDAP_SEARCH_FILTER="(objectClass=person)" \
-e DEPHEALTH_MYLDAP_LDAP_SEARCH_SCOPE=sub
```

### Kubernetes with Secret

```yaml
# instances/auth-svc.yaml
instances:
  - name: auth-svc
    connections:
      - name: corp-ad
        type: ldap
        url: "ldaps://ad.corp.example.com:636"
        critical: "yes"
        ldapCheckMethod: "simple_bind"
        ldapBindDN: "cn=svc-healthcheck,ou=Service Accounts,dc=corp,dc=example,dc=com"
        auth:
          existingSecret: "ldap-creds"
          ldapBindPasswordKey: "password"
```

---

## Tips and Tricks

### Extract Only Unhealthy Dependencies from Any Depth

```bash
curl -s "http://entry:8080/?detail=true&depth=5" | \
  jq '.. | objects | select(.healthy == false) | {name, host, port, type, status, detail, latency_ms}'
```

### Simple Monitoring Loop (no Prometheus)

```bash
while true; do
    UNHEALTHY=$(curl -sf "http://entry:8080/?detail=true&depth=3" | \
      jq '[.. | objects | select(.healthy == false and .critical == true)] | length')
    if [ "$UNHEALTHY" -gt 0 ]; then
        echo "$(date): ALERT — $UNHEALTHY critical deps down!"
        # Send notification (Slack, Telegram, email)
    fi
    sleep 30
done
```

### Get Latency Report

```bash
curl -s "http://entry:8080/?detail=true&depth=3" | \
  jq '.. | objects | select(.latency_ms != null) | {name, latency_ms, host, port}' | \
  jq -s 'sort_by(.latency_ms) | reverse'
```

### JSON Logging for Log Aggregators

```bash
# Run with JSON logging for Loki/ELK/Fluentd
docker run \
  -e LOG_FORMAT=json \
  -e LOG_TIME_KEY=@timestamp \
  -e LOG_MESSAGE_KEY=message \
  -e LOG_LEVEL_KEY=severity \
  ...
  uniproxy:dev
```
