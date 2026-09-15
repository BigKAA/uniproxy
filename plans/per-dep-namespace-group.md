# План разработки: Per-dependency `depNamespace` / `depGroup` для группировки топологии

## 📋 Метаданные

- **Версия плана**: 1.0.0
- **Дата создания**: 2026-09-15
- **Последнее обновление**: 2026-09-15
- **Статус**: Completed
- **Репозиторий**: `uniproxy`
- **Источник**: `dephealth-ui/tmp/issue-uniproxy.md` (draft issue для BigKAA/uniproxy)

---

## 📚 История версий

- **v1.0.0** (2026-09-15): Начальная версия плана

---

## 📍 Текущий статус

- **Активная фаза**: Завершено
- **Активный подпункт**: —
- **Последнее обновление**: 2026-09-15
- **Примечание**: Все фазы выполнены. `make test` / `make lint` / `make helm-lint` зелёные. Реализация на ветке `feature/dep-namespace-group`.

---

## 📑 Оглавление

- [x] [Phase 1: Конфигурация (struct, YAML, env, приоритеты, валидация)](#phase-1-конфигурация-struct-yaml-env-приоритеты-валидация)
- [x] [Phase 2: Прокидывание лейблов в SDK и тесты](#phase-2-прокидывание-лейблов-в-sdk-и-тесты)
- [x] [Phase 3: Helm chart и документация](#phase-3-helm-chart-и-документация)

---

## Контекст и причина

dephealth-ui группирует endpoints топологии по зарезервированным лейблам метрик
`dep_namespace` / `dep_group`. SDK умеет их эмитить как кастомные лейблы
(`WithLabel`, env `DEPHEALTH_<DEP>_LABEL_<KEY>`), но в uniproxy нет first-class,
документированной конфигурации. Цель — добавить:

1. Глобальные дефолты `depNamespace` / `depGroup` (YAML + env).
2. Per-dependency поля `depNamespace` / `depGroup` (YAML + env).
3. Валидацию значений по правилу имён зависимостей: `[a-z][a-z0-9-]*`, 1–63 символа.
4. Прокидывание в SDK как `WithLabel("dep_namespace"|"dep_group", value)`.

### Приоритет разрешения (итоговый порядок)

per-dep env > global env > per-dep YAML > global YAML

Выбор согласован с существующими правилами uniproxy: «env всегда перекрывает YAML»
(как в `loadDepAuth` — global env перекрывает per-dep YAML) и «per-dependency > global».

### Взаимодействие с interim-механизмом

При одновременном использовании старого `DEPHEALTH_<NAME>_LABEL_DEP_NAMESPACE` (читает SDK)
и нового `DEPHEALTH_<NAME>_DEP_NAMESPACE` (читает uniproxy → `WithLabel`) оба значения
дойдут до SDK; приоритет API над env в SDK даёт победу новому пути. Задокументировать.

### Кейс `DEPHEALTH_DEPS`

Когда зависимости заданы через `DEPHEALTH_DEPS`, YAML-зависимости полностью заменяются,
но глобальные `depNamespace`/`depGroup` (env или YAML) обязаны применяться и к env-deps —
наследование выполняется в `applyDefaults` после всех перекрытий.

---

## Phase 1: Конфигурация (struct, YAML, env, приоритеты, валидация)

**Dependencies**: None
**Status**: Completed

### Описание

Добавить поля в `Config`/`Dependency`, YAML-маппинг, env-переменные, разрешение
приоритетов и валидацию в `internal/config`.

### Подпункты

- [x] **1.1 Поля структур и YAML**
  - **Dependencies**: None
  - **Description**: `Config.DepNamespace`/`Config.DepGroup` (глобальные дефолты),
    `Dependency.DepNamespace`/`Dependency.DepGroup`; теги `yaml:"depNamespace"` /
    `yaml:"depGroup"` в `yamlConfig` и `yamlDep`, маппинг в конвертерах.
  - **Creates**:
    - `internal/config/config.go`
    - `internal/config/yaml.go`

- [x] **1.2 Env-переменные и приоритеты**
  - **Dependencies**: 1.1
  - **Description**: Глобальные `DEPHEALTH_DEP_NAMESPACE`/`DEPHEALTH_DEP_GROUP`;
    per-dep `DEPHEALTH_<NAME>_DEP_NAMESPACE`/`DEPHEALTH_<NAME>_DEP_GROUP`
    (в `parseSingleDep` для env-deps и в overlay-цикле для YAML-deps, вместе с `loadDepAuth`);
    наследование глобальных значений в `applyDefaults`.
  - **Creates**:
    - `internal/config/config.go`

- [x] **1.3 Валидация**
  - **Dependencies**: 1.2
  - **Description**: Значения (глобальные и разрешённые per-dep) обязаны соответствовать
    `[a-z][a-z0-9-]*` (1–63). Ошибки с указанием источника (глобальный / зависимость).
  - **Creates**:
    - `internal/config/config.go`

### ✅ Критерии завершения Phase 1

- [x] Все подпункты завершены (1.1, 1.2, 1.3)
- [x] Приоритеты per-dep env > global env > per-dep YAML > global YAML работают
- [x] Наследование глобальных значений работает для env-deps и YAML-deps
- [x] Невалидные значения дают понятную ошибку

---

## Phase 2: Прокидывание лейблов в SDK и тесты

**Dependencies**: Phase 1
**Status**: Completed

### Описание

Прокинуть разрешённые значения в SDK и покрыть тестами парсинг, приоритеты,
валидацию и эмиссию лейблов.

### Подпункты

- [x] **2.1 WithLabel в buildDependencyOption**
  - **Dependencies**: Phase 1
  - **Description**: В `main.go` рядом с блоком `isentry` добавить
    `WithLabel("dep_namespace", ...)` / `WithLabel("dep_group", ...)`.
  - **Creates**:
    - `main.go`

- [x] **2.2 Тесты конфигурации**
  - **Dependencies**: 2.1
  - **Description**: `internal/config/config_test.go` — env-only, наследование,
    приоритеты (4 уровня, включая YAML+env микс), валидация. `internal/config/yaml_test.go` —
    парсинг глобальных и per-dep полей.
  - **Creates**:
    - `internal/config/config_test.go`
    - `internal/config/yaml_test.go`

- [x] **2.3 Тест эмиссии лейблов (SDK end-to-end)**
  - **Dependencies**: 2.1
  - **Description**: В `main_test.go`: httptest-сервер как HTTP-зависимость,
    `buildDependencyOption` → `dephealth.New` с чистым `prometheus.NewRegistry()`,
    `Start`, поллинг `Gather()` до появления `app_dependency_health` с лейблами
    `dep_namespace`/`dep_group`, `Stop`. Учесть DefaultInitialDelay 5s (поллинг ≤ 10s).
  - **Creates**:
    - `main_test.go`

### ✅ Критерии завершения Phase 2

- [x] Все подпункты завершены (2.1, 2.2, 2.3)
- [x] `curl /metrics` показывает лейблы на всех `app_dependency_*` (проверено тестом 2.3)
- [x] `make test` проходит

---

## Phase 3: Helm chart и документация

**Dependencies**: Phase 2
**Status**: Completed

### Описание

Глобальные дефолты в Helm values, документация EN/RU, пример конфига.

### Подпункты

- [x] **3.1 Helm chart `charts/uniproxy`**
  - **Dependencies**: Phase 2
  - **Description**: `config.depNamespace`/`config.depGroup` в `values.yaml` →
    условные env `DEPHEALTH_DEP_NAMESPACE`/`DEPHEALTH_DEP_GROUP` в `deployment.yaml`.
    Per-dep остаётся через `configFile.content` / `extraEnv` (упомянуть в комментариях).
  - **Creates**:
    - `charts/uniproxy/values.yaml`
    - `charts/uniproxy/templates/deployment.yaml`

- [x] **3.2 Документация**
  - **Dependencies**: 3.1
  - **Description**: README.md/README.ru.md — строки в таблицах Global/Per-Dependency
    Variables, поля в YAML-примере, секция «Topology grouping» (лейблы, приоритеты,
    interim-механизм `DEPHEALTH_<NAME>_LABEL_DEP_NAMESPACE` для старых версий).
    `examples/config.yaml` — глобальные и per-dep поля.
  - **Creates**:
    - `README.md`
    - `README.ru.md`
    - `examples/config.yaml`

### ✅ Критерии завершения Phase 3

- [x] Все подпункты завершены (3.1, 3.2)
- [x] `make helm-lint` проходит
- [x] README (EN/RU) и `examples/config.yaml` обновлены

---

## 📝 Примечания

- Веткa: `feature/dep-namespace-group`, коммит `feat(config): ...` по Conventional Commits.
- После мержа — релиз по навыку github-release (вне этого плана).
- Ссылка на issue: `dephealth-ui/tmp/issue-uniproxy.md`; acceptance criteria из issue
  покрыты фазами 1–3.
