# План разработки: Подключение SDK с лейблом `path` в uniproxy

## 📋 Метаданные

- **Версия плана**: 1.0.0
- **Дата создания**: 2026-07-30
- **Последнее обновление**: 2026-07-30
- **Статус**: Pending
- **Репозиторий**: `uniproxy`
- **Связанные планы**: `topologymetrics/plans/add-path-label.md` (предусловие), `dephealth-ui/plans/add-path-label.md` (consumer)

---

## 📚 История версий

- **v1.0.0** (2026-07-30): Начальная версия плана

---

## 📍 Текущий статус

- **Активная фаза**: Phase 1
- **Активный подпункт**: 1.1
- **Последнее обновление**: 2026-07-30
- **Примечание**: Выполняется **после** релиза SDK v0.9.0 и **до** dephealth-ui. Порядок между репозиториями: SDK v0.9.0 → uniproxy → dephealth-ui.

---

## 📑 Оглавление

- [ ] [Phase 1: Обновление зависимости SDK](#phase-1-обновление-зависимости-sdk)
- [ ] [Phase 2: Smoke-тест метрик](#phase-2-smoke-тест-метрик)
- [ ] [Phase 3: Тесты и документация](#phase-3-тесты-и-документация)

---

## Контекст и причина

`uniproxy` уже передаёт полный URL зависимости в SDK (`main.go` ≈ строки 287-291 → `dephealth.FromURL(dep.URL)`), поэтому **логических изменений в коде uniproxy не требуется** — путь начнёт эмититься автоматически после bump SDK. Конфиг уже содержит URL с путями (например, `url: http://smart.prod-dc-cloud2.passport.local:80/app/sys/mdm/...`).

### Принятые решения (project-wide)
- `path` — обязательный лейбл в SDK; значение по умолчанию `"/"`.
- После bump в `app_dependency_*` появится `path="..."` (для `http://host` → `path="/"`, для `http://host/...` → `path="/..."`).

### Подтверждение предусловия
- [ ] SDK v0.9.0 выпущен (теги `sdk-go/v0.9.0` и др.) — см. `topologymetrics/plans/add-path-label.md`.

---

## Phase 1: Обновление зависимости SDK

**Dependencies**: None (требует только релиза SDK)
**Status**: Pending

### Подпункты

- [ ] **1.1 Bump SDK**
  - **Dependencies**: None
  - **Description**: Обновить зависимость `github.com/BigKAA/topologymetrics/sdk-go` с `v0.8.2` до `v0.9.0` в `go.mod`; синхронизировать `go.sum`.
  - **Creates**: изменения в
    - `go.mod` — `github.com/BigKAA/topologymetrics/sdk-go v0.9.0`
    - `go.sum` — обновить хэши
  - **Links**: `uniproxy/go.mod`
  - **Code examples**
    ```bash
    go get github.com/BigKAA/topologymetrics/sdk-go@v0.9.0
    go mod tidy
    ```

### ✅ Критерии завершения Phase 1

- [ ] `go.mod` ссылается на `sdk-go v0.9.0`
- [ ] `go build ./...` успешен
- [ ] `go.sum` консистентен

---

## Phase 2: Smoke-тест метрик

**Dependencies**: Phase 1
**Status**: Pending

### Подпункты

- [ ] **2.1 Проверка наличия лейбла `path`**
  - **Dependencies**: None
  - **Description**: Запустить `uniproxy` локально с реальным конфигом (например, `tmp/config.yaml` с зависимостями, имеющими пути), открыть `/metrics` и убедиться, что:
    - в `app_dependency_health{...}` появился `path="..."`;
    - для URL с явным путём (`http://host/app/sys/mdm/...`) → `path="/app/sys/mdm/..."`;
    - для URL без пути (`http://host`) → `path="/"`.
    - тот же лейбл присутствует в `app_dependency_latency_seconds_*`, `app_dependency_status`, `app_dependency_status_detail`.
  - **Creates**: Test results (ручная проверка)
  - **Links**: `uniproxy/examples/config.yaml`

### ✅ Критерии завершения Phase 2

- [ ] Во всех 4 метриках присутствует лейбл `path`
- [ ] Значение `path` корректно для URL с путём и без

---

## Phase 3: Тесты и документация

**Dependencies**: Phase 2
**Status**: Pending

### Подпункты

- [ ] **3.1 Тесты**
  - **Dependencies**: None
  - **Description**: Если в репозитории есть фикстуры/тесты, фиксирующие явный лейбл-сет метрик (строки вида `app_dependency_health{...}`), обновить ожидания — добавить `path="..."`. Запустить полный набор.
  - **Creates**: изменения в тестах (при наличии)
  - **Links**: `uniproxy/internal/config/*_test.go`, `uniproxy/internal/config/yaml_test.go`

- [ ] **3.2 Документация**
  - **Dependencies**: None
  - **Description**: Упомянуть в README/docs, что метрики теперь содержат обязательный лейбл `path`; при необходимости добавить пример.
  - **Creates**: изменения в README/docs

### ✅ Критерии завершения Phase 3

- [ ] Все тесты проходят (`go test ./...`)
- [ ] Документация обновлена
- [ ] Готово к проверке consumer'ом (см. `dephealth-ui/plans/add-path-label.md`)

---

## 📝 Примечания

- Логических изменений в коде `uniproxy` нет — путь эмитится SDK на основе URL, который `uniproxy` уже передаёт.
- `Dependency.HealthPath` (конфиг) — это путь health-чек эндпоинта, **не** URL-путь зависимости; к этому лейблу отношения не имеет.

---

**🎯 План готов к использованию. Удачной разработки!**
