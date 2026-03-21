# uniproxy — Сценарии использования и примеры

[English version (use-cases.md)](./use-cases.md)

## Обзор

uniproxy — универсальный тестовый прокси для мониторинга здоровья зависимостей. Ключевая возможность — **рекурсивная видимость цепочек**: отправив один HTTP-запрос к первому uniproxy в цепочке, вы получаете статус здоровья всего дерева зависимостей — **без Prometheus, без какой-либо инфраструктуры мониторинга**.

```
curl "http://entry-point:8080/?detail=true&depth=5"
```

Один запрос возвращает вложенное JSON-дерево со статусом здоровья, задержкой, деталями подключения и ответами downstream-сервисов для каждого сервиса в цепочке.

uniproxy работает в любом окружении: Kubernetes, Docker, bare metal, ВМ или в любой их комбинации. Поддерживает проверки HTTP, gRPC, TCP, PostgreSQL, MySQL, Redis, AMQP, Kafka и LDAP.

---

## Содержание

1. [Рекурсивная диагностика цепочек (без Prometheus)](#1-рекурсивная-диагностика-цепочек-без-prometheus)
2. [Цепочка микросервисов в Kubernetes](#2-цепочка-микросервисов-в-kubernetes)
3. [Docker Compose для локальной разработки](#3-docker-compose-для-локальной-разработки)
4. [Bare Metal / ВМ инфраструктура](#4-bare-metal--вм-инфраструктура)
5. [Смешанное окружение (K8s + ВМ)](#5-смешанное-окружение-k8s--вм)
6. [Sidecar для legacy-приложений](#6-sidecar-для-legacy-приложений)
7. [Тестирование сетевых политик и файрволов](#7-тестирование-сетевых-политик-и-файрволов)
8. [Health gate в CI/CD пайплайне](#8-health-gate-в-cicd-пайплайне)
9. [Мониторинг между кластерами](#9-мониторинг-между-кластерами)
10. [Проверка готовности к миграции БД](#10-проверка-готовности-к-миграции-бд)
11. [Верификация аварийного восстановления](#11-верификация-аварийного-восстановления)
12. [Проверка сервисов за Bearer-аутентификацией](#12-проверка-сервисов-за-bearer-аутентификацией)
13. [Безопасное хранение учётных данных в Kubernetes Secrets](#13-безопасное-хранение-учётных-данных-в-kubernetes-secrets)
14. [Пользовательские API-ключи в заголовках для сторонних сервисов](#14-пользовательские-api-ключи-в-заголовках-для-сторонних-сервисов)
15. [Тестирование связности с LDAP / Active Directory](#15-тестирование-связности-с-ldap--active-directory)

---

## 1. Рекурсивная диагностика цепочек (без Prometheus)

### Проблема

У вас есть цепочка сервисов и нужно быстро понять, какая зависимость упала — без настройки Prometheus, Grafana или другого стека мониторинга. Достаточно `curl`.

### Архитектура

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

### Один запрос — полное дерево

```bash
curl -s "http://frontend:8080/?detail=true&depth=3" | jq .
```

**Ответ:**

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

Из одного ответа сразу видно:
- **frontend → backend**: OK (5.2 мс)
- **backend → auth-svc**: OK (3.1 мс)
- **backend → cache (redis)**: OK (0.8 мс)
- **auth-svc → user-db (postgres)**: **НЕДОСТУПЕН** — connection refused (3012 мс таймаут)

**Управление глубиной:**
- `depth=0` — без рекурсивного fetch, только локальные зависимости
- `depth=1` — один уровень вглубь (по умолчанию)
- `depth=5` — до 5 уровней рекурсии
- `depth=10` — максимальная глубина

---

## 2. Цепочка микросервисов в Kubernetes

### Проблема

У вас микросервисное приложение в Kubernetes, и нужно проверить, что все межсервисные соединения работают после деплоя.

### Архитектура

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
│       │         │ postgres │  │  stripe  │ (внеш.)  │
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

### Деплой и проверка

```bash
# Развёртывание всех экземпляров
helm install gateway ./deploy/helm/uniproxy \
  -f instances/gateway.yaml -n production

helm install order-svc ./deploy/helm/uniproxy \
  -f instances/order-svc.yaml -n production

helm install payment ./deploy/helm/uniproxy \
  -f instances/payment.yaml -n production

# Port-forward к gateway и проверка всей цепочки
kubectl port-forward -n production svc/gateway 8080:8080

curl -s "http://localhost:8080/?detail=true&depth=3" | jq .
```

---

## 3. Docker Compose для локальной разработки

### Проблема

Нужно быстро проверить связность между сервисами при локальной разработке без установки инструментов мониторинга.

### docker-compose.yml

```yaml
services:
  # --- Инфраструктура ---
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

  # --- Цепочка приложений ---
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

### Проверка

```bash
docker compose up -d

# Одна команда для проверки всей цепочки
curl -s "http://localhost:8080/?detail=true&depth=2" | jq .

# Быстрая проверка (простой формат)
curl -s http://localhost:8080/
# {"name":"frontend","podName":"","namespace":"","health":{"api:api-proxy:8080":true,"cache:redis:6379":true}}
```

### Полезные скрипты

```bash
# Мониторинг связности в реальном времени
watch -n 2 'curl -s "http://localhost:8080/?detail=true&depth=2" | jq ".dependencies | to_entries[] | {name: .value.name, healthy: .value.healthy, latency_ms: .value.latency_ms}"'

# Показать только нездоровые зависимости в цепочке
curl -s "http://localhost:8080/?detail=true&depth=3" | \
  jq '.. | objects | select(.healthy == false) | {name, host, port, status, detail}'
```

---

## 4. Bare Metal / ВМ инфраструктура

### Проблема

Традиционная инфраструктура (без контейнеров, без Kubernetes) — нужно мониторить связность между серверами и сервисами.

### Архитектура

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

### Установка (systemd)

Скачайте бинарный файл и создайте systemd unit:

```bash
# На web-server-01
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
# На app-server-01
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

### Проверка с любого хоста

```bash
# С рабочей станции — проверка всей инфраструктуры
curl -s "http://192.168.1.10:8080/?detail=true&depth=2" | jq .

# Простая проверка связности
curl -s http://192.168.1.10:8080/
# {"name":"web-server-01","health":{"app-server:192.168.1.20:8080":true}}

# Проверка конкретного сервера
curl -s "http://192.168.1.20:8080/?detail=true" | jq .
```

---

## 5. Смешанное окружение (K8s + ВМ)

### Проблема

Часть инфраструктуры работает в Kubernetes, часть — на bare metal ВМ. Нужна сквозная видимость через оба окружения.

### Архитектура

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

### Конфигурация

**Сторона Kubernetes** (Helm values):

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

**Сторона ВМ** (переменные окружения):

```bash
DEPHEALTH_NAME=legacy-api
DEPHEALTH_GROUP=infrastructure
DEPHEALTH_DEPS="oracle:tcp"
DEPHEALTH_ORACLE_HOST=10.0.5.200
DEPHEALTH_ORACLE_PORT=1521
DEPHEALTH_ORACLE_CRITICAL=yes
```

### Кросс-окружённая проверка

```bash
# Из K8s — проверка всей цепочки включая bare metal
kubectl port-forward svc/frontend 8080:8080
curl -s "http://localhost:8080/?detail=true&depth=3" | jq .
```

Ответ включит статус Redis из K8s **и** рекурсивно запросит статус legacy-api, который покажет связность с Oracle DB на bare metal сервере.

---

## 6. Sidecar для legacy-приложений

### Проблема

Есть приложения, в которые **невозможно интегрировать dephealth SDK** (закрытый исходный код, другой язык, legacy-код). Нужно мониторить их зависимости без модификации приложения.

### Архитектура

```
Pod / Хост
┌────────────────────────────────────────┐
│                                        │
│  ┌──────────────┐  ┌───────────────┐   │
│  │ legacy-app   │  │   uniproxy    │   │
│  │ (Java/.NET/  │  │   (sidecar)   │   │
│  │  PHP/и т.д.) │  │   :8080       │   │
│  │              │  │               │   │
│  │ Использует:  │  │ Мониторит:    │   │
│  │ - БД :5432   │  │ - БД :5432    │   │
│  │ - Redis :6379│  │ - Redis :6379 │   │
│  │ - API :443   │  │ - API :443    │   │
│  └──────────────┘  └───────────────┘   │
│                                        │
└────────────────────────────────────────┘
```

Sidecar uniproxy мониторит **те же зависимости**, что использует приложение, обеспечивая видимость здоровья без изменения кода приложения.

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
      image: uniproxy:0.7.3
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
    network_mode: "service:legacy-app"  # общее сетевое пространство
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

### Проверка здоровья sidecar

```bash
# Sidecar разделяет сетевое пространство пода/хоста
curl -s "http://legacy-app:8080/?detail=true" | jq .
```

### Преимущества

- **Ноль изменений в приложении** — интеграция SDK не требуется
- **Не зависит от языка** — работает с Java, .NET, PHP, Python, Node.js и любым другим стеком
- **Одинаковый обзор зависимостей** — мониторит те же самые эндпоинты, к которым подключается приложение
- **Метрики Prometheus** — sidecar экспортирует `app_dependency_health`, `app_dependency_latency_seconds`, `app_dependency_status` и `app_dependency_status_detail`

---

## 7. Тестирование сетевых политик и файрволов

### Проблема

После применения Kubernetes NetworkPolicy или правил файрвола нужно убедиться, что разрешённые соединения работают, а заблокированные действительно заблокированы.

### Архитектура

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
│  │ (должен  │      │         │  │ :5432    │      │
│  │  упасть) │      │         │  └──────────┘      │
│  └──────────┘      │         │                    │
└────────────────────┘         └────────────────────┘
```

### Конфигурация теста

```yaml
# probe-01: тестирует разрешённые и заблокированные соединения
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

### Проверка

```bash
curl -s "http://probe-01:8080/?detail=true" | jq '.dependencies | to_entries[] | {name: .value.name, healthy: .value.healthy, status: .value.status}'
```

Ожидаемый результат:
```json
{"name": "backend-allowed", "healthy": true, "status": "ok"}
{"name": "db-should-be-blocked", "healthy": false, "status": "connection_error"}
```

Если `db-should-be-blocked` здоров — в вашей NetworkPolicy есть брешь!

---

## 8. Health gate в CI/CD пайплайне

### Проблема

Перед деплоем новой версии проверить, что все downstream-зависимости доступны и здоровы.

### Скрипт пайплайна

```bash
#!/bin/bash
# pre-deploy-check.sh — Запуск перед деплоем

ENTRY_POINT="http://gateway.staging.svc:8080"
DEPTH=3

echo "Проверка цепочки зависимостей (depth=$DEPTH)..."

RESPONSE=$(curl -sf "$ENTRY_POINT/?detail=true&depth=$DEPTH")
if [ $? -ne 0 ]; then
    echo "FAIL: Не удалось подключиться к точке входа $ENTRY_POINT"
    exit 1
fi

# Проверка нездоровых критичных зависимостей во всём дереве
UNHEALTHY=$(echo "$RESPONSE" | jq -r '
  [.. | objects | select(.healthy == false and .critical == true)]
  | length
')

if [ "$UNHEALTHY" -gt 0 ]; then
    echo "FAIL: $UNHEALTHY критичных зависимостей нездоровы:"
    echo "$RESPONSE" | jq '.. | objects | select(.healthy == false and .critical == true) | {name, host, port, status, detail}'
    exit 1
fi

echo "PASS: Все критичные зависимости здоровы"
exit 0
```

### Пример GitLab CI

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
        echo "Критичные зависимости нездоровы!"
        echo "$RESPONSE" | jq '.. | objects | select(.healthy == false and .critical == true)'
        exit 1
      fi
      echo "Все зависимости здоровы"
  rules:
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
```

---

## 9. Мониторинг между кластерами

### Проблема

Сервисы распределены по нескольким Kubernetes-кластерам или ЦОД-ам — нужна видимость зависимостей между кластерами.

### Архитектура

```
     Кластер A (EU)                         Кластер B (US)
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

### Конфигурация

**Кластер A — eu-front:**

```bash
DEPHEALTH_NAME=eu-front
DEPHEALTH_GROUP=eu-cluster
DEPHEALTH_DEPS="us-api:http,eu-db:postgres"
DEPHEALTH_US_API_HOST=us-api.example.com    # внешний эндпоинт
DEPHEALTH_US_API_PORT=443
DEPHEALTH_US_API_CRITICAL=yes
DEPHEALTH_US_API_TLS=yes
DEPHEALTH_EU_DB_URL=postgres://eu-db.svc:5432/app
DEPHEALTH_EU_DB_CRITICAL=yes
```

**Кластер B — us-api:**

```bash
DEPHEALTH_NAME=us-api
DEPHEALTH_GROUP=us-cluster
DEPHEALTH_DEPS="us-db:postgres"
DEPHEALTH_US_DB_URL=postgres://us-db.svc:5432/app
DEPHEALTH_US_DB_CRITICAL=yes
```

### Проверка между кластерами

```bash
# Из кластера A — видимость обоих кластеров
curl -s "http://eu-front:8080/?detail=true&depth=2" | jq .
```

Ответ покажет статус локальной EU БД **и** рекурсивно запросит статус US API, который включает связность с US БД.

---

## 10. Проверка готовности к миграции БД

### Проблема

Перед запуском миграции базы данных убедиться, что все приложения могут подключиться к БД и БД принимает соединения.

### Архитектура

```
┌──────────┐  ┌──────────┐  ┌──────────┐
│ service-1│  │ service-2│  │ service-3│
│ uniproxy │  │ uniproxy │  │ uniproxy │
└────┬─────┘  └────┬─────┘  └────┬─────┘
     │             │             │
     └──────┬──────┘─────────────┘
            ▼
      ┌──────────┐
      │ postgres │  ← цель миграции
      │  :5432   │
      └──────────┘
```

### Скрипт проверки перед миграцией

```bash
#!/bin/bash
# check-db-connectivity.sh

SERVICES=("service-1:8080" "service-2:8080" "service-3:8080")
DB_NAME="main-db"
ALL_OK=true

for svc in "${SERVICES[@]}"; do
    RESPONSE=$(curl -sf "http://$svc/?detail=true" 2>/dev/null)
    if [ $? -ne 0 ]; then
        echo "WARN: Не удалось подключиться к $svc"
        ALL_OK=false
        continue
    fi

    DB_STATUS=$(echo "$RESPONSE" | jq -r ".dependencies | to_entries[] | select(.value.name == \"$DB_NAME\") | .value.healthy")
    DB_LATENCY=$(echo "$RESPONSE" | jq -r ".dependencies | to_entries[] | select(.value.name == \"$DB_NAME\") | .value.latency_ms")

    if [ "$DB_STATUS" = "true" ]; then
        echo "OK: $svc → $DB_NAME (${DB_LATENCY}мс)"
    else
        echo "FAIL: $svc → $DB_NAME нездоров"
        ALL_OK=false
    fi
done

if [ "$ALL_OK" = "true" ]; then
    echo ""
    echo "Все сервисы могут подключиться к БД. Можно запускать миграцию."
else
    echo ""
    echo "Некоторые сервисы не могут подключиться к БД. НЕ запускайте миграцию."
    exit 1
fi
```

---

## 11. Верификация аварийного восстановления

### Проблема

После активации DR-площадки убедиться, что все сервисы могут подключиться к своим зависимостям в новом окружении.

### Архитектура

```
Основная площадка (DOWN)         DR-площадка (ACTIVE)
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

### Скрипт валидации DR

```bash
#!/bin/bash
# dr-validate.sh — Запуск после переключения на DR

DR_ENTRY="http://app-proxy.dr-site:8080"

echo "=== Валидация DR-площадки ==="
echo "Время: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
echo ""

RESPONSE=$(curl -sf "$DR_ENTRY/?detail=true&depth=3")
if [ $? -ne 0 ]; then
    echo "CRITICAL: Точка входа DR недоступна!"
    exit 1
fi

echo "Статус зависимостей:"
echo "$RESPONSE" | jq -r '
  .. | objects | select(.name != null and .healthy != null) |
  (if .healthy then "  ✓ " else "  ✗ " end) + .name + " (" + .host + ":" + .port + ") — " + .status
'

TOTAL=$(echo "$RESPONSE" | jq '[.. | objects | select(.healthy != null)] | length')
HEALTHY=$(echo "$RESPONSE" | jq '[.. | objects | select(.healthy == true)] | length')
CRITICAL_DOWN=$(echo "$RESPONSE" | jq '[.. | objects | select(.healthy == false and .critical == true)] | length')

echo ""
echo "Итого: $HEALTHY/$TOTAL здоровы"

if [ "$CRITICAL_DOWN" -gt 0 ]; then
    echo "CRITICAL: $CRITICAL_DOWN критичных зависимостей НЕДОСТУПНЫ"
    exit 1
fi

echo "Валидация DR-площадки: ПРОЙДЕНА"
```

**Пример вывода:**

```
=== Валидация DR-площадки ===
Время: 2026-02-15T12:00:00Z

Статус зависимостей:
  ✓ api-backend (api.dr-site:8080) — ok
  ✓ database (db-dr.dr-site:5432) — ok
  ✓ cache (redis.dr-site:6379) — ok
  ✗ primary-db (db.primary-site:5432) — connection_error

Итого: 3/4 здоровы
Валидация DR-площадки: ПРОЙДЕНА
```

---

## 12. Проверка сервисов за Bearer-аутентификацией

### Проблема

Ваши HTTP или gRPC зависимости требуют аутентификации. Без корректных учётных данных проверки возвращают `401 Unauthorized` или `403 Forbidden`, и невозможно определить, работает ли сервис на самом деле.

### Решение

Настройте uniproxy с bearer token аутентификацией для каждой зависимости.

### Архитектура

```
uniproxy ──[Authorization: Bearer xxx]──> secure-api (HTTP 200 OK)
    │
    └──[Authorization: Bearer yyy]──> internal-svc (HTTP 200 OK)
```

### Конфигурация

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
  uniproxy:0.7.3
```

### Глобальный Bearer Token

Если все зависимости используют один и тот же токен, задайте его глобально:

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
  uniproxy:0.7.3
```

---

## 13. Безопасное хранение учётных данных в Kubernetes Secrets

### Проблема

Хранение токенов и паролей непосредственно в переменных окружения (ConfigMaps, Helm values) — риск безопасности. Необходимо использовать Kubernetes Secrets для чувствительных данных.

### Решение

Используйте паттерн суффикса `_FILE` для чтения секретов из смонтированных файлов или `secretKeyRef` в Helm values.

### Архитектура

```
K8s Secret "api-creds"
  └─ token: "eyJhbGci..."
  └─ password: "s3cret"
       │
       ├──[смонтирован как /run/secrets/token]──> uniproxy (читает через _FILE)
       └──[secretKeyRef в env]──> uniproxy (внедрено kubelet)
```

### Вариант A: Паттерн `_FILE` (Docker / K8s volume mount)

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
# deployment (подход через volume mount)
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

### Вариант B: Helm `existingSecret` (secretKeyRef)

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

Это рендерит `valueFrom.secretKeyRef` в deployment — никаких инлайн-секретов в Helm values.

---

## 14. Пользовательские API-ключи в заголовках для сторонних сервисов

### Проблема

Некоторые сторонние API используют пользовательские заголовки для аутентификации (например, `X-API-Key`, `X-Auth-Token`) вместо стандартных заголовков `Authorization`.

### Решение

Используйте опцию `HEADERS` для отправки произвольных HTTP-заголовков при health check.

### Конфигурация

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
  uniproxy:0.7.3
```

### gRPC Metadata

Для gRPC-сервисов используйте `METADATA` для добавления произвольных пар ключ-значение:

```bash
-e DEPHEALTH_GRPC_SVC_METADATA='{"x-api-key":"key123","x-request-id":"health-check"}'
```

---

## 15. Тестирование связности с LDAP / Active Directory

### Проблема

Ваши приложения зависят от корпоративного LDAP или Active Directory сервера для аутентификации. Нужно проверять связность с LDAP и мониторить его доступность наряду с другими зависимостями.

### Архитектура

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

### Конфигурация

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

### Методы проверки LDAP

| Метод | Описание | Применение |
|-------|----------|------------|
| `root_dse` | Чтение Root DSE (без аутентификации) | Базовая проверка связности |
| `anonymous_bind` | Анонимный LDAP bind | Проверка что сервер принимает соединения |
| `simple_bind` | Bind с DN + паролем | Валидация учётных данных сервисного аккаунта |
| `search` | Выполнить LDAP-поиск | Проверка доступа на чтение каталога |

### Пример метода search

```bash
-e DEPHEALTH_MYLDAP_LDAP_CHECK_METHOD=search \
-e DEPHEALTH_MYLDAP_LDAP_BIND_DN="cn=reader,dc=example,dc=com" \
-e DEPHEALTH_MYLDAP_LDAP_BIND_PASSWORD="secret" \
-e DEPHEALTH_MYLDAP_LDAP_BASE_DN="ou=Users,dc=example,dc=com" \
-e DEPHEALTH_MYLDAP_LDAP_SEARCH_FILTER="(objectClass=person)" \
-e DEPHEALTH_MYLDAP_LDAP_SEARCH_SCOPE=sub
```

### Kubernetes с Secret

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

## Советы и приёмы

### Извлечение только нездоровых зависимостей на любой глубине

```bash
curl -s "http://entry:8080/?detail=true&depth=5" | \
  jq '.. | objects | select(.healthy == false) | {name, host, port, type, status, detail, latency_ms}'
```

### Простой цикл мониторинга (без Prometheus)

```bash
while true; do
    UNHEALTHY=$(curl -sf "http://entry:8080/?detail=true&depth=3" | \
      jq '[.. | objects | select(.healthy == false and .critical == true)] | length')
    if [ "$UNHEALTHY" -gt 0 ]; then
        echo "$(date): ALERT — $UNHEALTHY критичных зависимостей недоступны!"
        # Отправить уведомление (Slack, Telegram, email)
    fi
    sleep 30
done
```

### Отчёт по задержкам

```bash
curl -s "http://entry:8080/?detail=true&depth=3" | \
  jq '.. | objects | select(.latency_ms != null) | {name, latency_ms, host, port}' | \
  jq -s 'sort_by(.latency_ms) | reverse'
```

### JSON-логирование для лог-агрегаторов

```bash
# Запуск с JSON-логированием для Loki/ELK/Fluentd
docker run \
  -e LOG_FORMAT=json \
  -e LOG_TIME_KEY=@timestamp \
  -e LOG_MESSAGE_KEY=message \
  -e LOG_LEVEL_KEY=severity \
  ...
  uniproxy:dev
```
