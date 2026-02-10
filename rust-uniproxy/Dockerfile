# ============================================================================
# Multi-stage Dockerfile для Rust приложения
# ============================================================================
#
# ОБРАЗОВАТЕЛЬНЫЕ ЦЕЛИ:
# - Демонстрация multi-stage build для минимизации размера образа
# - Использование Alpine для production образа
# - Кеширование зависимостей для быстрой сборки
#
# ============================================================================

# ============================================================================
# STAGE 1: Builder - сборка приложения
# ============================================================================

FROM rust:1.75-alpine AS builder

# Устанавливаем необходимые build dependencies
# musl-dev - для статической линковки с musl libc (Alpine uses musl)
# pkgconfig - для поиска системных библиотек
RUN apk add --no-cache musl-dev pkgconfig openssl-dev

# Создаем рабочую директорию
WORKDIR /app

# ============================================================================
# OPTIMIZATION: Кеширование зависимостей
# ============================================================================
#
# Docker кеширует слои. Если Cargo.toml не изменился, зависимости
# не будут пересобираться при изменении исходного кода.
#
# Этот трюк значительно ускоряет incremental builds:
# 1. Копируем только Cargo.toml и Cargo.lock
# 2. Создаем dummy src/main.rs для сборки зависимостей
# 3. Собираем зависимости (cargo build --dependencies-only не существует в cargo,
#    поэтому используем dummy main.rs)
# 4. Копируем настоящий исходный код и пересобираем только приложение

# Копируем манифесты для сборки зависимостей
COPY Cargo.toml Cargo.lock ./

# Создаем dummy файлы для сборки зависимостей
RUN mkdir -p src && \
    echo "fn main() {}" > src/main.rs && \
    echo "// dummy" > src/lib.rs

# Собираем зависимости (release mode для оптимизации)
# Это создаст слой с скомпилированными dependencies
RUN cargo build --release

# Удаляем dummy файлы
RUN rm -rf src

# ============================================================================
# Копируем настоящий исходный код
# ============================================================================

COPY src ./src

# Пересобираем только наше приложение
# --release - production build с оптимизациями
# Зависимости уже скомпилированы и закешированы в предыдущем слое
RUN cargo build --release

# ============================================================================
# STAGE 2: Runtime - минимальный production образ
# ============================================================================

FROM alpine:latest

# Устанавливаем runtime dependencies
# ca-certificates - для HTTPS запросов
# libgcc - runtime library для Rust приложений
RUN apk add --no-cache ca-certificates libgcc

# Создаем non-root пользователя для безопасности
# В production не стоит запускать приложения от root
RUN addgroup -g 1000 uniproxy && \
    adduser -D -u 1000 -G uniproxy uniproxy

# Создаем директорию для приложения
WORKDIR /app

# Копируем скомпилированный binary из builder stage
# --from=builder - копируем из предыдущего stage
# --chown - устанавливаем владельца файла
COPY --from=builder --chown=uniproxy:uniproxy \
    /app/target/release/uniproxy \
    /usr/local/bin/uniproxy

# Переключаемся на non-root пользователя
USER uniproxy

# Expose порты
# 8080 - HTTP server
# 9090 - Prometheus metrics (опционально, можно объединить с 8080)
EXPOSE 8080

# Healthcheck для Docker
# Проверяем /healthz endpoint каждые 30 секунд
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/healthz || exit 1

# Запускаем приложение
CMD ["uniproxy"]

# ============================================================================
# МЕТКИ (labels) для metadata
# ============================================================================

LABEL org.opencontainers.image.title="uniproxy-rust"
LABEL org.opencontainers.image.description="Universal test proxy (Rust implementation with educational comments)"
LABEL org.opencontainers.image.version="0.1.0"
LABEL org.opencontainers.image.authors="uniproxy team"
LABEL org.opencontainers.image.source="https://github.com/BigKAA/uniproxy"

# ============================================================================
# ИСПОЛЬЗОВАНИЕ:
# ============================================================================
#
# Сборка:
#   docker build -t uniproxy-rust:dev .
#
# Запуск:
#   docker run -p 8080:8080 \
#     -e DEPHEALTH_NAME=test-proxy \
#     -e DEPHEALTH_DEPS=httpbin:http \
#     -e DEPHEALTH_HTTPBIN_URL=http://httpbin.org \
#     -e DEPHEALTH_HTTPBIN_CRITICAL=yes \
#     uniproxy-rust:dev
#
# Multi-platform build (amd64 + arm64):
#   docker buildx build --platform linux/amd64,linux/arm64 \
#     -t uniproxy-rust:latest .
#
# ============================================================================
# РАЗМЕР ОБРАЗА:
# ============================================================================
#
# - Builder stage: ~1.5 GB (Rust toolchain + dependencies)
# - Runtime stage: ~20-25 MB (Alpine + binary)
#
# Финальный образ включает только:
# - Alpine Linux (~5 MB)
# - Rust binary (~10-15 MB с strip=true в Cargo.toml)
# - Runtime libraries (~5 MB)
#
# Сравнение с Go:
# - Go binary: ~25 MB (статически слинкован)
# - Rust binary: ~10-15 MB (с оптимизациями)
# - Оба используют Alpine base (~5 MB)
#
# ============================================================================
# OPTIMIZATION TIPS:
# ============================================================================
#
# 1. Используйте multi-stage build (уже сделано)
# 2. Кешируйте dependencies (уже сделано)
# 3. В Cargo.toml установите:
#    [profile.release]
#    opt-level = 3
#    lto = true
#    codegen-units = 1
#    strip = true
# 4. Для минимального размера можно использовать:
#    FROM scratch (но нужна статическая линковка)
# 5. Используйте cargo-chef для лучшего кеширования:
#    https://github.com/LukeMathWalker/cargo-chef
#
# ============================================================================
