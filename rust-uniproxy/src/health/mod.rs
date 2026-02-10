// ============================================================================
// health/mod.rs - health checking логика для различных типов зависимостей
// ============================================================================
//
// ОБРАЗОВАТЕЛЬНЫЕ ЦЕЛИ:
// - Trait objects и dynamic dispatch
// - Async trait methods
// - HashMap и Mutex для concurrent access
// - tokio::time для periodic tasks
// - Error handling в async контексте
//
// ============================================================================

use std::collections::HashMap;
use std::sync::Arc;
use std::time::Duration;
use tokio::sync::RwLock;
use tokio::time;
use tracing::{info, warn, error};

use crate::config::{Dependency, DependencyType};

// ============================================================================
// HEALTH CHECKER TRAIT
// ============================================================================

/// Trait для health checking различных типов зависимостей
///
/// Trait в Rust - это аналог interface в Go
/// Определяет контракт: любой тип, который implements HealthChecker,
/// должен предоставить метод check()
///
/// #[async_trait] - макрос для поддержки async методов в traits
/// (в stable Rust пока нет нативной поддержки async в traits)
#[async_trait::async_trait]
pub trait HealthChecker: Send + Sync {
    /// Проверить здоровье зависимости
    ///
    /// Возвращает:
    /// - Ok(true) - зависимость здорова
    /// - Ok(false) - зависимость недоступна
    /// - Err - ошибка при проверке
    async fn check(&self) -> Result<bool, Box<dyn std::error::Error + Send + Sync>>;

    /// Имя зависимости
    fn name(&self) -> &str;

    /// Критичная ли зависимость
    fn is_critical(&self) -> bool;
}

// ============================================================================
// HTTP HEALTH CHECKER
// ============================================================================

/// HTTP health checker
///
/// Проверяет доступность HTTP endpoint
pub struct HttpChecker {
    name: String,
    url: String,
    critical: bool,
    // reqwest::Client - HTTP клиент
    // Shared между проверками для переиспользования connection pool
    client: reqwest::Client,
}

impl HttpChecker {
    pub fn new(dep: &Dependency) -> Result<Self, String> {
        // Строим URL
        let url = if let Some(ref url) = dep.url {
            url.clone()
        } else if let (Some(ref host), Some(port)) = (&dep.host, dep.port) {
            // Формируем URL из host:port
            let path = dep.health_path.as_deref().unwrap_or("/");
            format!("http://{}:{}{}", host, port, path)
        } else {
            return Err("HTTP checker requires url or host+port".to_string());
        };

        // Создаем HTTP клиент
        // timeout - максимальное время ожидания ответа
        let client = reqwest::Client::builder()
            .timeout(Duration::from_secs(5))
            .build()
            .map_err(|e| format!("Failed to create HTTP client: {}", e))?;

        Ok(Self {
            name: dep.name.clone(),
            url,
            critical: dep.critical,
            client,
        })
    }
}

// Реализация HealthChecker trait для HttpChecker
#[async_trait::async_trait]
impl HealthChecker for HttpChecker {
    async fn check(&self) -> Result<bool, Box<dyn std::error::Error + Send + Sync>> {
        // Выполняем GET запрос
        match self.client.get(&self.url).send().await {
            Ok(response) => {
                // Проверяем status code
                // 2xx и 3xx считаем успешными
                let is_healthy = response.status().is_success()
                    || response.status().is_redirection();
                Ok(is_healthy)
            }
            Err(e) => {
                // Ошибка подключения - зависимость недоступна
                warn!("HTTP check failed for {}: {}", self.name, e);
                Ok(false)
            }
        }
    }

    fn name(&self) -> &str {
        &self.name
    }

    fn is_critical(&self) -> bool {
        self.critical
    }
}

// ============================================================================
// REDIS HEALTH CHECKER
// ============================================================================

/// Redis health checker
///
/// Проверяет доступность Redis с помощью PING команды
pub struct RedisChecker {
    name: String,
    connection_string: String,
    critical: bool,
}

impl RedisChecker {
    pub fn new(dep: &Dependency) -> Result<Self, String> {
        let connection_string = if let Some(ref url) = dep.url {
            url.clone()
        } else if let (Some(ref host), Some(port)) = (&dep.host, dep.port) {
            format!("redis://{}:{}", host, port)
        } else {
            return Err("Redis checker requires url or host+port".to_string());
        };

        Ok(Self {
            name: dep.name.clone(),
            connection_string,
            critical: dep.critical,
        })
    }
}

#[async_trait::async_trait]
impl HealthChecker for RedisChecker {
    async fn check(&self) -> Result<bool, Box<dyn std::error::Error + Send + Sync>> {
        // Для простоты пока возвращаем заглушку
        // В реальной реализации:
        // 1. Подключаемся к Redis
        // 2. Выполняем PING
        // 3. Проверяем ответ PONG

        // TODO: Реализовать настоящую проверку Redis
        // let client = redis::Client::open(self.connection_string.as_str())?;
        // let mut con = client.get_async_connection().await?;
        // redis::cmd("PING").query_async(&mut con).await?;

        warn!("Redis health check not implemented yet for {}", self.name);
        Ok(true) // Заглушка
    }

    fn name(&self) -> &str {
        &self.name
    }

    fn is_critical(&self) -> bool {
        self.critical
    }
}

// ============================================================================
// POSTGRES HEALTH CHECKER
// ============================================================================

/// PostgreSQL health checker
pub struct PostgresChecker {
    name: String,
    connection_string: String,
    critical: bool,
}

impl PostgresChecker {
    pub fn new(dep: &Dependency) -> Result<Self, String> {
        let connection_string = if let Some(ref url) = dep.url {
            url.clone()
        } else if let (Some(ref host), Some(port)) = (&dep.host, dep.port) {
            format!("postgresql://{}:{}", host, port)
        } else {
            return Err("Postgres checker requires url or host+port".to_string());
        };

        Ok(Self {
            name: dep.name.clone(),
            connection_string,
            critical: dep.critical,
        })
    }
}

#[async_trait::async_trait]
impl HealthChecker for PostgresChecker {
    async fn check(&self) -> Result<bool, Box<dyn std::error::Error + Send + Sync>> {
        // TODO: Реализовать настоящую проверку PostgreSQL
        warn!("PostgreSQL health check not implemented yet for {}", self.name);
        Ok(true) // Заглушка
    }

    fn name(&self) -> &str {
        &self.name
    }

    fn is_critical(&self) -> bool {
        self.critical
    }
}

// ============================================================================
// GRPC HEALTH CHECKER
// ============================================================================

/// gRPC health checker
pub struct GrpcChecker {
    name: String,
    address: String,
    critical: bool,
}

impl GrpcChecker {
    pub fn new(dep: &Dependency) -> Result<Self, String> {
        let address = if let Some(ref url) = dep.url {
            url.clone()
        } else if let (Some(ref host), Some(port)) = (&dep.host, dep.port) {
            format!("{}:{}", host, port)
        } else {
            return Err("gRPC checker requires url or host+port".to_string());
        };

        Ok(Self {
            name: dep.name.clone(),
            address,
            critical: dep.critical,
        })
    }
}

#[async_trait::async_trait]
impl HealthChecker for GrpcChecker {
    async fn check(&self) -> Result<bool, Box<dyn std::error::Error + Send + Sync>> {
        // TODO: Реализовать gRPC health check protocol
        warn!("gRPC health check not implemented yet for {}", self.name);
        Ok(true) // Заглушка
    }

    fn name(&self) -> &str {
        &self.name
    }

    fn is_critical(&self) -> bool {
        self.critical
    }
}

// ============================================================================
// HEALTH MANAGER
// ============================================================================

/// Health Manager координирует проверки всех зависимостей
///
/// Отвечает за:
/// - Создание checkers для каждой зависимости
/// - Periodic проверки
/// - Хранение состояния (здорова/нездорова)
pub struct HealthManager {
    /// Список checkers
    /// Box<dyn HealthChecker> - trait object (dynamic dispatch)
    /// Позволяет хранить разные типы checkers в одном векторе
    checkers: Vec<Box<dyn HealthChecker>>,

    /// Текущее состояние зависимостей
    /// RwLock - для concurrent read/write access
    /// HashMap<String, bool> - имя -> статус
    ///
    /// RwLock vs Mutex:
    /// - RwLock - позволяет multiple readers или one writer
    /// - Mutex - только one accessor (reader or writer)
    status: Arc<RwLock<HashMap<String, bool>>>,

    /// Интервал проверки
    check_interval: Duration,
}

impl HealthManager {
    /// Создать новый Health Manager
    pub fn new(dependencies: Vec<Dependency>, check_interval: Duration) -> Self {
        let mut checkers: Vec<Box<dyn HealthChecker>> = Vec::new();
        let mut initial_status = HashMap::new();

        // Создаем checker для каждой зависимости
        for dep in dependencies {
            match Self::create_checker(&dep) {
                Ok(checker) => {
                    // Инициализируем статус как false (unknown)
                    initial_status.insert(dep.name.clone(), false);
                    checkers.push(checker);
                }
                Err(e) => {
                    error!("Failed to create checker for {}: {}", dep.name, e);
                }
            }
        }

        Self {
            checkers,
            status: Arc::new(RwLock::new(initial_status)),
            check_interval,
        }
    }

    /// Создать checker в зависимости от типа зависимости
    ///
    /// Возвращает Box<dyn HealthChecker> - trait object
    /// dyn - dynamic dispatch (runtime polymorphism)
    fn create_checker(dep: &Dependency) -> Result<Box<dyn HealthChecker>, String> {
        match dep.dep_type {
            DependencyType::Http => {
                Ok(Box::new(HttpChecker::new(dep)?))
            }
            DependencyType::Redis => {
                Ok(Box::new(RedisChecker::new(dep)?))
            }
            DependencyType::Postgres => {
                Ok(Box::new(PostgresChecker::new(dep)?))
            }
            DependencyType::Grpc => {
                Ok(Box::new(GrpcChecker::new(dep)?))
            }
        }
    }

    /// Запустить periodic проверки
    ///
    /// Эта функция запускается в background task и выполняется в цикле
    pub async fn start_periodic_checks(&self) {
        info!("Starting periodic health checks (interval: {:?})", self.check_interval);

        // interval() создает поток тиков с заданным интервалом
        let mut interval = time::interval(self.check_interval);

        loop {
            // tick().await ждет следующего тика
            interval.tick().await;

            // Выполняем проверки
            self.check_all().await;
        }
    }

    /// Проверить все зависимости
    async fn check_all(&self) {
        info!("Running health checks...");

        // Создаем задачи для параллельных проверок
        // В Rust мы можем запустить несколько async tasks одновременно
        let mut tasks = Vec::new();

        for checker in &self.checkers {
            // Клонируем Arc для передачи в task
            let status = self.status.clone();
            let name = checker.name().to_string();

            // Выполняем проверку
            let result = checker.check().await;

            // Обновляем статус
            match result {
                Ok(is_healthy) => {
                    let mut status_map = status.write().await;
                    status_map.insert(name.clone(), is_healthy);

                    let emoji = if is_healthy { "✅" } else { "❌" };
                    info!("{} {} health check: {}", emoji, name,
                        if is_healthy { "UP" } else { "DOWN" });
                }
                Err(e) => {
                    error!("❌ {} health check error: {}", name, e);
                    let mut status_map = status.write().await;
                    status_map.insert(name, false);
                }
            }
        }
    }

    /// Получить текущий статус всех зависимостей
    ///
    /// Используется в HTTP handler для GET /
    pub async fn get_health_status(&self) -> HashMap<String, bool> {
        // read() для read lock (позволяет multiple readers)
        // clone() создает копию HashMap
        self.status.read().await.clone()
    }

    /// Остановить health checks
    pub async fn stop(&self) {
        info!("Stopping health manager");
        // В текущей реализации просто логируем
        // В реальной реализации можно отправить signal для остановки loop
    }
}

// ============================================================================
// ВАЖНЫЕ КОНЦЕПЦИИ RUST В ЭТОМ МОДУЛЕ:
// ============================================================================
//
// 1. TRAIT OBJECTS (dyn Trait)
//    - Box<dyn HealthChecker> - heap-allocated trait object
//    - Dynamic dispatch (runtime polymorphism)
//    - В Go аналог: interface{} или конкретный interface
//
// 2. ASYNC TRAITS
//    - #[async_trait] макрос для async методов в traits
//    - async fn в trait -> Future
//
// 3. CONCURRENT ACCESS
//    - Arc<RwLock<T>> - thread-safe shared state
//    - RwLock - multiple readers, one writer
//    - read().await / write().await - async locks
//
// 4. PERIODIC TASKS
//    - tokio::time::interval() для periodic ticks
//    - loop + tick().await для бесконечного цикла
//
// 5. ERROR HANDLING
//    - Result<T, Box<dyn Error>> - для dynamic error types
//    - Send + Sync bounds для thread-safety
//
// 6. PATTERN MATCHING
//    - match для разных типов зависимостей
//    - Ok/Err pattern matching
//
// 7. OWNERSHIP
//    - Box для heap allocation
//    - Arc для shared ownership
//    - clone() для RwLock guard
//
// ============================================================================
//
// СРАВНЕНИЕ С GO:
//
// GO:
//   type HealthChecker interface {
//       Check() (bool, error)
//   }
//
//   checkers := []HealthChecker{
//       &HttpChecker{},
//       &RedisChecker{},
//   }
//
// RUST:
//   trait HealthChecker {
//       async fn check(&self) -> Result<bool, Error>;
//   }
//
//   let checkers: Vec<Box<dyn HealthChecker>> = vec![
//       Box::new(HttpChecker::new()),
//       Box::new(RedisChecker::new()),
//   ];
//
// Отличия:
// - В Rust нужен Box для trait objects
// - В Rust async требует async_trait макрос
// - В Rust явное управление памятью (Box, Arc)
//
// ============================================================================
