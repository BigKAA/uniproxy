# 🚀 Быстрый старт - uniproxy на Rust

## Установка Rust

```bash
# Установка через rustup (если еще не установлен)
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh

# Проверка версии
rustc --version  # должно быть 1.75+
cargo --version
```

## Локальный запуск (без Docker)

```bash
# Перейти в директорию
cd rust-uniproxy

# Сборка (debug mode - быстрая компиляция)
cargo build

# Или сразу build + run
cargo run
```

**Примечание**: Go версия требует Docker для запуска, но Rust версию можно запустить локально для обучения!

## Запуск с тестовой конфигурацией

```bash
# Установим переменные окружения
export DEPHEALTH_NAME=uniproxy-rust-test
export DEPHEALTH_DEPS=httpbin:http
export DEPHEALTH_HTTPBIN_URL=http://httpbin.org
export DEPHEALTH_HTTPBIN_CRITICAL=yes
export DEPHEALTH_CHECK_INTERVAL=10
export LOG_LEVEL=debug

# Запуск
cargo run
```

## Проверка работы

Откройте в браузере или curl:

```bash
# Статус приложения
curl http://localhost:8080/

# Liveness probe
curl http://localhost:8080/healthz

# Prometheus метрики
curl http://localhost:8080/metrics
```

## Docker запуск

```bash
# Сборка образа
docker build -t uniproxy-rust:dev .

# Запуск
docker run -p 8080:8080 \
  -e DEPHEALTH_NAME=test \
  -e DEPHEALTH_DEPS=httpbin:http \
  -e DEPHEALTH_HTTPBIN_URL=http://httpbin.org \
  -e DEPHEALTH_HTTPBIN_CRITICAL=yes \
  uniproxy-rust:dev

# Проверка
curl http://localhost:8080/
```

## Тестирование

```bash
# Все тесты
cargo test

# С выводом
cargo test -- --nocapture

# Конкретный модуль
cargo test config::tests
```

## Изучение кода

Рекомендуемый порядок:

1. `Cargo.toml` - зависимости
2. `src/lib.rs` - модули
3. `src/config/mod.rs` - Result, Option, enum
4. `src/main.rs` - async/await, tokio
5. `src/server/mod.rs` - Axum, handlers
6. `src/health/mod.rs` - traits, RwLock
7. `src/metrics/mod.rs` - lazy_static

## Полезные команды

```bash
# Проверка кода (без сборки)
cargo check

# Форматирование кода
cargo fmt

# Линтер
cargo clippy

# Документация
cargo doc --open

# Release build (оптимизированный)
cargo build --release

# Запуск release версии
./target/release/uniproxy
```

## Отладка

```bash
# Debug логи
RUST_LOG=debug cargo run

# Trace логи для конкретного модуля
RUST_LOG=uniproxy::health=trace cargo run

# Backtrace при ошибках
RUST_BACKTRACE=1 cargo run
```

## IDE Setup

### VS Code

```bash
# Установите расширения:
# - rust-analyzer (обязательно!)
# - CodeLLDB (для отладки)
# - crates (подсветка версий в Cargo.toml)
```

### IntelliJ/CLion

```bash
# Установите плагин Rust
# File -> Settings -> Plugins -> Rust
```

## Проблемы и решения

### Ошибка: "cargo: command not found"

```bash
# Добавьте cargo в PATH
source $HOME/.cargo/env
```

### Ошибка при сборке зависимостей

```bash
# Очистите кеш и пересоберите
cargo clean
cargo build
```

### Медленная компиляция

```bash
# Используйте mold linker (Linux) или lld (macOS)
# Добавьте в ~/.cargo/config.toml:
[target.x86_64-unknown-linux-gnu]
linker = "clang"
rustflags = ["-C", "link-arg=-fuse-ld=mold"]
```

---

**Готово! Теперь можно изучать Rust на реальном примере** 🦀
