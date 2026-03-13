# uniproxy

[![Go Version](https://img.shields.io/badge/go-1.25-00ADD8.svg)](https://golang.org/)
[![dephealth SDK](https://img.shields.io/badge/dephealth_SDK-v0.8.2-blue.svg)](https://github.com/BigKAA/topologymetrics)
[![License](https://img.shields.io/badge/license-Apache%202.0-green.svg)](./LICENSE)

**Универсальный тестовый прокси для мониторинга здоровья зависимостей с dephealth SDK**

[English version (README.md)](./README.md)

## Обзор

**uniproxy** — легковесное Go-приложение, которое проверяет здоровье настроенных зависимостей с помощью [dephealth SDK](https://github.com/BigKAA/topologymetrics) и экспортирует метрики Prometheus. Предназначено как универсальный тестовый инструмент для валидации визуализации топологии dephealth-ui в любом окружении — Docker, Kubernetes или bare metal.

### Возможности

- Проверка здоровья зависимостей: HTTP, gRPC, PostgreSQL, MySQL, Redis, AMQP, Kafka, LDAP, TCP
- Опциональная метка `isentry` для обозначения точек входа в визуализации топологии
- Enriched Status API — детальная информация о зависимостях и рекурсивный просмотр цепочек HTTP
- Конфигурация через переменные окружения или YAML-файл (12-factor app)
- Серверная аутентификация для эндпоинтов статуса и метрик (Basic, Bearer, API Key)
- Экспорт метрик Prometheus через dephealth SDK
- Kubernetes-native с Helm chart для инстанс-ориентированного развёртывания
- Per-dependency настройка интервалов, таймаутов, TLS и др.
- Аутентификация зависимостей: Bearer token, Basic Auth, пользовательские HTTP-заголовки и gRPC metadata
- Безопасное управление секретами через суффикс `_FILE` (Kubernetes Secrets / Docker Secrets)

## Быстрый старт

### Docker

```bash
# Сборка образа
docker build -t uniproxy:0.5.0 .

# Запуск с HTTP-зависимостью
docker run -p 8080:8080 \
  -e DEPHEALTH_NAME=my-proxy \
  -e DEPHEALTH_GROUP=my-group \
  -e DEPHEALTH_DEPS="httpbin:http" \
  -e DEPHEALTH_HTTPBIN_URL="http://httpbin.org" \
  -e DEPHEALTH_HTTPBIN_CRITICAL=yes \
  uniproxy:0.5.0
```

### Docker Compose

Самый быстрый способ попробовать uniproxy с реальными зависимостями:

```bash
docker compose -f examples/docker-compose.yaml up -d

# Проверить статус — Redis и PostgreSQL должны быть healthy
curl http://localhost:8080/

# Проверить метрики Prometheus
curl http://localhost:8080/metrics | grep app_dependency_health

# Остановить
docker compose -f examples/docker-compose.yaml down
```

### Проверка эндпоинтов

```bash
# Простой статус
curl http://localhost:8080/

# Детальный статус с информацией о зависимостях
curl "http://localhost:8080/?detail=true"

# Детальный статус с рекурсивным HTTP-запросом (depth=2)
curl "http://localhost:8080/?detail=true&depth=2"

# Пробы liveness / readiness
curl http://localhost:8080/healthz
curl http://localhost:8080/readyz

# Метрики Prometheus
curl http://localhost:8080/metrics | grep app_dependency
```

### Kubernetes (Helm)

```bash
helm install my-proxy charts/uniproxy \
  --set config.name=my-proxy \
  -n my-namespace --create-namespace
```

## Конфигурация

uniproxy поддерживает два метода конфигурации:

1. **Переменные окружения** — традиционный 12-factor подход (работает всегда)
2. **YAML-файл конфигурации** — структурированный конфиг с переопределением через env vars

Переменные окружения всегда имеют приоритет над значениями из YAML.

### Конфигурация через YAML

Укажите переменную `CONFIG_FILE` с путём к YAML-файлу:

```bash
docker run -p 8080:8080 \
  -e CONFIG_FILE=/config/config.yaml \
  -v ./config.yaml:/config/config.yaml:ro \
  uniproxy:0.5.0
```

Пример YAML-файла:

```yaml
name: my-proxy
group: my-group
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

Полный пример: [examples/config.yaml](./examples/config.yaml).

**Правила приоритетов:**
- `CONFIG_FILE` env var → загрузка YAML как базовый конфиг
- Переменные окружения → переопределяют значения из YAML
- `DEPHEALTH_DEPS` env var → **заменяет** все зависимости из YAML (без слияния)
- Per-dependency env vars → overlay на существующие (загруженные из YAML) зависимости

### Глобальные переменные

| Переменная | Обязательная | По умолчанию | Описание |
|------------|:------------:|:------------:|----------|
| `CONFIG_FILE` | Нет | — | Путь к YAML-файлу конфигурации |
| `DEPHEALTH_NAME` | Да | — | Имя приложения (используется в метриках и ответах) |
| `DEPHEALTH_GROUP` | Да | — | Логическая группа для метки Prometheus |
| `DEPHEALTH_DEPS` | Нет | — | Список зависимостей через запятую: `имя1:тип1,имя2:тип2` |
| `LISTEN_ADDR` | Нет | `:8080` | Адрес HTTP-сервера |
| `LOG_FORMAT` | Нет | `text` | Формат вывода логов: `text` или `json` |
| `LOG_LEVEL` | Нет | `info` | Уровень логирования: `debug`, `info`, `warn`, `error` |
| `LOG_TIME_FORMAT` | Нет | `rfc3339nano` | Формат времени: `rfc3339`, `rfc3339nano`, `unix`, `unixmilli` |
| `LOG_ADD_SOURCE` | Нет | `false` | Включить файл:строку в вывод логов (`true`/`false`) |
| `LOG_TIME_KEY` | Нет | `time` | JSON-ключ для метки времени (действует только при `LOG_FORMAT=json`) |
| `LOG_LEVEL_KEY` | Нет | `level` | JSON-ключ для уровня лога (действует только при `LOG_FORMAT=json`) |
| `LOG_MESSAGE_KEY` | Нет | `msg` | JSON-ключ для сообщения (действует только при `LOG_FORMAT=json`) |
| `LOG_SOURCE_KEY` | Нет | `source` | JSON-ключ для расположения исходника (действует только при `LOG_FORMAT=json`) |
| `DEPHEALTH_CHECK_INTERVAL` | Нет | `10` | Интервал проверки здоровья в секундах |
| `DEPHEALTH_TIMEOUT` | Нет | SDK default | Глобальный таймаут проверки в секундах |
| `DEPHEALTH_FETCH_TIMEOUT` | Нет | `5` | Таймаут рекурсивного HTTP detail fetch в секундах |
| `DEPHEALTH_ISENTRY` | Нет | — | Добавить метку `isentry=yes` ко всем метрикам зависимостей (`yes`/`no`) |

### Переменные для каждой зависимости

Для каждой зависимости из `DEPHEALTH_DEPS` используются переменные с префиксом `DEPHEALTH_<ИМЯ>_`, где `<ИМЯ>` — имя зависимости в верхнем регистре с заменой дефисов на подчёркивания (например, `my-backend` становится `MY_BACKEND`).

| Переменная | Обязательная | Описание |
|------------|:------------:|----------|
| `DEPHEALTH_<ИМЯ>_URL` | Да* | URL подключения |
| `DEPHEALTH_<ИМЯ>_HOST` | Да* | Целевой хост (альтернатива URL) |
| `DEPHEALTH_<ИМЯ>_PORT` | Да* | Целевой порт (требуется вместе с HOST) |
| `DEPHEALTH_<ИМЯ>_CRITICAL` | Да | Критичная зависимость (`yes`/`no`) |
| `DEPHEALTH_<ИМЯ>_CHECK_INTERVAL` | Нет | Интервал проверки (секунды) |
| `DEPHEALTH_<ИМЯ>_TIMEOUT` | Нет | Таймаут проверки (секунды) |
| `DEPHEALTH_<ИМЯ>_HEALTH_PATH` | Нет | Путь для HTTP health check |
| `DEPHEALTH_<ИМЯ>_TLS` | Нет | Включить TLS (`yes`/`no`, HTTP/gRPC) |
| `DEPHEALTH_<ИМЯ>_TLS_SKIP_VERIFY` | Нет | Пропустить проверку TLS (`yes`/`no`) |
| `DEPHEALTH_<ИМЯ>_HOST_HEADER` | Нет | Пользовательский HTTP Host header (также устанавливает TLS SNI). Для сервисов за ingress/proxy |
| `DEPHEALTH_<ИМЯ>_GRPC_SERVICE_NAME` | Нет | Имя gRPC-сервиса для health check |
| `DEPHEALTH_<ИМЯ>_GRPC_AUTHORITY` | Нет | Пользовательский gRPC pseudo-header `:authority` (также устанавливает TLS SNI). Для gRPC за proxy |
| `DEPHEALTH_<ИМЯ>_POSTGRES_QUERY` | Нет | Пользовательский запрос PostgreSQL |
| `DEPHEALTH_<ИМЯ>_MYSQL_QUERY` | Нет | Пользовательский запрос MySQL |
| `DEPHEALTH_<ИМЯ>_REDIS_PASSWORD` | Нет | Пароль Redis |
| `DEPHEALTH_<ИМЯ>_REDIS_DB` | Нет | Номер базы Redis |
| `DEPHEALTH_<ИМЯ>_AMQP_URL` | Нет | URL подключения AMQP |
| `DEPHEALTH_<ИМЯ>_LDAP_CHECK_METHOD` | Нет | Метод проверки LDAP: `root_dse` (по умолчанию), `anonymous_bind`, `simple_bind`, `search` |
| `DEPHEALTH_<ИМЯ>_LDAP_BIND_DN` | Нет | DN для `simple_bind` |
| `DEPHEALTH_<ИМЯ>_LDAP_BIND_PASSWORD` | Нет | Пароль для `simple_bind` (поддерживает `_FILE`) |
| `DEPHEALTH_<ИМЯ>_LDAP_BASE_DN` | Нет | Базовый DN для метода `search` |
| `DEPHEALTH_<ИМЯ>_LDAP_SEARCH_FILTER` | Нет | LDAP-фильтр для `search` (по умолчанию: `(objectClass=*)`) |
| `DEPHEALTH_<ИМЯ>_LDAP_SEARCH_SCOPE` | Нет | Область поиска: `base`, `one`, `sub` |
| `DEPHEALTH_<ИМЯ>_LDAP_START_TLS` | Нет | Включить StartTLS для `ldap://` подключений (`yes`/`no`) |
| `DEPHEALTH_<ИМЯ>_LDAP_TLS_SKIP_VERIFY` | Нет | Пропустить проверку TLS-сертификата (`yes`/`no`) |

*Требуется либо `URL`, либо `HOST` + `PORT`.

### Серверная аутентификация

uniproxy поддерживает серверную аутентификацию для защиты собственных эндпоинтов. Аутентификация настраивается по зонам — API статуса (`/`) и метрики Prometheus (`/metrics`) могут иметь независимые настройки. Пробы (`/healthz`, `/readyz`) всегда открыты.

#### Методы серверной аутентификации

| Метод | Описание |
|-------|----------|
| `none` | Без аутентификации (по умолчанию) |
| `basic` | HTTP Basic Auth (`Authorization: Basic <base64>`) |
| `bearer` | Bearer token (`Authorization: Bearer <token>`) |
| `apikey` | API-ключ через заголовок `X-API-Key` |

#### Переменные серверной аутентификации

| Переменная | Описание |
|------------|----------|
| `AUTH_METHOD` | Глобальный метод: `none`, `basic`, `bearer`, `apikey` |
| `AUTH_USER` | Логин Basic Auth |
| `AUTH_PASS` | Пароль Basic Auth |
| `AUTH_PASS_FILE` | Чтение пароля из файла |
| `AUTH_TOKEN` | Bearer token |
| `AUTH_TOKEN_FILE` | Чтение токена из файла |
| `AUTH_API_KEY` | API-ключ |
| `AUTH_API_KEY_FILE` | Чтение API-ключа из файла |

#### Переопределения по зонам

Каждая зона (`status`, `metrics`) может переопределить глобальные настройки:

| Переменная | Описание |
|------------|----------|
| `AUTH_STATUS_METHOD` | Метод для эндпоинта `/` |
| `AUTH_STATUS_USER` | Логин для `/` |
| `AUTH_STATUS_PASS` | Пароль для `/` |
| `AUTH_STATUS_TOKEN` | Токен для `/` |
| `AUTH_STATUS_API_KEY` | API-ключ для `/` |
| `AUTH_METRICS_METHOD` | Метод для `/metrics` |
| `AUTH_METRICS_USER` | Логин для `/metrics` |
| `AUTH_METRICS_PASS` | Пароль для `/metrics` |
| `AUTH_METRICS_TOKEN` | Токен для `/metrics` |
| `AUTH_METRICS_API_KEY` | API-ключ для `/metrics` |

Все переменные `_PASS`, `_TOKEN` и `_API_KEY` поддерживают суффикс `_FILE`.

#### Примеры серверной аутентификации

```bash
# Защитить API статуса bearer-токеном, метрики оставить открытыми
docker run -p 8080:8080 \
  -e DEPHEALTH_NAME=my-proxy \
  -e DEPHEALTH_GROUP=my-group \
  -e DEPHEALTH_DEPS="httpbin:http" \
  -e DEPHEALTH_HTTPBIN_URL="http://httpbin.org" \
  -e DEPHEALTH_HTTPBIN_CRITICAL=yes \
  -e AUTH_METHOD=bearer \
  -e AUTH_TOKEN=my-secret-token \
  -e AUTH_METRICS_METHOD=none \
  uniproxy:0.5.0

# Проверка доступа
curl http://localhost:8080/                                        # 401
curl -H "Authorization: Bearer my-secret-token" http://localhost:8080/  # 200
curl http://localhost:8080/metrics                                 # 200 (override: none)
curl http://localhost:8080/healthz                                 # 200 (всегда открыт)
```

### Аутентификация зависимостей

uniproxy поддерживает аутентификацию для HTTP и gRPC зависимостей. Auth можно настроить глобально или для каждой зависимости; per-dependency настройки полностью переопределяют глобальные.

#### Методы аутентификации зависимостей

| Метод | HTTP | gRPC | Описание |
|-------|:----:|:----:|----------|
| Bearer Token | Да | Да | Заголовок `Authorization: Bearer <token>` / gRPC call credential |
| Basic Auth | Да | Да | Заголовок `Authorization: Basic <base64>` / gRPC call credential |
| Custom Headers | Да | — | Произвольные HTTP-заголовки (например, API-ключи) |
| Custom Metadata | — | Да | Произвольные пары ключ-значение gRPC metadata |

Допускается только **один метод аутентификации** на зависимость. Одновременная установка bearer token и basic auth — ошибка.

#### Глобальные переменные аутентификации

Применяются ко всем зависимостям, для которых не настроена per-dependency аутентификация.

| Переменная | Описание |
|------------|----------|
| `DEPHEALTH_BEARER_TOKEN` | Глобальный bearer token |
| `DEPHEALTH_BEARER_TOKEN_FILE` | Чтение bearer token из файла (взаимоисключающее с предыдущей) |
| `DEPHEALTH_BASIC_USER` | Глобальный логин Basic Auth |
| `DEPHEALTH_BASIC_PASS` | Глобальный пароль Basic Auth |
| `DEPHEALTH_BASIC_PASS_FILE` | Чтение пароля из файла (взаимоисключающее с предыдущей) |
| `DEPHEALTH_HEADERS` | Глобальные HTTP-заголовки (JSON: `{"Key":"Value"}`) |
| `DEPHEALTH_METADATA` | Глобальные gRPC metadata (JSON: `{"key":"value"}`) |

Глобальные headers применяются только к HTTP-зависимостям; глобальные metadata — только к gRPC.

#### Per-dependency переменные аутентификации

| Переменная | Описание |
|------------|----------|
| `DEPHEALTH_<ИМЯ>_BEARER_TOKEN` | Bearer token для этой зависимости |
| `DEPHEALTH_<ИМЯ>_BEARER_TOKEN_FILE` | Чтение bearer token из файла |
| `DEPHEALTH_<ИМЯ>_BASIC_USER` | Логин Basic Auth |
| `DEPHEALTH_<ИМЯ>_BASIC_PASS` | Пароль Basic Auth |
| `DEPHEALTH_<ИМЯ>_BASIC_PASS_FILE` | Чтение пароля из файла |
| `DEPHEALTH_<ИМЯ>_HEADERS` | HTTP-заголовки (JSON-строка) |
| `DEPHEALTH_<ИМЯ>_METADATA` | gRPC metadata (JSON-строка) |

#### Паттерн суффикса `_FILE`

Для любой секретной переменной (`BEARER_TOKEN`, `BASIC_PASS`) можно добавить суффикс `_FILE`, чтобы прочитать значение из файла вместо переменной окружения. Рекомендуемый подход для Kubernetes Secrets и Docker Secrets:

```bash
# Монтирование K8s Secret как файл
-e DEPHEALTH_API_BEARER_TOKEN_FILE=/run/secrets/api-token
```

Правила:
- Одновременная установка `VAR` и `VAR_FILE` — ошибка
- Содержимое файла очищается от пробелов в начале и конце
- Файл должен существовать и быть доступен для чтения

#### Правила валидации

1. **Один метод на зависимость**: bearer token, basic auth, headers или metadata — выберите один
2. **Полный basic auth**: `BASIC_USER` и `BASIC_PASS` должны быть заданы вместе
3. **Проверка типа**: `HEADERS` только для HTTP; `METADATA` только для gRPC
4. **Без VAR + VAR_FILE**: нельзя задавать одновременно инлайн-значение и ссылку на файл

#### Пример аутентификации

```bash
docker run -p 8080:8080 \
  -e DEPHEALTH_NAME=auth-proxy \
  -e DEPHEALTH_GROUP=my-group \
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

### Поддерживаемые типы зависимостей

`http`, `grpc`, `tcp`, `postgres`, `mysql`, `redis`, `amqp`, `kafka`, `ldap`

### Пример конфигурации LDAP

```bash
docker run -p 8080:8080 \
  -e DEPHEALTH_NAME=ldap-monitor \
  -e DEPHEALTH_GROUP=my-group \
  -e DEPHEALTH_DEPS="corp-ldap:ldap" \
  -e DEPHEALTH_CORP_LDAP_URL="ldap://ldap.example.com:389" \
  -e DEPHEALTH_CORP_LDAP_CRITICAL=yes \
  -e DEPHEALTH_CORP_LDAP_LDAP_CHECK_METHOD=simple_bind \
  -e DEPHEALTH_CORP_LDAP_LDAP_BIND_DN="cn=healthcheck,dc=example,dc=com" \
  -e DEPHEALTH_CORP_LDAP_LDAP_BIND_PASSWORD="secret" \
  -e DEPHEALTH_CORP_LDAP_LDAP_START_TLS=yes \
  uniproxy:latest
```

### Пользовательский Host Header (маршрутизация через Ingress/Proxy)

При проверке здоровья сервиса, находящегося за ingress-контроллером или reverse proxy, по IP-адресу необходимо установить правильный заголовок `Host` для маршрутизации виртуального хоста. Для gRPC-сервисов за прокси используется pseudo-header `:authority`. При TLS эти опции также устанавливают SNI (ServerName).

**Переменные окружения:**

```bash
docker run -p 8080:8080 \
  -e DEPHEALTH_NAME=proxy-check \
  -e DEPHEALTH_GROUP=my-group \
  -e DEPHEALTH_DEPS="web-app:http,grpc-api:grpc" \
  -e DEPHEALTH_WEB_APP_HOST="192.168.1.100" \
  -e DEPHEALTH_WEB_APP_PORT=443 \
  -e DEPHEALTH_WEB_APP_CRITICAL=yes \
  -e DEPHEALTH_WEB_APP_TLS=yes \
  -e DEPHEALTH_WEB_APP_TLS_SKIP_VERIFY=yes \
  -e DEPHEALTH_WEB_APP_HOST_HEADER="app.example.com" \
  -e DEPHEALTH_GRPC_API_HOST="10.0.0.50" \
  -e DEPHEALTH_GRPC_API_PORT=8443 \
  -e DEPHEALTH_GRPC_API_CRITICAL=yes \
  -e DEPHEALTH_GRPC_API_TLS=yes \
  -e DEPHEALTH_GRPC_API_GRPC_AUTHORITY="grpc.example.com" \
  uniproxy:latest
```

**YAML-конфигурация:**

```yaml
dependencies:
  - name: web-app
    type: http
    host: "192.168.1.100"
    port: "443"
    critical: true
    tls: true
    tlsSkipVerify: true
    hostHeader: "app.example.com"
  - name: grpc-api
    type: grpc
    host: "10.0.0.50"
    port: "8443"
    critical: true
    tls: true
    tlsSkipVerify: true
    grpcAuthority: "grpc.example.com"
```

> **Примечание:** `hostHeader` конфликтует с ключом `Host` в пользовательских заголовках, а `grpcAuthority` — с `:authority` в gRPC metadata. SDK вернёт ошибку при одновременной установке обоих.

### Пример конфигурации

```bash
docker run -p 8080:8080 \
  -e DEPHEALTH_NAME=frontend \
  -e DEPHEALTH_GROUP=my-group \
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

## API-эндпоинты

### `GET /` — Простой статус

Возвращает базовый статус здоровья для обратной совместимости.

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

### `GET /?detail=true` — Обогащённый статус

Возвращает детальную информацию о зависимостях из `HealthDetails()` API SDK.

**Параметры запроса:**

| Параметр | По умолчанию | Описание |
|----------|:------------:|----------|
| `detail` | — | Установите `true` для включения детального ответа |
| `depth` | `1` | Глубина рекурсии для HTTP-зависимостей (0–10) |

```json
{
  "name": "frontend",
  "podName": "frontend-7b9f4-xk2lm",
  "namespace": "production",
  "dependencies": {
    "backend:backend.svc:8080": {
      "healthy": true,
      "status": "ok",
      "detail": "ok",
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

**Как работает рекурсивный fetch:**

- Для HTTP-зависимостей с `depth > 0` uniproxy выполняет HTTP-запрос к зависимости: `http://<host>:<port>/?detail=true&depth=N-1`
- Ответ включается в поле `response` зависимости
- Не-HTTP зависимости никогда не имеют поля `response`
- Если downstream недоступен, поле `response` отсутствует
- `depth=0` полностью отключает рекурсивный fetch
- `DEPHEALTH_FETCH_TIMEOUT` управляет таймаутом всех параллельных запросов

**Категории статусов:** `ok`, `timeout`, `connection_error`, `dns_error`, `auth_error`, `tls_error`, `unhealthy`, `error`, `unknown`

### `GET /healthz` — Проба Liveness

Всегда возвращает `200 OK` с телом `ok`.

### `GET /readyz` — Проба Readiness

Всегда возвращает `200 OK` с телом `ok`.

### `GET /metrics` — Метрики Prometheus

dephealth SDK экспортирует следующие метрики Prometheus:

| Метрика | Тип | Описание |
|---------|-----|----------|
| `app_dependency_health` | Gauge | Статус здоровья: 1 = healthy, 0 = unhealthy |
| `app_dependency_latency_seconds` | Histogram | Задержка проверки в секундах. Бакеты: 1ms, 5ms, 10ms, 50ms, 100ms, 500ms, 1s, 5s |
| `app_dependency_status` | Gauge | Категория результата последней проверки (enum-паттерн — ровно одно значение status установлено в 1, остальные в 0) |
| `app_dependency_status_detail` | Gauge | Детальная причина результата последней проверки (state-set паттерн — всегда 1 с меткой detail) |

**Базовые метки** (все метрики): `name`, `group`, `dependency`, `type`, `host`, `port`, `critical`

Дополнительные метки:
- `app_dependency_status` добавляет метку **`status`** с возможными значениями: `ok`, `timeout`, `connection_error`, `dns_error`, `auth_error`, `tls_error`, `unhealthy`, `error`
- `app_dependency_status_detail` добавляет метку **`detail`** с человекочитаемым описанием причины

## Структура проекта

```
uniproxy/
├── main.go                      # Точка входа, инициализация SDK
├── internal/
│   ├── auth/                    # Серверная аутентификация (Basic/Bearer/APIKey)
│   ├── config/                  # Парсинг env vars + YAML конфигурации
│   ├── logging/                 # Настройка структурированного логирования
│   └── server/                  # HTTP-обработчики, рекурсивный fetch
├── charts/
│   └── uniproxy/                # Стандартный Helm chart (single-instance)
│       ├── Chart.yaml
│       ├── values.yaml
│       └── templates/           # Deployment, Service, Ingress, HTTPRoute, ConfigMap
├── deploy/
│   └── helm/
│       └── uniproxy/            # Legacy multi-instance Helm chart
├── examples/
│   ├── config.yaml              # Полный пример YAML-конфигурации
│   └── docker-compose.yaml      # Docker Compose quick start (uniproxy + Redis + PostgreSQL)
├── Dockerfile                   # Многоэтапная сборка Docker
├── go.mod
├── LICENSE
└── NOTICE
```

## Развёртывание в Helm

Стандартный Helm chart (`charts/uniproxy/`) обеспечивает single-instance развёртывание с полной поддержкой типов Service, Ingress, Gateway API, серверной аутентификации и YAML-конфигурации.

```bash
# Установка с минимальной конфигурацией
helm install my-proxy charts/uniproxy \
  --set config.name=my-proxy \
  -n monitoring --create-namespace

# Установка с файлом значений
helm install my-proxy charts/uniproxy \
  -f my-values.yaml \
  -n monitoring --create-namespace

# Обновление
helm upgrade my-proxy charts/uniproxy -f my-values.yaml -n monitoring

# Отладка рендеринга шаблонов
helm template my-proxy charts/uniproxy -f my-values.yaml
```

### Типы Service

**ClusterIP** (по умолчанию):

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

### Параметры Helm

| Параметр | По умолчанию | Описание |
|----------|:------------:|----------|
| `replicaCount` | `1` | Количество реплик |
| `image.repository` | `container-registry.cloud.yandex.net/crpklna5l8v5m7c0ipst/uniproxy` | Репозиторий образа |
| `image.tag` | `""` (appVersion) | Тег образа |
| `config.name` | `""` (имя релиза) | `DEPHEALTH_NAME` — имя приложения |
| `config.group` | `""` | `DEPHEALTH_GROUP` — логическая группа |
| `config.listenAddr` | `":8080"` | `LISTEN_ADDR` |
| `config.checkInterval` | `"10"` | `DEPHEALTH_CHECK_INTERVAL` |
| `config.timeout` | `""` | `DEPHEALTH_TIMEOUT` |
| `config.fetchTimeout` | `"5"` | `DEPHEALTH_FETCH_TIMEOUT` |
| `config.deps` | `""` | `DEPHEALTH_DEPS` (напр. `"backend:http,cache:redis"`) |
| `log.format` | `""` | `LOG_FORMAT`: `text` или `json` |
| `log.level` | `""` | `LOG_LEVEL`: `debug`, `info`, `warn`, `error` |
| `configFile.enabled` | `false` | Монтировать YAML-конфиг через ConfigMap |
| `configFile.content` | — | Содержимое YAML-конфига |
| `serverAuth.method` | `"none"` | Серверная auth: `none`, `basic`, `bearer`, `apikey` |
| `serverAuth.existingSecret` | — | K8s Secret для учётных данных |
| `serverAuth.status` | — | Переопределение для эндпоинта `/` |
| `serverAuth.metrics` | — | Переопределение для эндпоинта `/metrics` |
| `extraEnv` | `[]` | Дополнительные переменные окружения |
| `service.type` | `ClusterIP` | Тип Service: `ClusterIP`, `NodePort`, `LoadBalancer` |
| `service.port` | `8080` | Порт Service |
| `ingress.enabled` | `false` | Включить ресурс Ingress |
| `gateway.enabled` | `false` | Включить HTTPRoute (Gateway API) |

Все доступные параметры: [`charts/uniproxy/values.yaml`](./charts/uniproxy/values.yaml).

### Legacy Multi-Instance Chart

Чарт в `deploy/helm/uniproxy/` поддерживает развёртывание нескольких экземпляров uniproxy из одного Helm-релиза. Подробности: [deploy/helm/uniproxy/values.yaml](./deploy/helm/uniproxy/values.yaml).

## Тестирование

```bash
# Все тесты
go test ./...

# С покрытием
go test -cover ./...

# Конкретный пакет
go test -v ./internal/server
```

## Сценарии использования

Подробные сценарии с примерами: [Сценарии использования](./docs/use-cases.ru.md)

- Рекурсивная диагностика цепочек без Prometheus (один `curl` → полное дерево зависимостей)
- Цепочки микросервисов в Kubernetes, Docker Compose, bare metal / ВМ
- Смешанные окружения (K8s + ВМ) со сквозной видимостью
- Sidecar для legacy-приложений без интеграции SDK
- Тестирование сетевых политик / файрволов, health gate в CI/CD
- Мониторинг между кластерами, готовность к миграции БД, верификация DR
- Аутентификация: bearer token, K8s Secrets, пользовательские API-ключи

## Интеграция с dephealth-ui

uniproxy предназначен для работы с [dephealth-ui](https://github.com/BigKAA/dephealth-ui) для визуализации топологии.

1. Разверните экземпляры uniproxy в Kubernetes-кластере
2. Настройте Prometheus для сбора метрик с подов uniproxy
3. Укажите dephealth-ui на ваш экземпляр Prometheus
4. Используйте `?detail=true&depth=N` для глубокой инспекции цепочек зависимостей

## Лицензия

Apache License 2.0 — подробности в [LICENSE](./LICENSE).

## Связанные проекты

- [dephealth SDK](https://github.com/BigKAA/topologymetrics) — Библиотека проверки здоровья и метрик
- [dephealth-ui](https://github.com/BigKAA/dephealth-ui) — Веб-интерфейс для визуализации топологии
