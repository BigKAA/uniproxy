// ============================================================================
// config/mod.rs - модуль конфигурации
// ============================================================================
//
// ОБРАЗОВАТЕЛЬНЫЕ ЦЕЛИ:
// - Демонстрация struct и derive макросов
// - Парсинг environment variables
// - Error handling с Result и кастомными ошибками
// - String vs &str
//
// ============================================================================

use std::env;
use std::time::Duration;
use serde::{Deserialize, Serialize};

// ============================================================================
// ТИПЫ ДАННЫХ
// ============================================================================

/// Основная конфигурация приложения
///
/// Derive макросы автоматически генерируют реализацию traits:
/// - Debug: для {:?} форматирования (полезно для отладки)
/// - Clone: для создания копий (нужно для Arc и async)
/// - Serialize/Deserialize: для JSON (serde)
///
/// В Go аналог: struct с JSON tags
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Config {
    /// Имя сервиса (обязательное)
    /// String - owned string (heap-allocated), в отличие от &str (string slice)
    pub name: String,

    /// Адрес для HTTP сервера (например, "0.0.0.0:8080")
    pub listen_addr: String,

    /// Уровень логирования ("info", "debug", "trace")
    pub log_level: String,

    /// Интервал проверки зависимостей
    /// Duration - тип из std::time, аналог time.Duration в Go
    pub check_interval: Duration,

    /// Список зависимостей для мониторинга
    /// Vec<T> - аналог []T в Go (динамический массив)
    pub dependencies: Vec<Dependency>,
}

/// Описание одной зависимости
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Dependency {
    /// Имя зависимости (например, "postgres-main")
    pub name: String,

    /// Тип зависимости: http, redis, postgres, grpc
    /// Используем String, но можно было бы использовать enum
    pub dep_type: DependencyType,

    /// URL для подключения (опционально, если не указаны host/port)
    /// Option<T> - аналог *T в Go (может быть None/Some)
    pub url: Option<String>,

    /// Хост (если URL не указан)
    pub host: Option<String>,

    /// Порт (если URL не указан)
    pub port: Option<u16>,

    /// Критичная ли зависимость
    pub critical: bool,

    /// Путь для HTTP health check (только для HTTP)
    pub health_path: Option<String>,
}

/// Тип зависимости
///
/// Enum в Rust - это algebraic data type (ADT)
/// Более мощный чем enum в Go (может содержать данные)
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
#[serde(rename_all = "lowercase")] // JSON: "http" вместо "Http"
pub enum DependencyType {
    Http,
    Redis,
    Postgres,
    Grpc,
}

// ============================================================================
// КАСТОМНЫЕ ОШИБКИ
// ============================================================================

/// Ошибки конфигурации
///
/// thiserror::Error - макрос для автоматической реализации std::error::Error
/// Упрощает создание кастомных error types
#[derive(Debug, thiserror::Error)]
pub enum ConfigError {
    #[error("environment variable '{0}' is required")]
    MissingEnvVar(String),

    #[error("invalid dependency format '{0}': expected 'name:type'")]
    InvalidDependencyFormat(String),

    #[error("unsupported dependency type '{0}'")]
    UnsupportedType(String),

    #[error("dependency '{0}': {1}")]
    DependencyError(String, String),

    #[error("invalid duration '{0}': {1}")]
    InvalidDuration(String, std::num::ParseFloatError),
}

// ============================================================================
// ЗАГРУЗКА КОНФИГУРАЦИИ
// ============================================================================

impl Config {
    /// Загрузить конфигурацию из environment variables
    ///
    /// Аналог Go функции config.Load()
    /// В Rust используем associated function (аналог static method)
    ///
    /// Result<T, E> - enum с вариантами Ok(T) и Err(E)
    /// Аналог: (Config, error) в Go
    pub fn load() -> Result<Self, ConfigError> {
        // ====================================================================
        // ШАГ 1: Обязательные переменные
        // ====================================================================

        // env::var() возвращает Result<String, VarError>
        // .map_err() конвертирует VarError в ConfigError
        let name = env::var("DEPHEALTH_NAME")
            .map_err(|_| ConfigError::MissingEnvVar("DEPHEALTH_NAME".to_string()))?;

        // ====================================================================
        // ШАГ 2: Опциональные переменные с defaults
        // ====================================================================

        // unwrap_or_else() возвращает значение или выполняет closure
        let listen_addr = env::var("LISTEN_ADDR")
            .unwrap_or_else(|_| "0.0.0.0:8080".to_string());

        let log_level = env::var("LOG_LEVEL")
            .unwrap_or_else(|_| "info".to_string());

        // ====================================================================
        // ШАГ 3: Парсинг интервала проверки
        // ====================================================================

        let interval_str = env::var("DEPHEALTH_CHECK_INTERVAL")
            .unwrap_or_else(|_| "10".to_string());

        // parse::<f64>() конвертирует String в f64
        // В Rust тип нужно указывать явно (type inference)
        let seconds: f64 = interval_str
            .parse()
            .map_err(|e| ConfigError::InvalidDuration(interval_str, e))?;

        // Duration::from_secs_f64 создает Duration из секунд
        let check_interval = Duration::from_secs_f64(seconds);

        // ====================================================================
        // ШАГ 4: Парсинг зависимостей
        // ====================================================================

        let deps_str = env::var("DEPHEALTH_DEPS")
            .unwrap_or_default(); // Пустая строка если не задано

        let dependencies = if deps_str.is_empty() {
            Vec::new() // Пустой вектор
        } else {
            Self::parse_dependencies(&deps_str)?
        };

        // ====================================================================
        // ШАГ 5: Создание Config
        // ====================================================================

        // Struct initialization syntax в Rust
        Ok(Config {
            name,
            listen_addr,
            log_level,
            check_interval,
            dependencies,
        })
    }

    /// Парсинг строки зависимостей: "name1:type1,name2:type2,..."
    ///
    /// & перед self означает borrowing (не берем ownership)
    /// &str - string slice (borrowed reference на строку)
    fn parse_dependencies(s: &str) -> Result<Vec<Dependency>, ConfigError> {
        // split(',') создает итератор по подстрокам
        // collect() собирает итератор в Vec<&str>
        let pairs: Vec<&str> = s.split(',').collect();

        // С помощью итераторов и ? можно элегантно обрабатывать ошибки
        // В Go это был бы цикл for с проверкой err
        pairs
            .iter()
            .filter(|p| !p.trim().is_empty()) // Пропускаем пустые строки
            .map(|pair| Self::parse_single_dependency(pair.trim()))
            .collect() // collect::<Result<Vec<T>, E>>() - умная магия!
                       // Если хоть один элемент Err - вернет Err
                       // Если все Ok - вернет Ok(Vec<T>)
    }

    /// Парсинг одной зависимости: "name:type"
    fn parse_single_dependency(pair: &str) -> Result<Dependency, ConfigError> {
        // splitn(2, ':') - split максимум на 2 части
        let parts: Vec<&str> = pair.splitn(2, ':').collect();

        // Pattern matching - одна из самых мощных фич Rust
        match parts.as_slice() {
            // Деструктуризация: если вектор содержит ровно 2 элемента
            [name, type_str] if !name.is_empty() && !type_str.is_empty() => {
                // Парсим тип зависимости
                let dep_type = Self::parse_dependency_type(type_str)?;

                // Загружаем параметры конкретной зависимости
                Self::load_dependency_params(name, dep_type)
            }
            // Иначе - ошибка формата
            _ => Err(ConfigError::InvalidDependencyFormat(pair.to_string())),
        }
    }

    /// Парсинг типа зависимости из строки
    fn parse_dependency_type(s: &str) -> Result<DependencyType, ConfigError> {
        // to_lowercase() создает новый String в lowercase
        // as_str() конвертирует String в &str для match
        match s.to_lowercase().as_str() {
            "http" => Ok(DependencyType::Http),
            "redis" => Ok(DependencyType::Redis),
            "postgres" => Ok(DependencyType::Postgres),
            "grpc" => Ok(DependencyType::Grpc),
            _ => Err(ConfigError::UnsupportedType(s.to_string())),
        }
    }

    /// Загрузка параметров конкретной зависимости из env vars
    ///
    /// В Go: parseSingleDep(name, depType)
    fn load_dependency_params(
        name: &str,
        dep_type: DependencyType,
    ) -> Result<Dependency, ConfigError> {
        // Префикс для env переменных: DEPHEALTH_<NAME>_
        // replace('-', "_") - заменяем дефисы на подчеркивания
        // to_uppercase() - в верхний регистр
        let prefix = format!("DEPHEALTH_{}_", name.replace('-', "_").to_uppercase());

        // ====================================================================
        // URL или Host+Port
        // ====================================================================

        let url = env::var(format!("{}URL", prefix)).ok(); // .ok() конвертирует Result в Option

        let (host, port) = if url.is_none() {
            // Если URL не задан - требуем HOST и PORT
            let h = env::var(format!("{}HOST", prefix)).map_err(|_| {
                ConfigError::DependencyError(
                    name.to_string(),
                    format!("either {}URL or {}HOST + {}PORT is required", prefix, prefix, prefix),
                )
            })?;

            let p_str = env::var(format!("{}PORT", prefix)).map_err(|_| {
                ConfigError::DependencyError(
                    name.to_string(),
                    format!("{}PORT is required when URL is not set", prefix),
                )
            })?;

            // parse::<u16>() - парсим строку в unsigned 16-bit integer
            let p: u16 = p_str.parse().map_err(|_| {
                ConfigError::DependencyError(
                    name.to_string(),
                    format!("invalid port number: {}", p_str),
                )
            })?;

            (Some(h), Some(p))
        } else {
            (None, None)
        };

        // ====================================================================
        // Critical flag
        // ====================================================================

        let critical_str = env::var(format!("{}CRITICAL", prefix)).map_err(|_| {
            ConfigError::DependencyError(
                name.to_string(),
                format!("{}CRITICAL is required (yes/no)", prefix),
            )
        })?;

        let critical = match critical_str.to_lowercase().as_str() {
            "yes" | "true" | "1" => true,
            "no" | "false" | "0" => false,
            _ => {
                return Err(ConfigError::DependencyError(
                    name.to_string(),
                    format!("invalid CRITICAL value '{}' (expected yes/no)", critical_str),
                ))
            }
        };

        // ====================================================================
        // HTTP-specific: health path
        // ====================================================================

        let health_path = if dep_type == DependencyType::Http {
            env::var(format!("{}HEALTH_PATH", prefix)).ok()
        } else {
            None
        };

        // ====================================================================
        // Создаем Dependency
        // ====================================================================

        Ok(Dependency {
            name: name.to_string(), // to_string() создает owned String из &str
            dep_type,
            url,
            host,
            port,
            critical,
            health_path,
        })
    }
}

// ============================================================================
// UNIT ТЕСТЫ
// ============================================================================

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_dependency_type() {
        assert_eq!(
            Config::parse_dependency_type("http").unwrap(),
            DependencyType::Http
        );
        assert_eq!(
            Config::parse_dependency_type("HTTP").unwrap(),
            DependencyType::Http
        );
        assert!(Config::parse_dependency_type("invalid").is_err());
    }

    // Добавьте больше тестов здесь!
}

// ============================================================================
// ВАЖНЫЕ КОНЦЕПЦИИ RUST В ЭТОМ МОДУЛЕ:
// ============================================================================
//
// 1. STRUCT & ENUM
//    - Derive макросы для автогенерации trait implementations
//    - Enum как algebraic data type
//    - Pattern matching на enum
//
// 2. OWNERSHIP
//    - String (owned) vs &str (borrowed)
//    - to_string() для создания owned String
//    - clone() для создания копий
//
// 3. OPTION & RESULT
//    - Option<T> - для опциональных значений (Some/None)
//    - Result<T, E> - для операций с ошибками (Ok/Err)
//    - ? оператор для propagation ошибок
//
// 4. ERROR HANDLING
//    - thiserror для кастомных error types
//    - map_err() для конверсии ошибок
//    - unwrap_or_else() для fallback значений
//
// 5. ITERATORS
//    - split(), collect(), map(), filter()
//    - Ленивые вычисления
//    - Композиция операций
//
// 6. PATTERN MATCHING
//    - match expression - exhaustive
//    - Деструктуризация в patterns
//    - Guards (if условия в match)
//
// ============================================================================
