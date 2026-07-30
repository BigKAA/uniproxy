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
- **Graceful shutdown** с настраиваемым таймаутом (30с по умолчанию) — активные запросы завершаются корректно
- **Circuit breaker** для downstream HTTP-запросов — предотвращает каскадные сбои и снижает нагрузку на проблемные зависимости
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
docker build -t uniproxy:0.7.3 .

# Запуск с HTTP-зависимостью
docker run -p 8080:8080 \
  -e DEPHEALTH_NAME=my-proxy \
  -e DEPHEALTH_GROUP=my-group \
  -e DEPHEALTH_DEPS="httpbin:http" \
  -e DEPHEALTH_HTTPBIN_URL="http://httpbin.org" \
  -e DEPHEALTH_HTTPBIN_CRITICAL=yes \
  uniproxy:0.7.3
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
  uniproxy:0.7.3
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
| `SHUTDOWN_TIMEOUT` | Нет | `30` | Graceful shutdown timeout (формат Go duration, напр. `30s`, `1m`) |
| `CIRCUIT_BREAKER_MAX_FAILURES` | Нет | `5` | Открыть circuit после N последовательных ошибок |
| `CIRCUIT_BREAKER_TIMEOUT` | Нет | `60s` | Время в открытом состоянии до half-open (Go duration) |
| `CIRCUIT_BREAKER_HALF_OPEN_LIMIT` | Нет | `3` | Тестовых запросов в состоянии half-open |
| `HTTP_TRANSPORT_MAX_IDLE_CONNS` | Нет | `100` | Всего idle HTTP соединений для всех хостов |
| `HTTP_TRANSPORT_MAX_IDLE_CONNS_PER_HOST` | Нет | `10` | Idle соединений на один хост |
| `HTTP_TRANSPORT_IDLE_CONN_TIMEOUT` | Нет | `90s` | Таймаут idle соединений (Go duration) |
| `TLS_ENABLED` | Нет | `false` | Включить HTTPS сервер (`yes`/`no`) |
| `TLS_CERT_FILE` | Нет* | — | Путь к файлу TLS-сертификата (PEM) |
| `TLS_KEY_FILE` | Нет* | — | Путь к файлу закрытого ключа (PEM) |
| `TLS_CERT_DATA` | Нет* | — | Inline содержимое TLS-сертификата (альтернатива FILE) |
| `TLS_KEY_DATA` | Нет* | — | Inline содержимое закрытого ключа (альтернатива FILE) |
| `TLS_LISTEN_ADDR` | Нет | `:8443` | Адрес HTTPS-сервера (когда TLS включен) |

*Когда `TLS_ENABLED=yes`, требуется либо `TLS_CERT_FILE`+`TLS_KEY_FILE`, либо `TLS_CERT_DATA`+`TLS_KEY_DATA`.

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

### HTTPS сервер (TLS)

uniproxy поддерживает HTTPS-сервер с TLS-терминацией. При включении сервер слушает на порту 8443 по умолчанию (настраивается через `TLS_LISTEN_ADDR`).

#### Способы конфигурации TLS

| Способ | Описание |
|--------|----------|
| **Файл** | Укажите пути к файлам сертификата и ключа через `TLS_CERT_FILE` и `TLS_KEY_FILE` |
| **Inline** | Укажите содержимое сертификата и ключа напрямую через `TLS_CERT_DATA` и `TLS_KEY_DATA` |

Inline-режим полезен для Kubernetes-развёртываний, где сертификаты хранятся в Secrets и монтируются как файлы или передаются через переменные окружения.

#### Переменные TLS

| Переменная | Обязательная | Описание |
|------------|:------------:|----------|
| `TLS_ENABLED` | Да | Включить HTTPS: `yes` или `no` |
| `TLS_CERT_FILE` | Да* | Путь к файлу сертификата (PEM) |
| `TLS_KEY_FILE` | Да* | Путь к файлу закрытого ключа (PEM) |
| `TLS_CERT_DATA` | Да* | Inline содержимое сертификата |
| `TLS_KEY_DATA` | Да* | Inline содержимое закрытого ключа |
| `TLS_LISTEN_ADDR` | Нет | Адрес HTTPS-сервера (по умолчанию: `:8443`) |

*Требуется либо файловый (`TLS_CERT_FILE`+`TLS_KEY_FILE`), либо inline (`TLS_CERT_DATA`+`TLS_KEY_DATA`) при включённом TLS.

#### Примеры TLS

**Файл-based сертификаты:**

```bash
docker run -p 8443:8443 \
  -e DEPHEALTH_NAME=my-proxy \
  -e DEPHEALTH_GROUP=my-group \
  -e DEPHEALTH_DEPS="httpbin:http" \
  -e DEPHEALTH_HTTPBIN_URL="http://httpbin.org" \
  -e DEPHEALTH_HTTPBIN_CRITICAL=yes \
  -e TLS_ENABLED=yes \
  -e TLS_CERT_FILE=/certs/server.crt \
  -e TLS_KEY_FILE=/certs/server.key \
  -v ./certs:/certs:ro \
  uniproxy:0.7.3
```

**Inline сертификаты (K8s Secrets):**

```bash
# Сертификаты передаются как переменные окружения из K8s Secrets
docker run -p 8443:8443 \
  -e DEPHEALTH_NAME=my-proxy \
  -e DEPHEALTH_GROUP=my-group \
  -e DEPHEALTH_DEPS="httpbin:http" \
  -e DEPHEALTH_HTTPBIN_URL="http://httpbin.org" \
  -e DEPHEALTH_HTTPBIN_CRITICAL=yes \
  -e TLS_ENABLED=yes \
  -e TLS_CERT_DATA="$(cat /var/run/secrets/tls.crt)" \
  -e TLS_KEY_DATA="$(cat /var/run/secrets/tls.key)" \
  uniproxy:0.7.3
```

**YAML-конфигурация:**

```yaml
tls:
  enabled: true
  certData: |
    -----BEGIN CERTIFICATE-----
    MIIDXTCCAkWgAwIBAgIJAKZ...
    -----END CERTIFICATE-----
  keyData: |
    -----BEGIN PRIVATE KEY-----
    MIIEvQIBADANBgkqhkiG9w0B...
    -----END PRIVATE KEY-----
  listenAddr: ":8443"
```

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
  uniproxy:0.7.3

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

#### Произвольные HTTP-заголовки

`DEPHEALTH_HEADERS` (глобально) и `DEPHEALTH_<ИМЯ>_HEADERS` (per-dependency) отправляют произвольные HTTP-заголовки с каждым запросом health check — например, API-ключи (`X-API-Key`), идентификаторы трассировки (`X-Request-Id`), согласование содержимого (`Accept`) или переопределение `User-Agent` по умолчанию. Значение — JSON-объект (`{"Key":"Value"}`); аналогично для gRPC используется `DEPHEALTH_*_METADATA`.

**Переменная окружения** (значение — JSON-строка):

```bash
DEPHEALTH_API_HEADERS='{"X-API-Key":"key-123","X-Request-Id":"health-check"}'
```

**YAML-конфиг** (нативный map под `auth.headers`):

```yaml
dependencies:
  - name: api
    type: http
    url: "https://api.example.com/health"
    auth:
      headers:
        X-API-Key: "key-123"
        X-Request-Id: "health-check"
```

Замечания:
- Только для HTTP. В per-dependency конфигурации headers полностью заменяют глобальные `DEPHEALTH_HEADERS`.
- Считается методом аутентификации: нельзя совмещать с bearer token, basic auth или metadata в одной зависимости.
- В Kubernetes YAML-форма через `CONFIG_FILE` избавляет от проблем с экранированием JSON внутри Helm values или ConfigMap.

#### Пример аутентификации

```bash
docker run -p 8080:8080 \
  -e DEPHEALTH_NAME=auth-proxy \
  -e DEPHEALTH_GROUP=my-group \
  -e DEPHEALTH_DEPS="secure-api:http,grpc-svc:grpc,third-party-api:http" \
  -e DEPHEALTH_SECURE_API_URL="https://api.example.com" \
  -e DEPHEALTH_SECURE_API_CRITICAL=yes \
  -e DEPHEALTH_SECURE_API_BEARER_TOKEN="eyJhbGciOi..." \
  -e DEPHEALTH_THIRD_PARTY_API_URL="https://api.example.com/health" \
  -e DEPHEALTH_THIRD_PARTY_API_CRITICAL=no \
  -e DEPHEALTH_THIRD_PARTY_API_HEADERS='{"X-API-Key":"key-123","X-Request-Id":"health-check"}' \
  -e DEPHEALTH_GRPC_SVC_HOST=grpc.example.com \
  -e DEPHEALTH_GRPC_SVC_PORT=443 \
  -e DEPHEALTH_GRPC_SVC_CRITICAL=yes \
  -e DEPHEALTH_GRPC_SVC_BASIC_USER=admin \
  -e DEPHEALTH_GRPC_SVC_BASIC_PASS=secret \
  uniproxy:0.7.3
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

### Graceful Shutdown

При получении SIGINT/SIGTERM uniproxy выполняет корректное завершение:

1. Останавливает планирование проверок здоровья (новые проверки не запускаются)
2. Ждёт завершения активных HTTP-запросов (до 30 секунд)
3. Закрывает HTTP-сервер

```
# Отправляем SIGTERM
kubectl delete pod uniproxy-xxx --grace-period=30

# В логах видно graceful shutdown
level=info msg="shutting down"
level=info msg="stopping health checks"
level=info msg="shutting down HTTP server" timeout=30s
level=info msg="HTTP server stopped gracefully"
```

### Circuit Breaker

Для рекурсивных HTTP-запросов (`?detail=true&depth=N`) uniproxy реализует паттерн circuit breaker для каждого downstream endpoint:

| Состояние | Поведение |
|-----------|-----------|
| **Closed** | Нормальные запросы проходят; ошибки считаются |
| **Open** | После 5 последовательных ошибок; запросы отклоняются без сетевого вызова |
| **Half-Open** | Через 60с в Open; разрешены 3 тестовых запроса |

Circuit breaker предотвращает каскадные сбои при проблемах с downstream-сервисами.

**HTTP Connection Pooling:**
- До 100 idle соединений для всех хостов
- До 10 idle соединений на хост
- 90-секундный таймаут idle соединений
- Переиспользование соединений между запросами

**Метрики:**
```prometheus
# Состояние circuit breaker (0=closed, 1=half-open, 2=open)
uniproxy_circuit_breaker_state{downstream="backend:8080"}

# Счётчики запросов по результату
uniproxy_circuit_breaker_requests_total{downstream="backend:8080", state="success"}
uniproxy_circuit_breaker_requests_total{downstream="backend:8080", state="failure"}

# Метрики HTTP пула
uniproxy_http_pool_idle_connections
uniproxy_http_pool_requests_total
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
  uniproxy:0.7.3
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

- Для HTTP-зависимостей с `depth > 0` uniproxy выполняет HTTP/HTTPS-запрос к зависимости: `<scheme>://<host>:<port>/?detail=true&depth=N-1`
- **Автоопределение схемы**: Порт 443 → HTTPS, остальные порты → HTTP (безопасность по умолчанию)
- Ответ включается в поле `response` зависимости
- Не-HTTP зависимости никогда не имеют поля `response`
- Если downstream недоступен, поле `response` отсутствует
- `depth=0` полностью отключает рекурсивный fetch
- `DEPHEALTH_FETCH_TIMEOUT` управляет таймаутом всех параллельных запросов

### Настройки устойчивости

Текущие настройки по умолчанию (настраиваются через YAML):

| Настройка | По умолчанию | Описание |
|-----------|-------------|---------|
| Graceful shutdown timeout | 30s | Время ожидания активных запросов |
| Circuit breaker max failures | 5 | Ошибок до открытия circuit |
| Circuit breaker timeout | 60s | Время в открытом состоянии |
| Circuit breaker half-open limit | 3 | Тестовых запросов в half-open |
| HTTP pool max idle | 100 | Всего idle соединений |
| HTTP pool per host | 10 | Idle соединений на хост |
| HTTP pool idle timeout | 90s | Таймаут idle соединений |

Примеры конфигурации: `examples/config.yaml`.

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
