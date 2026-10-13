# План разработки: Произвольные метки на зависимость (custom metric labels)

## 📋 Метаданные

- **Версия плана**: 1.0.0
- **Дата создания**: 2026-10-13
- **Последнее обновление**: 2026-10-13
- **Статус**: Completed
- **Репозиторий**: `uniproxy`
- **Связанные планы**: нет

---

## 📚 История версий

- **v1.0.0** (2026-10-13): Начальная версия плана

---

## 📍 Текущий статус

- **Активная фаза**: Completed
- **Активный подпункт**: —
- **Последнее обновление**: 2026-10-13
- **Примечание**: Реализовано в ветке `feature/dependency-custom-labels`. Все фазы завершены: тесты зелёные (`make test`), lint — 0 issues, e2e в Docker подтверждён (env-форма, YAML-форма, env>YAML по ключу, ранняя валидация).

---

## 📑 Оглавление

- [x] [Phase 1: Конфигурация (YAML + env-оверлей + валидация)](#phase-1-конфигурация-yaml--env-оверлей--валидация)
- [x] [Phase 2: Прокидывание меток в SDK (main.go)](#phase-2-прокидывание-меток-в-sdk-maingo)
- [x] [Phase 3: Тесты и документация](#phase-3-тесты-и-документация)

---

## Контекст и причина

dephealth SDK поддерживает произвольные метки на зависимость:
- API: `dephealth.WithLabel(key, value)` (DependencyOption);
- Env (нативно, читает сам SDK): `DEPHEALTH_<NAME>_LABEL_<KEY>=value` — суффикс приводится к нижнему регистру; API имеет приоритет над env при конфликте ключей (options.go:578-600).

Проверено в Docker (2026-10-13): env-форма работает в uniproxy уже сегодня без изменений кода — `test1`/`test2` появляются на всех 4 метриках SDK.

**Пробелы:**
1. YAML-конфиг (`CONFIG_FILE`) не поддерживает метки — SDK читает только env.
2. Валидация имён меток из env не выполняется (SDK валидирует только API-метки; зарезервированные: `name`, `group`, `dependency`, `type`, `host`, `port`, `path`, `critical`).
3. Прецедентность env > YAML: если метки заданы в YAML через API, SDK даст API приоритет над env-метками — нарушение глобального правила uniproxy «env всегда перекрывает YAML».
4. Документация упоминает LABEL_-механизм только вскользь (README, раздел Topology Grouping).

**Решение:** `internal/config` сам мержит `DEPHEALTH_<NAME>_LABEL_<KEY>` env-переменные в `Dependency.Labels` поверх YAML-значений и валидирует имена; `main.go` передаёт всё через `WithLabel`. Env-оверлей SDK при этом становится no-op (ключи уже заданы через API — тот же источник значения), прецедентность env > YAML сохраняется.

---

## Phase 1: Конфигурация (YAML + env-оверлей + валидация)

**Dependencies**: None
**Status**: In Progress

### Описание

Парсинг per-dependency меток в `internal/config`: поле `Labels` в `Dependency`, YAML-секция `labels:` у зависимости, env-оверлей `DEPHEALTH_<NAME>_LABEL_<KEY>`, ранняя валидация имён (паттерн Prometheus + зарезервированные имена SDK).

### Подпункты

- [ ] **1.1 Поле Labels и валидация**
  - **Dependencies**: None
  - **Description**: `Labels map[string]string` в `Dependency`; `validateMetricLabels` — зеркалирует правила `dephealth.ValidateLabelName` (паттерн `^[a-zA-Z_][a-zA-Z0-9_]*$`, reserved: name/group/dependency/type/host/port/path/critical); вызов в `validate()`.
  - **Creates**: `internal/config/config.go`
- [ ] **1.2 YAML labels**
  - **Dependencies**: 1.1
  - **Description**: `labels map[string]string` в `yamlDep` + копирование в `convertYAMLDep`.
  - **Creates**: `internal/config/yaml.go`
- [ ] **1.3 Env-оверлей LABEL_<KEY>**
  - **Dependencies**: 1.1
  - **Description**: `applyDepEnvLabelOverlay(dep, prefix)` — читает `os.Environ()` с префиксом `DEPHEALTH_<NAME>_LABEL_`, ключ = суффикс в нижнем регистре, значение перекрывает YAML. Вызовы: `parseSingleDep` (env-депы) и цикл оверлеев YAML-депов в `applyEnvOverrides`.
  - **Creates**: `internal/config/config.go`

### ✅ Критерии завершения Phase 1

- [ ] Все подпункты завершены (1.1, 1.2, 1.3)
- [ ] YAML `labels:` парсится, env перекрывает YAML по ключу
- [ ] Невалидное/зарезервированное имя метки — понятная ошибка на старте с именем зависимости

---

## Phase 2: Прокидывание меток в SDK (main.go)

**Dependencies**: Phase 1
**Status**: Pending

### Описание

В `buildDependencyOption` добавить цикл `dephealth.WithLabel(k, v)` по `dep.Labels`. Разместить **до** блока isentry/dep_namespace/dep_group, чтобы first-class переменные продолжали выигрывать при конфликте ключей (задокументированное поведение, README «Topology Grouping»).

### Подпункты

- [ ] **2.1 Цикл WithLabel в buildDependencyOption**
  - **Dependencies**: None
  - **Creates**: `main.go`

### ✅ Критерии завершения Phase 2

- [ ] Метки из конфига появляются на всех 4 метриках SDK
- [ ] При конфликте с first-class переменными (dep_namespace/dep_group/isentry) выигрывают first-class

---

## Phase 3: Тесты и документация

**Dependencies**: Phase 1, Phase 2
**Status**: Pending

### Описание

Тесты (config: env-оверлей, прецедентность env>YAML, валидация; yaml: парсинг labels; main: e2e метки на метрике), документация (README.md + README.ru.md: секция Custom labels, YAML-пример, строка в env-таблице; AGENTS.md: per-dependency env vars), проверка `make test`/`make lint` через Docker, e2e в Docker (env-форма + YAML-форма).

### Подпункты

- [ ] **3.1 Тесты config/yaml/main**
- [ ] **3.2 Документация README.md, README.ru.md, AGENTS.md**
- [ ] **3.3 make test, make lint, e2e в Docker**

### ✅ Критерии завершения Phase 3

- [ ] Все тесты зелёные (`make test`)
- [ ] `make lint` без замечаний
- [ ] E2e в Docker: метки видны в /metrics (env-форма и YAML-форма)
- [ ] Документация обновлена

---

## 📝 Примечания

- SDK для ключа, отсутствующего у конкретной зависимости, подставит пустое значение метки (union ключей по всем зависимостям) — нормальное поведение Prometheus.
- `_FILE`-паттерн для меток не вводится: метки не секреты.
- Решение пользователя (2026-10-13): глобальных меток (DEPHEALTH_LABELS / глобальный labels в YAML) НЕ вводить.

---

**🎯 План готов к использованию. Удачной разработки!**
