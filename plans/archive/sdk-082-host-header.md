# План разработки: Обновление SDK до v0.8.2 — поддержка кастомного Host/Authority

## 📋 Метаданные

- **Версия плана**: 1.5.0
- **Дата создания**: 2026-03-13
- **Последнее обновление**: 2026-03-13
- **Статус**: In Progress

---

## 📚 История версий

- **v1.5.0** (2026-03-13): Phase 3 шаги 3.1–3.3 выполнены (lint, build, Docker test). Ожидает 3.4 (K8s)
- **v1.4.0** (2026-03-13): Phase 2 завершена — документация, примеры, README.md/README.ru.md/CLAUDE.md обновлены
- **v1.3.0** (2026-03-13): Phase 1 завершена, все критерии выполнены (кроме make lint)
- **v1.2.0** (2026-03-13): Phase 1 шаги 1.2–1.6 выполнены, ожидает SDK v0.8.2 для шага 1.1
- **v1.1.0** (2026-03-13): Исправлен шаг 1.4 (YAML-парсинг через yaml.go, не config.go), добавлен файл yaml.go в Modifies для 1.2, уточнены зависимости
- **v1.0.0** (2026-03-13): Начальная версия плана

---

## 📍 Текущий статус

- **Активная фаза**: Phase 3
- **Активный подпункт**: 3.4 Тестирование в Kubernetes
- **Последнее обновление**: 2026-03-13
- **Примечание**: Phase 1–2 завершены. Phase 3: lint/test/build/Docker-тест пройдены (3.1–3.3). Осталось: 3.4 (K8s деплой).

---

## 📑 Оглавление

- [x] [Phase 1: Обновление SDK и конфигурации](#phase-1-обновление-sdk-и-конфигурации)
- [x] [Phase 2: Документация и примеры](#phase-2-документация-и-примеры)
- [ ] [Phase 3: Сборка и тестирование](#phase-3-сборка-и-тестирование)

---

## Контекст

### Что нового в SDK v0.8.2

Библиотека `github.com/BigKAA/topologymetrics/sdk-go` v0.8.2 добавляет две новые опции:

1. **`WithHTTPHostHeader(host string)`** — переопределяет HTTP-заголовок `Host` в запросах health check. При TLS также устанавливает SNI (ServerName). Полезно при обращении к сервисам за ingress/gateway по IP-адресу.

2. **`WithGRPCAuthority(authority string)`** — переопределяет pseudo-header `:authority` в gRPC-вызовах. При TLS также устанавливает SNI. Полезно для gRPC-сервисов за reverse proxy.

### Новые поля в SDK `DependencyConfig`

```go
HTTPHostHeader string // overrides Host header (and TLS SNI when HTTPS)
GRPCAuthority  string // overrides :authority pseudo-header (and TLS SNI when TLS)
```

### Новые SDK env vars

- `DEPHEALTH_<DEP>_HOST_HEADER` — HTTP Host header override
- `DEPHEALTH_<DEP>_GRPC_AUTHORITY` — gRPC authority override

### Валидация в SDK

- `WithHTTPHostHeader` конфликтует с `Host` ключом в `WithHTTPHeaders` → ошибка `conflicting Host header`
- `WithGRPCAuthority` конфликтует с `:authority` ключом в `WithGRPCMetadata` → ошибка `conflicting :authority`

---

## Phase 1: Обновление SDK и конфигурации

**Dependencies**: None
**Status**: ✅ Done

### Описание

Обновить зависимость SDK до v0.8.2 и добавить поддержку новых конфигурационных параметров `hostHeader` (HTTP) и `grpcAuthority` (gRPC) в конфигурационную модель uniproxy (env vars + YAML).

### Подпункты

- [x] **1.1 Обновить зависимость SDK в go.mod**
  - **Dependencies**: None
  - **Description**: Обновить `github.com/BigKAA/topologymetrics/sdk-go` с v0.8.1 до v0.8.2 в `go.mod`, запустить `go mod tidy`
  - **Modifies**:
    - `go.mod`
    - `go.sum`

- [x] **1.2 Добавить поля в структуру Dependency (config.go)**
  - **Dependencies**: 1.1
  - **Description**: Добавить два новых поля в структуру `Dependency`:
    - `HostHeader string` — для HTTP-зависимостей (YAML: `hostHeader`, env: `DEPHEALTH_<NAME>_HOST_HEADER`)
    - `GRPCAuthority string` — для gRPC-зависимостей (YAML: `grpcAuthority`, env: `DEPHEALTH_<NAME>_GRPC_AUTHORITY`)
  - **Modifies**:
    - `internal/config/config.go` — структура `Dependency` (после поля `HealthPath`/`TLSSkipVerify` для HTTP, после `GRPCServiceName`/`TLSSkipVerify` для gRPC)
    - `internal/config/yaml.go` — структура `yamlDep` (добавить поля с YAML-тегами `yaml:"hostHeader"`, `yaml:"grpcAuthority"`)

- [x] **1.3 Парсинг env vars для новых полей (config.go)**
  - **Dependencies**: 1.2
  - **Description**: В функции `parseSingleDep()` добавить чтение:
    - `DEPHEALTH_<NAME>_HOST_HEADER` → `dep.HostHeader`
    - `DEPHEALTH_<NAME>_GRPC_AUTHORITY` → `dep.GRPCAuthority`

    Расположить после парсинга TLS-опций, рядом с аналогичными type-specific полями.
  - **Modifies**:
    - `internal/config/config.go` — функция `parseSingleDep()`

- [x] **1.4 Конвертация YAML-полей в Dependency (yaml.go)**
  - **Dependencies**: 1.2
  - **Description**: YAML-парсинг использует отдельную структуру `yamlDep` в `internal/config/yaml.go`. Необходимо:
    1. Добавить поля `HostHeader` и `GRPCAuthority` в структуру `yamlDep` (с тегами `yaml:"hostHeader"` и `yaml:"grpcAuthority"`)  — выполняется в шаге 1.2
    2. Добавить копирование новых полей в функции `convertYAMLDep()` (`dep.HostHeader = yd.HostHeader`, `dep.GRPCAuthority = yd.GRPCAuthority`)

    **Примечание**: функция `applyEnvOverrides()` НЕ обрабатывает per-dependency поля напрямую — она делегирует в `parseDeps()`→`parseSingleDep()`, что уже покрыто шагом 1.3. Дополнительных изменений в `applyEnvOverrides()` не требуется.
  - **Modifies**:
    - `internal/config/yaml.go` — функция `convertYAMLDep()`

- [x] **1.5 Передача опций в SDK (main.go)**
  - **Dependencies**: 1.2
  - **Description**: В функции `buildDependencyOption()` в `main.go` добавить передачу новых опций в SDK:
    - Для HTTP-зависимостей: `if dep.HostHeader != "" { depOpts = append(depOpts, dephealth.WithHTTPHostHeader(dep.HostHeader)) }`
    - Для gRPC-зависимостей: `if dep.GRPCAuthority != "" { depOpts = append(depOpts, dephealth.WithGRPCAuthority(dep.GRPCAuthority)) }`

    Расположить после TLS-опций, перед auth-опциями — аналогично порядку в SDK.
  - **Modifies**:
    - `main.go` — функция `buildDependencyOption()`

- [x] **1.6 Unit-тесты конфигурации**
  - **Dependencies**: 1.3, 1.4
  - **Description**: Добавить тесты в `internal/config/config_test.go`:
    - Тест парсинга env var `DEPHEALTH_<NAME>_HOST_HEADER` (HTTP-зависимость)
    - Тест парсинга env var `DEPHEALTH_<NAME>_GRPC_AUTHORITY` (gRPC-зависимость)
    - Тест парсинга YAML-полей `hostHeader` и `grpcAuthority` (через YAML config file с `CONFIG_FILE`)
    - Тест конвертации `yamlDep` → `Dependency` для новых полей (покрывает `convertYAMLDep()`)
    - Запустить `go test ./...` для проверки
  - **Modifies**:
    - `internal/config/config_test.go`

### ✅ Критерии завершения Phase 1

- [x] Все подпункты завершены (1.1–1.6)
- [x] `go mod tidy` завершается без ошибок
- [x] `go build ./...` компилируется успешно
- [x] `go test ./...` — все тесты проходят
- [ ] `make lint` — нет ошибок линтера

---

## Phase 2: Документация и примеры

**Dependencies**: Phase 1
**Status**: ✅ Done

### Описание

Обновить документацию, примеры конфигурации и CLAUDE.md с описанием новых опций и use case'ом обращения к сервисам за ingress/nginx по IP.

### Подпункты

- [x] **2.1 Обновить examples/config.yaml**
  - **Dependencies**: None
  - **Description**: Добавить примеры использования `hostHeader` и `grpcAuthority`:
    - Новый HTTP-пример: обращение по IP к сервису за ingress с кастомным Host
    - Новый gRPC-пример: gRPC-сервис за reverse proxy с кастомным authority
    - Добавить как активный пример (не закомментированный) для наглядности
  - **Modifies**:
    - `examples/config.yaml`

- [x] **2.2 Обновить README.md**
  - **Dependencies**: None
  - **Description**: Обновить документацию:
    - Добавить `hostHeader` и `grpcAuthority` в таблицу per-dependency переменных
    - Добавить `DEPHEALTH_<NAME>_HOST_HEADER` и `DEPHEALTH_<NAME>_GRPC_AUTHORITY` env vars в таблицу
    - Добавить секцию/пример "Custom Host Header (Ingress/Proxy Routing)" с use case:
      - Сценарий: приложение за nginx/ingress, доступно по IP, но требует Host header для маршрутизации
      - Пример env vars
      - Пример YAML
    - Обновить версию SDK если упоминается
  - **Modifies**:
    - `README.md`

- [x] **2.3 Обновить CLAUDE.md**
  - **Dependencies**: None
  - **Description**: Добавить `hostHeader`/`grpcAuthority` в описание конфигурационной модели:
    - В секции per-dependency vars упомянуть `DEPHEALTH_<NAME>_HOST_HEADER`, `DEPHEALTH_<NAME>_GRPC_AUTHORITY`
    - В описании `Dependency` struct добавить поля `HostHeader`, `GRPCAuthority`
    - Обновить версию SDK (v0.8.1 → v0.8.2)
  - **Modifies**:
    - `CLAUDE.md`

- [x] **2.4 Обновить Helm chart values (при необходимости)**
  - **Dependencies**: None
  - **Description**: Проверить шаблоны Helm chart (`charts/uniproxy/` и `deploy/helm/uniproxy/`):
    - Если конфигурация зависимостей передается через values.yaml → добавить поддержку `hostHeader` и `grpcAuthority`
    - Если используется только env vars → обновить примеры instances
  - **Result**: Helm-шаблоны не требуют изменений — per-dependency опции передаются через `extraEnv` или `configFile.content`
  - **Modifies**:
    - `charts/uniproxy/values.yaml` (если применимо)
    - `deploy/helm/uniproxy/instances/*.yaml` (если применимо)

### ✅ Критерии завершения Phase 2

- [x] Все подпункты завершены (2.1–2.4)
- [x] Примеры конфигурации корректны и покрывают основной use case (ingress/proxy)
- [x] README содержит понятное описание и примеры
- [x] `make helm-lint` проходит (Helm-файлы не изменены)

---

## Phase 3: Сборка и тестирование

**Dependencies**: Phase 1, Phase 2
**Status**: Pending

### Описание

Сборка Docker-контейнера, запуск в тестовой Kubernetes среде и верификация работы новых опций.

### Подпункты

- [x] **3.1 Прогон линтеров и тестов**
  - **Dependencies**: None
  - **Description**: Запустить полный набор проверок:
    - `make check-all` (lint + test + audit + deadcode + helm-lint + hadolint)
    - Исправить все найденные проблемы
  - **Creates**:
    - Отчет о прохождении проверок

- [x] **3.2 Сборка Docker-образа**
  - **Dependencies**: 3.1
  - **Description**: Собрать Docker-образ для тестирования:
    ```bash
    docker build -t uniproxy:dev .
    ```
    Убедиться что образ собирается без ошибок.
  - **Creates**:
    - Docker image `uniproxy:dev`

- [x] **3.3 Локальное тестирование (Docker)**
  - **Dependencies**: 3.2
  - **Description**: Запустить контейнер с тестовой конфигурацией, включающей `hostHeader`:
    - Использовать env vars или config.yaml с `hostHeader`
    - Проверить health endpoint (`/`)
    - Проверить метрики (`/metrics`) — убедиться что метрики генерируются
    - Проверить логи — убедиться что нет ошибок конфигурации
  - **Creates**:
    - Результаты тестирования

- [ ] **3.4 Тестирование в Kubernetes**
  - **Dependencies**: 3.3
  - **Description**: Развернуть в тестовом кластере:
    - Собрать образ для dev-реестра: `docker build -t harbor.kryukov.lan/library/uniproxy:dev .`
    - Push в dev-реестр
    - Развернуть через Helm с конфигурацией, использующей `hostHeader`
    - Проверить, что health check с кастомным Host header работает корректно
    - Проверить метрики через port-forward
  - **Creates**:
    - Результаты тестирования в Kubernetes

### ✅ Критерии завершения Phase 3

- [ ] Все подпункты завершены (3.1–3.4) — ожидает 3.4
- [x] `make lint` + `make test` проходят без ошибок
- [x] Docker-образ собирается успешно
- [x] Health check с `hostHeader` отправляет корректный Host header (проверено с httpbin.org)
- [ ] Health check с `grpcAuthority` отправляет корректный :authority (требует gRPC-сервис за proxy)
- [x] Метрики генерируются корректно
- [x] Нет ошибок в логах

---

## 📝 Примечания

- SDK v0.8.2 автоматически валидирует конфликты: `hostHeader` vs `Host` в headers, `grpcAuthority` vs `:authority` в metadata — дополнительная валидация в uniproxy не нужна
- Поле `host` в метриках Prometheus **не меняется** — оно всегда отражает реальный endpoint (IP), а не значение Host header
- Для тестирования `hostHeader` удобно использовать httpbin.org или любой сервис с виртуальным хостингом
- Тестирование `grpcAuthority` требует gRPC-сервис за reverse proxy (envoy, nginx с gRPC pass-through)
- `_FILE` суффикс для секретов **не нужен** для `hostHeader`/`grpcAuthority` — это не секретные значения

---

**🎯 План готов к использованию. Запуск: `/sc:implement`**
