# План разработки: uniproxy на Rust (обучающая версия)

## 📋 Метаданные

- **Версия плана**: 1.0.0
- **Дата создания**: 2026-02-10
- **Последнее обновление**: 2026-02-10
- **Статус**: Pending

---

## 📚 История версий

- **v1.0.0** (2026-02-10): Начальная версия плана - Rust реализация с подробными обучающими комментариями на русском

---

## 📍 Текущий статус

- **Активная фаза**: Phase 1
- **Активный подпункт**: 1.1
- **Последнее обновление**: 2026-02-10
- **Примечание**: Создание структуры Rust проекта с Cargo

---

## 📑 Оглавление

- [ ] [Phase 1: Инициализация Rust проекта и базовая структура](#phase-1-инициализация-rust-проекта-и-базовая-структура)
- [ ] [Phase 2: Конфигурация и парсинг environment variables](#phase-2-конфигурация-и-парсинг-environment-variables)
- [ ] [Phase 3: HTTP сервер с Axum](#phase-3-http-сервер-с-axum)
- [ ] [Phase 4: Интеграция health checks и Prometheus метрик](#phase-4-интеграция-health-checks-и-prometheus-метрик)
- [ ] [Phase 5: Dockerfile и тестирование](#phase-5-dockerfile-и-тестирование)

---

## Phase 1: Инициализация Rust проекта и базовая структура

**Dependencies**: None
**Status**: Pending

### Описание

Создание базовой структуры Rust проекта с использованием Cargo. Настройка зависимостей и модульной структуры, аналогичной Go версии. Все комментарии будут на русском языке для образовательных целей.

### Подпункты

- [ ] **1.1 Создание Cargo проекта**
  - **Dependencies**: None
  - **Description**: Инициализация Rust проекта в директории rust-uniproxy с Cargo.toml
  - **Creates**:
    - `rust-uniproxy/Cargo.toml`
    - `rust-uniproxy/src/main.rs`
  - **Links**:
    - [Cargo Book](https://doc.rust-lang.org/cargo/)

- [ ] **1.2 Настройка зависимостей**
  - **Dependencies**: 1.1
  - **Description**: Добавление основных crates: tokio (async runtime), serde (сериализация), env_logger (логирование)
  - **Creates**:
    - Обновленный `Cargo.toml` с dependencies
  - **Links**:
    - [Tokio Documentation](https://tokio.rs/)
    - [Serde Documentation](https://serde.rs/)

- [ ] **1.3 Создание модульной структуры**
  - **Dependencies**: 1.2
  - **Description**: Создание модулей config, server в src/ по аналогии с Go версией
  - **Creates**:
    - `rust-uniproxy/src/config/mod.rs`
    - `rust-uniproxy/src/server/mod.rs`
    - `rust-uniproxy/src/lib.rs`
  - **Links**: N/A

### ✅ Критерии завершения Phase 1

- [ ] Все подпункты завершены (1.1, 1.2, 1.3)
- [ ] Проект компилируется: `cargo build`
- [ ] Базовая структура модулей создана
- [ ] README.md создан с описанием проекта

---

## Phase 2: Конфигурация и парсинг environment variables

**Dependencies**: Phase 1
**Status**: Pending

### Описание

Реализация модуля конфигурации для парсинга environment variables. Демонстрация Rust концепций: struct, enum, Result, Option, String vs &str, error handling с использованием thiserror.

### Подпункты

- [ ] **2.1 Структуры Config и Dependency**
  - **Dependencies**: None
  - **Description**: Создание Config и Dependency structs с derive макросами (Debug, Clone)
  - **Creates**:
    - `rust-uniproxy/src/config/types.rs`
  - **Links**:
    - [Rust Book - Structs](https://doc.rust-lang.org/book/ch05-00-structs.html)

- [ ] **2.2 Парсинг environment variables**
  - **Dependencies**: 2.1
  - **Description**: Функция load() для чтения env vars с использованием std::env, демонстрация Result<T, E>
  - **Creates**:
    - `rust-uniproxy/src/config/loader.rs`
  - **Links**:
    - [Rust Book - Error Handling](https://doc.rust-lang.org/book/ch09-00-error-handling.html)

- [ ] **2.3 Валидация и обработка ошибок**
  - **Dependencies**: 2.2
  - **Description**: Кастомные error types с использованием thiserror, демонстрация ? operator
  - **Creates**:
    - `rust-uniproxy/src/config/error.rs`
  - **Links**:
    - [thiserror crate](https://docs.rs/thiserror/)

- [ ] **2.4 Unit тесты для конфигурации**
  - **Dependencies**: 2.3
  - **Description**: Написание тестов для парсинга config с использованием #[cfg(test)]
  - **Creates**:
    - Тесты в `rust-uniproxy/src/config/loader.rs`
  - **Links**:
    - [Rust Book - Testing](https://doc.rust-lang.org/book/ch11-00-testing.html)

### ✅ Критерии завершения Phase 2

- [ ] Все подпункты завершены (2.1, 2.2, 2.3, 2.4)
- [ ] Config успешно парсит все env переменные из Go версии
- [ ] Все unit тесты проходят: `cargo test`
- [ ] Обработка ошибок с понятными сообщениями

---

## Phase 3: HTTP сервер с Axum

**Dependencies**: Phase 2
**Status**: Pending

### Описание

Реализация HTTP сервера с использованием Axum framework. Демонстрация async/await, trait objects, Arc<T>, lifetime параметров, routing.

### Подпункты

- [ ] **3.1 Базовый Axum сервер**
  - **Dependencies**: None
  - **Description**: Создание простого HTTP сервера с Axum, демонстрация async fn и tokio runtime
  - **Creates**:
    - `rust-uniproxy/src/server/mod.rs`
  - **Links**:
    - [Axum Documentation](https://docs.rs/axum/)

- [ ] **3.2 Endpoints: /, /healthz, /readyz**
  - **Dependencies**: 3.1
  - **Description**: Реализация handler функций, демонстрация Json extractor, State pattern
  - **Creates**:
    - `rust-uniproxy/src/server/handlers.rs`
  - **Links**:
    - [Axum Handlers](https://docs.rs/axum/latest/axum/#handlers)

- [ ] **3.3 Application State с Arc**
  - **Dependencies**: 3.2
  - **Description**: Создание AppState с Arc для shared state между handlers
  - **Creates**:
    - `rust-uniproxy/src/server/state.rs`
  - **Links**:
    - [Rust Book - Smart Pointers](https://doc.rust-lang.org/book/ch15-00-smart-pointers.html)

- [ ] **3.4 Graceful shutdown**
  - **Dependencies**: 3.3
  - **Description**: Реализация graceful shutdown с tokio::signal, демонстрация select! макроса
  - **Creates**:
    - Код в `rust-uniproxy/src/main.rs`
  - **Links**:
    - [Tokio Shutdown](https://tokio.rs/tokio/topics/shutdown)

### ✅ Критерии завершения Phase 3

- [ ] Все подпункты завершены (3.1, 3.2, 3.3, 3.4)
- [ ] HTTP сервер запускается и отвечает на все endpoints
- [ ] Graceful shutdown работает корректно
- [ ] Логирование с использованием tracing crate

---

## Phase 4: Интеграция health checks и Prometheus метрик

**Dependencies**: Phase 3
**Status**: Pending

### Описание

Реализация health checking логики для различных типов зависимостей (HTTP, Redis, PostgreSQL) и экспорт Prometheus метрик. Демонстрация trait objects, async functions, HashMap, channels.

### Подпункты

- [ ] **4.1 Health Checker trait**
  - **Dependencies**: None
  - **Description**: Определение trait HealthChecker с async методом check(), демонстрация trait objects
  - **Creates**:
    - `rust-uniproxy/src/health/mod.rs`
    - `rust-uniproxy/src/health/checker.rs`
  - **Links**:
    - [Rust Book - Traits](https://doc.rust-lang.org/book/ch10-02-traits.html)

- [ ] **4.2 Реализация HTTP checker**
  - **Dependencies**: 4.1
  - **Description**: HTTP health checker с использованием reqwest, демонстрация async HTTP клиента
  - **Creates**:
    - `rust-uniproxy/src/health/http.rs`
  - **Links**:
    - [reqwest Documentation](https://docs.rs/reqwest/)

- [ ] **4.3 Реализация Redis и PostgreSQL checkers**
  - **Dependencies**: 4.2
  - **Description**: Database health checkers, демонстрация работы с различными async clients
  - **Creates**:
    - `rust-uniproxy/src/health/redis.rs`
    - `rust-uniproxy/src/health/postgres.rs`
  - **Links**:
    - [redis crate](https://docs.rs/redis/)
    - [tokio-postgres crate](https://docs.rs/tokio-postgres/)

- [ ] **4.4 Health Manager с periodic checks**
  - **Dependencies**: 4.3
  - **Description**: Менеджер для periodic health checks с использованием tokio::time::interval
  - **Creates**:
    - `rust-uniproxy/src/health/manager.rs`
  - **Links**:
    - [Tokio Time](https://docs.rs/tokio/latest/tokio/time/index.html)

- [ ] **4.5 Prometheus метрики**
  - **Dependencies**: 4.4
  - **Description**: Экспорт метрик с prometheus crate, endpoint /metrics
  - **Creates**:
    - `rust-uniproxy/src/metrics/mod.rs`
  - **Links**:
    - [prometheus crate](https://docs.rs/prometheus/)

### ✅ Критерии завершения Phase 4

- [ ] Все подпункты завершены (4.1, 4.2, 4.3, 4.4, 4.5)
- [ ] Health checks работают для всех типов зависимостей
- [ ] Prometheus метрики экспортируются на /metrics
- [ ] Periodic checks выполняются с заданным интервалом

---

## Phase 5: Dockerfile и тестирование

**Dependencies**: Phase 1, Phase 2, Phase 3, Phase 4
**Status**: Pending

### Описание

Создание multi-stage Dockerfile для Rust приложения и полное тестирование в Docker контейнере.

### Подпункты

- [ ] **5.1 Multi-stage Dockerfile**
  - **Dependencies**: None
  - **Description**: Создание оптимизированного Dockerfile с builder stage и runtime stage
  - **Creates**:
    - `rust-uniproxy/Dockerfile`
  - **Links**:
    - [Docker Rust Best Practices](https://docs.docker.com/language/rust/)

- [ ] **5.2 Сборка Docker образа**
  - **Dependencies**: 5.1
  - **Description**: Сборка образа и проверка размера
  - **Creates**:
    - Docker image `uniproxy-rust:dev`
  - **Links**: N/A

- [ ] **5.3 Тестирование в контейнере**
  - **Dependencies**: 5.2
  - **Description**: Запуск контейнера с тестовой конфигурацией и проверка всех endpoints
  - **Creates**:
    - Test results
  - **Links**: N/A

- [ ] **5.4 Документация и примеры**
  - **Dependencies**: 5.3
  - **Description**: Обновление README.md с примерами использования Rust версии
  - **Creates**:
    - `rust-uniproxy/README.md`
    - `rust-uniproxy/LEARNING_NOTES.md` (образовательные заметки)
  - **Links**: N/A

### ✅ Критерии завершения Phase 5

- [ ] Все подпункты завершены (5.1, 5.2, 5.3, 5.4)
- [ ] **Docker образ успешно собран**
- [ ] **Все тесты в контейнере пройдены**
- [ ] Размер образа оптимизирован (использование Alpine/distroless)
- [ ] Документация содержит обучающие комментарии и примеры

---

## 📝 Примечания

### Образовательные цели

Rust версия демонстрирует следующие концепции Rust:

1. **Ownership и Borrowing**: String vs &str, Arc<T> для shared ownership
2. **Error Handling**: Result<T, E>, ? operator, thiserror для кастомных ошибок
3. **Async/Await**: tokio runtime, async fn, Future trait
4. **Traits**: HealthChecker trait, trait objects (dyn Trait)
5. **Pattern Matching**: match expressions, if let
6. **Structs и Enums**: derive макросы, методы
7. **Modules**: модульная организация кода
8. **Testing**: unit tests, integration tests
9. **Lifetime параметры**: в случаях, где необходимо
10. **Smart Pointers**: Arc<T>, Mutex<T> для concurrent access

### Отличия от Go версии

- **Нет dephealth SDK**: реализуем health checking логику напрямую
- **Async/await вместо goroutines**: демонстрация Rust подхода к concurrency
- **Строгая типизация**: все типы явно определены
- **Ownership model**: демонстрация управления памятью без GC

### Технические решения

- **Web framework**: Axum (современный, ergonomic, базируется на tokio)
- **Async runtime**: Tokio (стандарт де-факто)
- **Logging**: tracing + tracing-subscriber (structured logging)
- **Config**: std::env + serde для десериализации
- **Metrics**: prometheus crate для экспорта метрик

---

**🎯 План готов к использованию. Начинаем изучение Rust!**
