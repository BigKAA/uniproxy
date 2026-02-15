# uniproxy

[![Go Version](https://img.shields.io/badge/go-1.25-00ADD8.svg)](https://golang.org/)
[![dephealth SDK](https://img.shields.io/badge/dephealth_SDK-v0.4.1-blue.svg)](https://github.com/BigKAA/topologymetrics)
[![License](https://img.shields.io/badge/license-Apache%202.0-green.svg)](./LICENSE)

**Универсальный тестовый прокси для мониторинга здоровья зависимостей с dephealth SDK**

[English version (README.md)](./README.md)

## Обзор

**uniproxy** — легковесное Go-приложение, которое проверяет здоровье настроенных зависимостей с помощью [dephealth SDK](https://github.com/BigKAA/topologymetrics) и экспортирует метрики Prometheus. Предназначено как универсальный тестовый инструмент для валидации визуализации топологии dephealth-ui в любом окружении — Docker, Kubernetes или bare metal.

### Возможности

- Проверка здоровья зависимостей: HTTP, gRPC, PostgreSQL, MySQL, Redis, AMQP, Kafka, TCP
- Enriched Status API — детальная информация о зависимостях и рекурсивный просмотр цепочек HTTP
- Конфигурация через переменные окружения (12-factor app)
- Экспорт метрик Prometheus через dephealth SDK
- Kubernetes-native с Helm chart для инстанс-ориентированного развёртывания
- Per-dependency настройка интервалов, таймаутов, TLS и др.

## Быстрый старт

### Docker

```bash
# Сборка образа
docker build -t uniproxy:0.4.1 .

# Запуск с HTTP-зависимостью
docker run -p 8080:8080 \
  -e DEPHEALTH_NAME=my-proxy \
  -e DEPHEALTH_DEPS="httpbin:http" \
  -e DEPHEALTH_HTTPBIN_URL="http://httpbin.org" \
  -e DEPHEALTH_HTTPBIN_CRITICAL=yes \
  uniproxy:0.4.1
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
helm install uniproxy-01 ./deploy/helm/uniproxy \
  -f ./deploy/helm/uniproxy/instances/ns1-homelab.yaml \
  -n uniproxy-ns1 --create-namespace
```

## Конфигурация

Вся конфигурация выполняется через переменные окружения.

### Глобальные переменные

| Переменная | Обязательная | По умолчанию | Описание |
|------------|:------------:|:------------:|----------|
| `DEPHEALTH_NAME` | Да | — | Имя приложения (используется в метриках и ответах) |
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
| `DEPHEALTH_<ИМЯ>_GRPC_SERVICE_NAME` | Нет | Имя gRPC-сервиса для health check |
| `DEPHEALTH_<ИМЯ>_POSTGRES_QUERY` | Нет | Пользовательский запрос PostgreSQL |
| `DEPHEALTH_<ИМЯ>_MYSQL_QUERY` | Нет | Пользовательский запрос MySQL |
| `DEPHEALTH_<ИМЯ>_REDIS_PASSWORD` | Нет | Пароль Redis |
| `DEPHEALTH_<ИМЯ>_REDIS_DB` | Нет | Номер базы Redis |
| `DEPHEALTH_<ИМЯ>_AMQP_URL` | Нет | URL подключения AMQP |

*Требуется либо `URL`, либо `HOST` + `PORT`.

### Поддерживаемые типы зависимостей

`http`, `grpc`, `tcp`, `postgres`, `mysql`, `redis`, `amqp`, `kafka`

### Пример конфигурации

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

**Базовые метки** (все метрики): `name`, `dependency`, `type`, `host`, `port`, `critical`

Дополнительные метки:
- `app_dependency_status` добавляет метку **`status`** с возможными значениями: `ok`, `timeout`, `connection_error`, `dns_error`, `auth_error`, `tls_error`, `unhealthy`, `error`
- `app_dependency_status_detail` добавляет метку **`detail`** с человекочитаемым описанием причины

## Структура проекта

```
uniproxy/
├── main.go                      # Точка входа, инициализация SDK
├── internal/
│   ├── config/
│   │   ├── config.go            # Парсинг переменных окружения
│   │   └── config_test.go       # Тесты конфигурации
│   └── server/
│       ├── server.go            # HTTP-обработчики, типы
│       ├── server_test.go       # Тесты сервера (20 тестов)
│       ├── fetch.go             # Рекурсивный HTTP fetch
│       └── fetch_test.go        # Тесты fetch
├── deploy/
│   └── helm/
│       └── uniproxy/
│           ├── Chart.yaml       # Метаданные чарта (v0.4.1)
│           ├── values.yaml      # Значения по умолчанию
│           ├── templates/       # Шаблоны манифестов K8s
│           └── instances/       # Конфигурации экземпляров
├── plans/                       # Планы разработки
├── Dockerfile                   # Многоэтапная сборка Docker
├── go.mod
├── LICENSE
└── NOTICE
```

## Развёртывание в Helm

Helm chart поддерживает инстанс-ориентированное развёртывание — несколько экземпляров uniproxy с разными конфигурациями зависимостей в одном namespace.

```bash
# Установка
helm install uniproxy-01 ./deploy/helm/uniproxy \
  -f ./deploy/helm/uniproxy/instances/ns1-homelab.yaml \
  -n uniproxy-ns1 --create-namespace

# Обновление
helm upgrade uniproxy-01 ./deploy/helm/uniproxy \
  -f ./deploy/helm/uniproxy/instances/ns1-homelab.yaml \
  -n uniproxy-ns1

# Отладка рендеринга шаблонов
helm template uniproxy-01 ./deploy/helm/uniproxy \
  -f ./deploy/helm/uniproxy/instances/ns1-homelab.yaml
```

### Параметры Helm

| Параметр | По умолчанию | Описание |
|----------|:------------:|----------|
| `global.pushRegistry` | `""` | Префикс container registry |
| `image.name` | `uniproxy` | Имя образа |
| `image.tag` | `latest` | Тег образа |
| `checkInterval` | `"10"` | Интервал проверки (секунды) |
| `timeout` | `""` | Глобальный таймаут проверки (секунды) |
| `fetchTimeout` | `"5"` | Таймаут рекурсивного HTTP fetch (секунды) |

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
