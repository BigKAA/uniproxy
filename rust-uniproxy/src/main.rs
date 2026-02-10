// ============================================================================
// main.rs - точка входа в приложение uniproxy (Rust версия)
// ============================================================================
//
// ОБРАЗОВАТЕЛЬНЫЕ ЦЕЛИ:
// - Демонстрация async/await с tokio runtime
// - Graceful shutdown с signal handling
// - Управление lifetime приложения
// - Error handling с Result и ?
//
// ============================================================================

// Подключаем стандартную библиотеку и внешние crates
use std::sync::Arc;
use tracing::{info, error};
use tracing_subscriber;

// Импортируем наши модули
// В Rust используется явный импорт, в отличие от Go где пакеты импортируются целиком
use uniproxy::config::Config;
use uniproxy::server::Server;
use uniproxy::health::HealthManager;

// #[tokio::main] - макрос, который превращает async fn main в обычный fn main
// Под капотом создает tokio runtime и запускает async функцию
// Аналог: в Go вы просто пишете main(), а goroutines запускаете через go keyword
#[tokio::main]
async fn main() -> anyhow::Result<()> {
    // ========================================================================
    // ШАГ 1: Инициализация логирования
    // ========================================================================

    // Tracing - это structured logging framework для Rust
    // Аналог: в Go используется log/slog
    tracing_subscriber::fmt()
        .with_env_filter(
            // Читаем уровень логирования из RUST_LOG env переменной
            // По умолчанию: info
            tracing_subscriber::EnvFilter::try_from_default_env()
                .unwrap_or_else(|_| tracing_subscriber::EnvFilter::new("info"))
        )
        .init();

    info!("🚀 Starting uniproxy (Rust implementation)");

    // ========================================================================
    // ШАГ 2: Загрузка конфигурации
    // ========================================================================

    // Config::load() возвращает Result<Config, ConfigError>
    // Оператор ? - это syntactic sugar для:
    //   match Config::load() {
    //       Ok(cfg) => cfg,
    //       Err(e) => return Err(e.into()),
    //   }
    //
    // В Go аналог:
    //   cfg, err := config.Load()
    //   if err != nil { return err }
    let config = Config::load()?;

    info!(
        "✅ Configuration loaded: name={}, dependencies={}",
        config.name,
        config.dependencies.len()
    );

    // ========================================================================
    // ШАГ 3: Создание Health Manager
    // ========================================================================

    // Arc (Atomic Reference Counting) - умный указатель для shared ownership
    // Аналог: в Go вы бы просто передавали указатель, а GC сам управляет памятью
    //
    // В Rust ownership model:
    // - Каждое значение имеет одного владельца (owner)
    // - Когда владелец выходит из scope, значение уничтожается (drop)
    // - Arc позволяет иметь несколько владельцев (reference counting)
    //
    // Arc<T> - thread-safe (atomic operations)
    // Rc<T> - single-threaded версия (дешевле, но не Send/Sync)
    let health_manager = Arc::new(
        HealthManager::new(config.dependencies.clone(), config.check_interval)
    );

    // Запускаем periodic health checks в фоновом режиме
    // clone() для Arc - это cheap operation (инкремент счетчика)
    let health_manager_bg = health_manager.clone();
    tokio::spawn(async move {
        // move - переносит ownership переменных в closure
        // health_manager_bg теперь owned этой async task
        health_manager_bg.start_periodic_checks().await;
    });

    info!("✅ Health manager started");

    // ========================================================================
    // ШАГ 4: Запуск HTTP сервера
    // ========================================================================

    let server = Server::new(
        config.name.clone(),
        config.listen_addr.clone(),
        health_manager.clone(),
    );

    // Запускаем сервер в отдельной task
    let server_handle = tokio::spawn(async move {
        server.run().await
    });

    info!("🌐 HTTP server listening on {}", config.listen_addr);

    // ========================================================================
    // ШАГ 5: Graceful Shutdown
    // ========================================================================

    // В Rust нет встроенных сигналов как в Go (SIGINT, SIGTERM)
    // Используем tokio::signal для кросс-платформенного signal handling

    // Ждем сигнал завершения (Ctrl+C или SIGTERM)
    match tokio::signal::ctrl_c().await {
        Ok(()) => {
            info!("🛑 Received shutdown signal, gracefully shutting down...");
        }
        Err(err) => {
            error!("❌ Failed to listen for shutdown signal: {}", err);
        }
    }

    // Останавливаем health manager
    health_manager.stop().await;

    // Ждем завершения сервера
    // join() возвращает Result<Result<(), E>, JoinError>
    //   Внешний Result - успешно ли завершилась task
    //   Внутренний Result - результат выполнения task
    if let Err(e) = server_handle.await {
        error!("❌ Server task failed: {}", e);
    }

    info!("👋 Shutdown complete");

    // Result<()> - либо Ok(()), либо Err
    // В Go: return nil / return err
    Ok(())
}

// ============================================================================
// ВАЖНЫЕ КОНЦЕПЦИИ RUST, ПРОДЕМОНСТРИРОВАННЫЕ В ЭТОМ ФАЙЛЕ:
// ============================================================================
//
// 1. ASYNC/AWAIT
//    - async fn возвращает Future<Output = T>
//    - .await приостанавливает выполнение до завершения Future
//    - tokio - runtime для выполнения async кода
//
// 2. OWNERSHIP & BORROWING
//    - Arc<T> для shared ownership между threads
//    - clone() создает новый Arc (cheap, инкремент счетчика)
//    - move closure переносит ownership в async block
//
// 3. ERROR HANDLING
//    - Result<T, E> для функций, которые могут завершиться ошибкой
//    - ? оператор для propagation ошибок
//    - anyhow::Result для быстрого прототипирования
//
// 4. PATTERN MATCHING
//    - match для exhaustive pattern matching
//    - if let для случаев, когда нужна одна ветка
//
// 5. TRAITS
//    - Clone trait для типов, которые можно копировать
//    - Send + Sync traits для thread-safety (требуется для Arc)
//
// ============================================================================
