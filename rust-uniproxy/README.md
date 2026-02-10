# uniproxy-rust

🦀 **Образовательная Rust реализация uniproxy с подробными комментариями на русском**

## 📚 О проекте

Это Rust версия [uniproxy](../README.md) - универсального test proxy для мониторинга зависимостей. Создана специально для **изучения Rust** с подробными образовательными комментариями на русском языке.

### Основные отличия от Go версии

| Аспект | Go версия | Rust версия |
|--------|-----------|-------------|
| **Concurrency** | Goroutines + channels | async/await + tokio |
| **Memory** | Garbage Collector | Ownership + Borrowing |
| **Error handling** | Multiple return values | Result<T, E> + ? operator |
| **Web framework** | Chi router | Axum (tokio-based) |
| **Тестирование** | testing package | cargo test |
| **Сборка** | go build | cargo build |

## 🎯 Образовательные цели

Этот проект демонстрирует ключевые концепции Rust:

### 1. Ownership & Borrowing
- `String` vs `&str` (owned vs borrowed)
- `Arc<T>` для shared ownership
- `clone()` для создания копий
- Move semantics в closures

### 2. Error Handling
- `Result<T, E>` для fallible operations
- `Option<T>` для опциональных значений
- `?` operator для propagation ошибок
- `thiserror` для кастомных error types

### 3. Async/Await
- `async fn` и `Future` trait
- Tokio runtime
- `tokio::spawn` для background tasks
- `tokio::time::interval` для periodic checks

### 4. Traits
- `HealthChecker` trait (аналог interface)
- Trait objects: `Box<dyn Trait>`
- `#[async_trait]` для async методов в traits
- Derive макросы: `#[derive(Debug, Clone)]`

### 5. Concurrency
- `Arc<RwLock<T>>` для shared state
- Multiple readers, one writer pattern
- Thread-safe types: `Send + Sync`

### 6. Web Development
- Axum framework
- Extractors (State, Json)
- Middleware (TraceLayer)
- Routing и handlers

## 🚀 Quick Start

### Требования

- Rust 1.75+ (установите через [rustup](https://rustup.rs/))
- Docker (для контейнеризации)

### Сборка

```bash
# Сборка в debug режиме
cargo build

# Сборка с оптимизациями (release)
cargo build --release

# Запуск
cargo run
```

### Тестирование

```bash
# Запуск всех тестов
cargo test

# Тесты с выводом
cargo test -- --nocapture

# Тесты конкретного модуля
cargo test config::tests
```

### Запуск в Docker

```bash
# Сборка образа
docker build -t uniproxy-rust:dev .

# Запуск контейнера
docker run -p 8080:8080 -p 9090:9090 \
  -e DEPHEALTH_NAME=test-proxy \
  -e DEPHEALTH_DEPS=httpbin:http \
  -e DEPHEALTH_HTTPBIN_URL=http://httpbin.org \
  -e DEPHEALTH_HTTPBIN_CRITICAL=yes \
  uniproxy-rust:dev
```

## 📦 Структура проекта

```
rust-uniproxy/
├── Cargo.toml              # Манифест проекта и зависимости
├── Dockerfile              # Multi-stage Docker build
├── README.md               # Этот файл
├── LEARNING_NOTES.md       # Образовательные заметки по Rust
└── src/
    ├── main.rs             # Точка входа (токио runtime, graceful shutdown)
    ├── lib.rs              # Корневой модуль библиотеки
    ├── config/
    │   └── mod.rs          # Парсинг env vars (Result, Option, enum)
    ├── server/
    │   └── mod.rs          # HTTP сервер (Axum, handlers, State)
    ├── health/
    │   └── mod.rs          # Health checkers (traits, async, RwLock)
    └── metrics/
        └── mod.rs          # Prometheus метрики (lazy_static, GaugeVec)
```

## 🔧 Конфигурация

Точно так же как в Go версии - через environment variables:

### Обязательные

```bash
export DEPHEALTH_NAME=uniproxy-rust
export DEPHEALTH_DEPS=postgres:postgres,redis:redis
```

### Опциональные

```bash
export LISTEN_ADDR=0.0.0.0:8080           # HTTP server address
export LOG_LEVEL=info                     # info, debug, trace
export DEPHEALTH_CHECK_INTERVAL=10        # секунды
```

### Per-dependency

```bash
# PostgreSQL пример
export DEPHEALTH_POSTGRES_HOST=localhost
export DEPHEALTH_POSTGRES_PORT=5432
export DEPHEALTH_POSTGRES_CRITICAL=yes

# Redis пример
export DEPHEALTH_REDIS_URL=redis://localhost:6379
export DEPHEALTH_REDIS_CRITICAL=no

# HTTP пример
export DEPHEALTH_HTTPBIN_URL=http://httpbin.org
export DEPHEALTH_HTTPBIN_CRITICAL=yes
export DEPHEALTH_HTTPBIN_HEALTH_PATH=/status/200
```

## 🌐 Endpoints

| Endpoint | Описание |
|----------|----------|
| `GET /` | Статус приложения и зависимостей (JSON) |
| `GET /healthz` | Liveness probe (всегда 200 OK) |
| `GET /readyz` | Readiness probe (всегда 200 OK) |
| `GET /metrics` | Prometheus метрики |

### Пример ответа GET /

```json
{
  "name": "uniproxy-rust",
  "podName": "uniproxy-pod-123",
  "namespace": "default",
  "health": {
    "postgres-main": true,
    "redis-cache": true,
    "httpbin": false
  }
}
```

## 📊 Prometheus Метрики

### app_dependency_health

Gauge метрика: 1 = UP, 0 = DOWN

```prometheus
app_dependency_health{name="uniproxy",namespace="default",dependency="postgres",type="postgres",host="localhost",port="5432",critical="yes"} 1
```

### app_dependency_latency_seconds

Histogram метрика latency проверок

```prometheus
app_dependency_latency_seconds_bucket{name="uniproxy",namespace="default",dependency="postgres",le="0.05"} 42
```

## 📝 Изучение кода

Рекомендуемый порядок изучения:

1. **Cargo.toml** - зависимости и их назначение
2. **src/lib.rs** - модульная структура Rust
3. **src/config/mod.rs** - Result, Option, enum, error handling
4. **src/main.rs** - async/await, tokio, Arc, graceful shutdown
5. **src/server/mod.rs** - Axum, handlers, State, IntoResponse
6. **src/health/mod.rs** - traits, trait objects, RwLock, async tasks
7. **src/metrics/mod.rs** - lazy_static, Prometheus integration

### Ключевые паттерны

**Ownership pattern**:
```rust
// Owned String
let s = String::from("hello");

// Borrowed &str
let s_ref: &str = &s;

// Move ownership
let s2 = s; // s больше недоступен!
```

**Error handling pattern**:
```rust
fn load_config() -> Result<Config, ConfigError> {
    let name = env::var("NAME")
        .map_err(|_| ConfigError::Missing("NAME"))?;
    Ok(Config { name })
}
```

**Async pattern**:
```rust
#[tokio::main]
async fn main() {
    let task = tokio::spawn(async {
        // async работа
    });
    task.await.unwrap();
}
```

## 🐳 Docker

### Multi-stage Dockerfile

```dockerfile
# Stage 1: Builder
FROM rust:1.75-alpine AS builder
WORKDIR /app
COPY . .
RUN cargo build --release

# Stage 2: Runtime
FROM alpine:latest
COPY --from=builder /app/target/release/uniproxy /usr/local/bin/
CMD ["uniproxy"]
```

### Оптимизация размера

- **Rust binary**: ~10-15 MB (с strip = true)
- **Alpine base**: ~5 MB
- **Итого**: ~20 MB (vs Go ~25 MB с Alpine)

## 🔍 Отладка

### Логи

```bash
# Включить debug логи
export RUST_LOG=debug
cargo run

# Trace логи для конкретного модуля
export RUST_LOG=uniproxy::health=trace
cargo run
```

### Проверка памяти

```bash
# Valgrind (на Linux)
valgrind --leak-check=full target/debug/uniproxy

# Heaptrack (на Linux)
heaptrack target/debug/uniproxy
```

## 🧪 Тестирование

```bash
# Unit тесты
cargo test

# Integration тесты
cargo test --test integration_tests

# Benchmark тесты
cargo bench

# Coverage (требует tarpaulin)
cargo tarpaulin --out Html
```

## 📚 Дополнительные материалы

### Официальная документация

- [The Rust Book](https://doc.rust-lang.org/book/) - основы языка
- [Async Book](https://rust-lang.github.io/async-book/) - async/await
- [Cargo Book](https://doc.rust-lang.org/cargo/) - система сборки
- [Tokio Tutorial](https://tokio.rs/tokio/tutorial) - async runtime

### Crates документация

- [Axum](https://docs.rs/axum/) - web framework
- [Tokio](https://docs.rs/tokio/) - async runtime
- [Serde](https://serde.rs/) - сериализация
- [Prometheus](https://docs.rs/prometheus/) - метрики

### Обучающие ресурсы

- [Rust by Example](https://doc.rust-lang.org/rust-by-example/)
- [Rustlings](https://github.com/rust-lang/rustlings) - упражнения
- [Too Many Lists](https://rust-unofficial.github.io/too-many-lists/) - структуры данных

## 🤝 Сравнение с Go версией

| Концепция | Go | Rust |
|-----------|-----|------|
| Concurrency | `go func()` | `tokio::spawn(async {})` |
| Channels | `ch := make(chan T)` | `tokio::sync::mpsc::channel()` |
| Shared state | `&sync.Mutex{val}` | `Arc<RwLock<T>>` |
| Error handling | `if err != nil` | `?` operator |
| JSON | `json.Marshal()` | `serde_json::to_string()` |
| HTTP server | `http.ListenAndServe()` | `axum::Server::bind().serve()` |
| Testing | `func TestX(t *testing.T)` | `#[test] fn test_x()` |

## 💡 FAQ

**Q: Почему Rust сложнее Go?**
A: Rust дает больше контроля над памятью и производительностью, но требует понимания ownership/borrowing. Go проще, но менее эффективен.

**Q: Когда использовать String vs &str?**
A: `String` - owned (heap), используйте когда нужно владеть данными. `&str` - borrowed, используйте для чтения.

**Q: Что такое Arc<RwLock<T>>?**
A: `Arc` - shared ownership между threads, `RwLock` - multiple readers или one writer.

**Q: Зачем #[tokio::main]?**
A: Создает tokio runtime для выполнения async кода. Без этого async fn не будет работать.

## 📄 Лицензия

Apache 2.0 (как и Go версия)

## 🙏 Благодарности

- [Go версия uniproxy](../) - оригинальная реализация
- [dephealth SDK](https://github.com/BigKAA/topologymetrics) - вдохновение для health checking
- Rust Community - за отличную документацию и инструменты

---

**Создано с ❤️ для изучения Rust**
