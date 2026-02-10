// ============================================================================
// server/mod.rs - HTTP сервер с Axum framework
// ============================================================================
//
// ОБРАЗОВАТЕЛЬНЫЕ ЦЕЛИ:
// - Async web framework (Axum)
// - Routing и handlers
// - Shared state с Arc
// - JSON serialization
// - Error handling в web контексте
//
// ============================================================================

use std::sync::Arc;
use axum::{
    routing::get,
    Router,
    Json,
    extract::State,
    response::{Response, IntoResponse},
    http::StatusCode,
};
use serde::{Serialize, Deserialize};
use tower_http::trace::TraceLayer;
use tracing::info;

use crate::health::HealthManager;

// ============================================================================
// APPLICATION STATE
// ============================================================================

/// Shared state приложения
///
/// В веб-приложениях часто нужно передавать общее состояние (БД, кеши и т.д.)
/// между разными handlers. В Axum это делается через State.
///
/// Arc - для shared ownership между threads (handlers могут выполняться параллельно)
#[derive(Clone)]
pub struct AppState {
    /// Имя сервиса
    pub name: String,
    /// Health manager для проверки зависимостей
    /// Arc<HealthManager> - shared ownership, thread-safe
    pub health_manager: Arc<HealthManager>,
}

// ============================================================================
// RESPONSE TYPES
// ============================================================================

/// Ответ на GET / - статус приложения
///
/// Serialize - для конверсии в JSON
/// Deserialize - для парсинга из JSON (на случай если понадобится)
#[derive(Debug, Serialize, Deserialize)]
pub struct StatusResponse {
    /// Имя сервиса
    pub name: String,
    /// Имя pod (из env переменной POD_NAME)
    #[serde(rename = "podName")]
    pub pod_name: String,
    /// Namespace (из env переменной NAMESPACE)
    pub namespace: String,
    /// Статус зависимостей: name -> is_healthy
    pub health: std::collections::HashMap<String, bool>,
}

// ============================================================================
// SERVER
// ============================================================================

/// HTTP сервер
///
/// Инкапсулирует Axum router и конфигурацию
pub struct Server {
    /// Имя сервиса
    name: String,
    /// Адрес для прослушивания
    listen_addr: String,
    /// Health manager
    health_manager: Arc<HealthManager>,
}

impl Server {
    /// Создать новый сервер
    ///
    /// В Rust часто используют паттерн "builder" или "new" для конструкторов
    /// pub fn new() - это associated function (как static method в других языках)
    pub fn new(
        name: String,
        listen_addr: String,
        health_manager: Arc<HealthManager>,
    ) -> Self {
        Self {
            name,
            listen_addr,
            health_manager,
        }
    }

    /// Запустить сервер
    ///
    /// async fn - асинхронная функция
    /// &self - borrowing (не берем ownership структуры)
    pub async fn run(self) -> anyhow::Result<()> {
        // Создаем shared state
        let state = AppState {
            name: self.name.clone(),
            health_manager: self.health_manager.clone(),
        };

        // ====================================================================
        // ROUTING
        // ====================================================================

        // Router::new() создает новый router
        // .route(path, handler) добавляет route
        // get(handler) - HTTP GET метод
        //
        // В Axum handlers - это обычные async функции
        // Аргументы извлекаются автоматически (dependency injection):
        // - State<T> - shared state
        // - Json<T> - JSON body
        // - Path<T> - path parameters
        // и т.д.
        let app = Router::new()
            // GET / - статус приложения
            .route("/", get(handle_root))
            // GET /healthz - liveness probe (всегда OK)
            .route("/healthz", get(handle_healthz))
            // GET /readyz - readiness probe (всегда OK)
            .route("/readyz", get(handle_readyz))
            // GET /metrics - Prometheus метрики
            .route("/metrics", get(handle_metrics))
            // with_state() - добавляем shared state
            .with_state(state)
            // TraceLayer - middleware для логирования запросов
            // В Go аналог: middleware функции
            .layer(TraceLayer::new_for_http());

        // ====================================================================
        // ЗАПУСК СЕРВЕРА
        // ====================================================================

        info!("🌐 Starting HTTP server on {}", self.listen_addr);

        // Парсим адрес в SocketAddr
        // parse() использует FromStr trait
        let addr: std::net::SocketAddr = self
            .listen_addr
            .parse()
            .expect("Invalid listen address");

        // axum::Server::bind() создает HTTP сервер
        // serve() запускает сервер с нашим router
        axum::Server::bind(&addr)
            .serve(app.into_make_service())
            .await?;

        Ok(())
    }
}

// ============================================================================
// HANDLERS
// ============================================================================

/// Handler для GET /
///
/// State<AppState> - Axum автоматически извлечет state из request
/// async fn - асинхронная функция (returns Future)
///
/// impl IntoResponse - возвращаемый тип может быть преобразован в HTTP Response
/// Это trait, который реализован для многих типов:
/// - (StatusCode, Json<T>)
/// - String
/// - &'static str
/// - Json<T>
/// и т.д.
async fn handle_root(
    State(state): State<AppState>,
) -> impl IntoResponse {
    // Получаем статус всех зависимостей
    let health = state.health_manager.get_health_status().await;

    // Читаем env переменные (могут быть установлены в Kubernetes)
    let pod_name = std::env::var("POD_NAME").unwrap_or_else(|_| "unknown".to_string());
    let namespace = std::env::var("NAMESPACE").unwrap_or_else(|_| "default".to_string());

    // Создаем response
    let response = StatusResponse {
        name: state.name.clone(),
        pod_name,
        namespace,
        health,
    };

    // Json<T> автоматически serializes в JSON и устанавливает Content-Type
    // В Go аналог: json.NewEncoder(w).Encode(response)
    Json(response)
}

/// Handler для GET /healthz
///
/// Liveness probe - проверяет что приложение живо
/// Всегда возвращает 200 OK
///
/// В Kubernetes используется для перезапуска pod при сбое
async fn handle_healthz() -> impl IntoResponse {
    // &'static str - string literal, lifetime 'static (существует весь runtime)
    // Автоматически конвертируется в Response с status 200
    (StatusCode::OK, "ok")
}

/// Handler для GET /readyz
///
/// Readiness probe - проверяет что приложение готово обрабатывать запросы
/// В нашем случае всегда OK, но можно проверять готовность зависимостей
///
/// В Kubernetes используется для управления трафиком (Service endpoints)
async fn handle_readyz() -> impl IntoResponse {
    (StatusCode::OK, "ok")
}

/// Handler для GET /metrics
///
/// Prometheus метрики
/// TODO: реализовать интеграцию с prometheus crate
async fn handle_metrics() -> impl IntoResponse {
    // Пока возвращаем заглушку
    // В Phase 4 добавим настоящие метрики
    (
        StatusCode::OK,
        "# HELP app_dependency_health Dependency health status\n\
         # TYPE app_dependency_health gauge\n"
    )
}

// ============================================================================
// ВАЖНЫЕ КОНЦЕПЦИИ RUST В ЭТОМ МОДУЛЕ:
// ============================================================================
//
// 1. ASYNC WEB FRAMEWORK (AXUM)
//    - Router для определения routes
//    - Handlers как async функции
//    - Extractors для dependency injection (State, Json, Path, etc.)
//    - IntoResponse trait для гибких return types
//
// 2. SHARED STATE
//    - Arc<T> для shared ownership между threads
//    - Clone для AppState (clone Arc = дешево)
//    - State<T> extractor в Axum
//
// 3. TRAITS
//    - IntoResponse - для конверсии типов в HTTP Response
//    - Serialize/Deserialize - для JSON
//    - Clone - для копирования state
//
// 4. MIDDLEWARE
//    - TraceLayer для logging
//    - Layer pattern в tower/axum
//
// 5. ASYNC/AWAIT
//    - async fn возвращает Future
//    - .await для ожидания Future
//    - Все IO операции асинхронные
//
// 6. ERROR HANDLING
//    - anyhow::Result для упрощения
//    - ? оператор
//    - expect() для паник при невалидных данных
//
// 7. LIFETIME ANNOTATIONS
//    - &'static str - string literal с static lifetime
//    - В большинстве случаев Rust выводит lifetimes автоматически
//
// ============================================================================
//
// СРАВНЕНИЕ С GO (chi router):
//
// GO:
//   r := chi.NewRouter()
//   r.Get("/", handler)
//   http.ListenAndServe(":8080", r)
//
// RUST:
//   let app = Router::new()
//       .route("/", get(handler))
//       .with_state(state);
//   axum::Server::bind(&addr).serve(app).await
//
// Основные отличия:
// - В Rust все async (явный async/await)
// - State извлекается через extractors (type-safe dependency injection)
// - Compile-time проверка типов для handlers
//
// ============================================================================
