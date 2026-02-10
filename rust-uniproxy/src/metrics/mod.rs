// ============================================================================
// metrics/mod.rs - Prometheus метрики
// ============================================================================
//
// ОБРАЗОВАТЕЛЬНЫЕ ЦЕЛИ:
// - Интеграция с prometheus crate
// - Lazy static initialization
// - Метрики: Counter, Gauge, Histogram
//
// ============================================================================

use prometheus::{Encoder, TextEncoder, Registry, GaugeVec, HistogramVec, Opts, HistogramOpts};
use lazy_static::lazy_static;

// ============================================================================
// МЕТРИКИ
// ============================================================================

// lazy_static! - макрос для ленивой инициализации static переменных
// В Rust нельзя создавать сложные типы в static напрямую
// lazy_static! инициализирует их при первом использовании
//
// В Go аналог: sync.Once или init()
lazy_static! {
    /// Registry для метрик
    static ref REGISTRY: Registry = Registry::new();

    /// app_dependency_health - статус зависимости (1=UP, 0=DOWN)
    ///
    /// GaugeVec - gauge метрика с labels
    /// Gauge - значение которое может увеличиваться и уменьшаться
    ///
    /// Labels:
    /// - name: имя сервиса
    /// - namespace: namespace
    /// - dependency: имя зависимости
    /// - type: тип зависимости
    /// - host: хост
    /// - port: порт
    /// - critical: критичность (yes/no)
    pub static ref DEPENDENCY_HEALTH: GaugeVec = {
        let opts = Opts::new(
            "app_dependency_health",
            "Dependency health status (1=UP, 0=DOWN)"
        );

        let gauge = GaugeVec::new(
            opts,
            &["name", "namespace", "dependency", "type", "host", "port", "critical"]
        ).expect("Failed to create DEPENDENCY_HEALTH metric");

        REGISTRY.register(Box::new(gauge.clone()))
            .expect("Failed to register DEPENDENCY_HEALTH metric");

        gauge
    };

    /// app_dependency_latency_seconds - latency проверки зависимости
    ///
    /// HistogramVec - histogram метрика с labels
    /// Histogram - распределение значений по buckets
    pub static ref DEPENDENCY_LATENCY: HistogramVec = {
        let opts = HistogramOpts::new(
            "app_dependency_latency_seconds",
            "Dependency health check latency in seconds"
        )
        // Buckets для latency: 1ms, 5ms, 10ms, 50ms, 100ms, 500ms, 1s, 5s
        .buckets(vec![0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0, 5.0]);

        let histogram = HistogramVec::new(
            opts,
            &["name", "namespace", "dependency"]
        ).expect("Failed to create DEPENDENCY_LATENCY metric");

        REGISTRY.register(Box::new(histogram.clone()))
            .expect("Failed to register DEPENDENCY_LATENCY metric");

        histogram
    };
}

// ============================================================================
// ФУНКЦИИ ДЛЯ РАБОТЫ С МЕТРИКАМИ
// ============================================================================

/// Обновить метрику health для зависимости
///
/// # Arguments
/// * `name` - имя сервиса
/// * `namespace` - namespace
/// * `dependency` - имя зависимости
/// * `dep_type` - тип зависимости
/// * `host` - хост
/// * `port` - порт
/// * `critical` - критичность
/// * `is_healthy` - статус (true=UP, false=DOWN)
pub fn set_dependency_health(
    name: &str,
    namespace: &str,
    dependency: &str,
    dep_type: &str,
    host: &str,
    port: &str,
    critical: bool,
    is_healthy: bool,
) {
    let critical_str = if critical { "yes" } else { "no" };
    let value = if is_healthy { 1.0 } else { 0.0 };

    DEPENDENCY_HEALTH
        .with_label_values(&[name, namespace, dependency, dep_type, host, port, critical_str])
        .set(value);
}

/// Записать latency для проверки зависимости
pub fn observe_dependency_latency(
    name: &str,
    namespace: &str,
    dependency: &str,
    duration_secs: f64,
) {
    DEPENDENCY_LATENCY
        .with_label_values(&[name, namespace, dependency])
        .observe(duration_secs);
}

/// Получить все метрики в формате Prometheus text
///
/// Возвращает String с метриками в формате, который понимает Prometheus
pub fn get_metrics() -> Result<String, Box<dyn std::error::Error>> {
    let encoder = TextEncoder::new();
    let metric_families = REGISTRY.gather();

    let mut buffer = Vec::new();
    encoder.encode(&metric_families, &mut buffer)?;

    // Vec<u8> -> String
    Ok(String::from_utf8(buffer)?)
}

// ============================================================================
// ВАЖНЫЕ КОНЦЕПЦИИ RUST В ЭТОМ МОДУЛЕ:
// ============================================================================
//
// 1. LAZY STATIC
//    - lazy_static! для инициализации static переменных
//    - Инициализация при первом обращении
//    - Thread-safe (использует Once под капотом)
//
// 2. PROMETHEUS METRICS
//    - Gauge - значение которое может изменяться
//    - Histogram - распределение значений
//    - Counter - монотонно возрастающее значение (не используем здесь)
//
// 3. LABELS
//    - with_label_values() для установки значений labels
//    - &[] - slice литерал для передачи массива
//
// 4. ERROR HANDLING
//    - expect() для паники при ошибках инициализации
//    - Result<T, Box<dyn Error>> для гибкого error handling
//
// 5. STRING CONVERSIONS
//    - Vec<u8> -> String через from_utf8()
//    - &str для string slices
//
// ============================================================================
//
// ИСПОЛЬЗОВАНИЕ:
//
// ```rust
// use crate::metrics;
//
// // Обновить статус зависимости
// metrics::set_dependency_health(
//     "uniproxy",
//     "default",
//     "postgres-main",
//     "postgres",
//     "localhost",
//     "5432",
//     true,
//     true, // UP
// );
//
// // Записать latency
// metrics::observe_dependency_latency(
//     "uniproxy",
//     "default",
//     "postgres-main",
//     0.05, // 50ms
// );
//
// // Получить все метрики
// let metrics_text = metrics::get_metrics()?;
// ```
//
// ============================================================================
