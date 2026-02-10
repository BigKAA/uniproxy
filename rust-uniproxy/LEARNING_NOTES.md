# Образовательные заметки по Rust

**Полный гид по концепциям Rust на примере uniproxy**

---

## 📚 Содержание

1. [Ownership & Borrowing](#1-ownership--borrowing)
2. [String vs &str](#2-string-vs-str)
3. [Option и Result](#3-option-и-result)
4. [Error Handling](#4-error-handling)
5. [Traits](#5-traits)
6. [Async/Await](#6-asyncawait)
7. [Concurrency](#7-concurrency)
8. [Pattern Matching](#8-pattern-matching)
9. [Lifetimes](#9-lifetimes)
10. [Smart Pointers](#10-smart-pointers)

---

## 1. Ownership & Borrowing

### Основные правила

Rust использует **ownership model** для управления памятью без Garbage Collector:

1. Каждое значение имеет **одного владельца** (owner)
2. Когда owner выходит из scope, значение уничтожается (drop)
3. Значение может быть **перемещено** (moved) или **заимствовано** (borrowed)

### Примеры из кода

```rust
// Ownership - создание owned String
let name = String::from("uniproxy");
// name владеет строкой в heap

// Move - перенос ownership
let name2 = name;
// name больше недоступен! Ownership перешел к name2

// Clone - создание копии
let name3 = name2.clone();
// name2 и name3 оба владеют своими копиями
```

### Borrowing (заимствование)

```rust
fn print_name(s: &String) {
    // &String - immutable borrow (чтение)
    println!("{}", s);
}

fn modify_name(s: &mut String) {
    // &mut String - mutable borrow (запись)
    s.push_str("-rust");
}

let mut name = String::from("uniproxy");
print_name(&name);        // Borrowing
modify_name(&mut name);   // Mutable borrowing
```

### Правила borrowing

- Можно иметь **любое количество immutable borrows** (`&T`)
- Можно иметь **только один mutable borrow** (`&mut T`)
- Нельзя одновременно иметь mutable и immutable borrows

### В нашем коде

```rust
// src/config/mod.rs
impl Config {
    pub fn load() -> Result<Self, ConfigError> {
        let name = env::var("DEPHEALTH_NAME")?;
        // name - owned String

        Ok(Config {
            name, // move ownership в struct
            // ...
        })
    }

    fn parse_single_dependency(pair: &str) -> Result<Dependency, ConfigError> {
        // pair - borrowed &str (не владеем данными)
        // ...
    }
}
```

---

## 2. String vs &str

### String

- **Owned** string (в heap)
- Изменяемый (если `mut`)
- Автоматически освобождается при drop

```rust
let mut s = String::from("hello");
s.push_str(" world"); // можно изменять
// s автоматически drop когда выходит из scope
```

### &str

- **Borrowed** string slice
- Immutable
- Не владеет данными (указатель + длина)

```rust
let s: &str = "hello"; // string literal (&'static str)

let owned = String::from("hello world");
let slice: &str = &owned[0..5]; // "hello" - slice
```

### Когда использовать

| Ситуация | Используйте |
|----------|-------------|
| Нужно владеть строкой | `String` |
| Строка будет изменяться | `String` |
| Возврат из функции | `String` |
| Только чтение | `&str` |
| Параметр функции (чтение) | `&str` |
| String literal | `&str` |

### В нашем коде

```rust
// src/config/mod.rs
pub struct Config {
    pub name: String,  // owned - нужно владеть данными
}

fn parse_dependency_type(s: &str) -> Result<DependencyType, ConfigError> {
    // s: &str - просто читаем, не нужен ownership
    match s.to_lowercase().as_str() {
        "http" => Ok(DependencyType::Http),
        // ...
    }
}
```

---

## 3. Option и Result

### Option<T>

Для значений, которые могут отсутствовать:

```rust
enum Option<T> {
    Some(T),  // Есть значение
    None,     // Нет значения
}
```

**Пример**:
```rust
let maybe_name: Option<String> = env::var("NAME").ok();

match maybe_name {
    Some(name) => println!("Name: {}", name),
    None => println!("No name"),
}

// Или короче:
let name = maybe_name.unwrap_or("default".to_string());
```

### Result<T, E>

Для операций, которые могут завершиться ошибкой:

```rust
enum Result<T, E> {
    Ok(T),   // Успех
    Err(E),  // Ошибка
}
```

**Пример**:
```rust
fn load_config() -> Result<Config, ConfigError> {
    let name = env::var("NAME")
        .map_err(|_| ConfigError::Missing)?;

    Ok(Config { name })
}
```

### Методы Option/Result

```rust
// unwrap() - получить значение или panic
let x = Some(5).unwrap(); // 5
let y = None.unwrap(); // panic!

// unwrap_or() - значение или default
let x = None.unwrap_or(42); // 42

// unwrap_or_else() - значение или вычислить
let x = None.unwrap_or_else(|| expensive_computation());

// map() - преобразовать значение
let x = Some(5).map(|n| n * 2); // Some(10)

// and_then() - chain операций
let result = get_user()
    .and_then(|user| get_email(user))
    .and_then(|email| send_message(email));
```

### В нашем коде

```rust
// src/config/mod.rs
pub struct Dependency {
    pub url: Option<String>,      // может не быть
    pub host: Option<String>,     // может не быть
    pub port: Option<u16>,        // может не быть
}

impl Config {
    pub fn load() -> Result<Self, ConfigError> {
        // Result для обработки ошибок
        let name = env::var("DEPHEALTH_NAME")
            .map_err(|_| ConfigError::MissingEnvVar("DEPHEALTH_NAME".to_string()))?;

        Ok(Config { name, /* ... */ })
    }
}
```

---

## 4. Error Handling

### Стратегии обработки ошибок

1. **Panic** - для невосстановимых ошибок
2. **Result** - для восстановимых ошибок
3. **Option** - для опциональных значений

### ? оператор

Упрощает propagation ошибок:

```rust
// Без ?
fn load_config() -> Result<Config, ConfigError> {
    let name = match env::var("NAME") {
        Ok(n) => n,
        Err(_) => return Err(ConfigError::Missing),
    };
    Ok(Config { name })
}

// С ?
fn load_config() -> Result<Config, ConfigError> {
    let name = env::var("NAME")
        .map_err(|_| ConfigError::Missing)?;
    Ok(Config { name })
}
```

### Кастомные Error types с thiserror

```rust
use thiserror::Error;

#[derive(Debug, Error)]
pub enum ConfigError {
    #[error("environment variable '{0}' is required")]
    MissingEnvVar(String),

    #[error("invalid dependency format '{0}'")]
    InvalidFormat(String),
}

// Использование:
return Err(ConfigError::MissingEnvVar("NAME".to_string()));
```

### map_err для конверсии ошибок

```rust
let num: u16 = port_str
    .parse()
    .map_err(|e| ConfigError::InvalidPort(port_str, e))?;
```

### В нашем коде

```rust
// src/config/mod.rs
#[derive(Debug, thiserror::Error)]
pub enum ConfigError {
    #[error("environment variable '{0}' is required")]
    MissingEnvVar(String),
    // ...
}

impl Config {
    pub fn load() -> Result<Self, ConfigError> {
        // ? propagates ошибку
        let name = env::var("DEPHEALTH_NAME")
            .map_err(|_| ConfigError::MissingEnvVar("DEPHEALTH_NAME".to_string()))?;

        let dependencies = Self::parse_dependencies(&deps_str)?;

        Ok(Config { name, dependencies, /* ... */ })
    }
}
```

---

## 5. Traits

### Что такое trait

Trait - это аналог **interface** в Go. Определяет набор методов, которые тип должен реализовать.

```rust
// Определение trait
trait Animal {
    fn make_sound(&self) -> String;
    fn name(&self) -> &str;
}

// Реализация trait
struct Dog {
    name: String,
}

impl Animal for Dog {
    fn make_sound(&self) -> String {
        "Woof!".to_string()
    }

    fn name(&self) -> &str {
        &self.name
    }
}
```

### Trait bounds

```rust
// Функция принимает любой тип, который implements Animal
fn greet<T: Animal>(animal: &T) {
    println!("{} says {}", animal.name(), animal.make_sound());
}

// Или с where clause
fn greet<T>(animal: &T)
where
    T: Animal
{
    // ...
}
```

### Trait objects (dynamic dispatch)

```rust
// Vec разных типов через trait object
let animals: Vec<Box<dyn Animal>> = vec![
    Box::new(Dog { name: "Rex".to_string() }),
    Box::new(Cat { name: "Whiskers".to_string() }),
];

for animal in animals {
    println!("{}", animal.make_sound());
}
```

### Derive макросы

```rust
#[derive(Debug, Clone, PartialEq)]
struct Config {
    name: String,
}

// Теперь можно:
let config = Config { name: "test".to_string() };
println!("{:?}", config);        // Debug
let config2 = config.clone();    // Clone
assert_eq!(config, config2);     // PartialEq
```

### В нашем коде

```rust
// src/health/mod.rs
#[async_trait::async_trait]
pub trait HealthChecker: Send + Sync {
    async fn check(&self) -> Result<bool, Box<dyn std::error::Error>>;
    fn name(&self) -> &str;
    fn is_critical(&self) -> bool;
}

// Реализация для HTTP
impl HealthChecker for HttpChecker {
    async fn check(&self) -> Result<bool, Box<dyn std::error::Error>> {
        let response = self.client.get(&self.url).send().await?;
        Ok(response.status().is_success())
    }
    // ...
}

// Использование trait objects
pub struct HealthManager {
    checkers: Vec<Box<dyn HealthChecker>>, // разные типы checkers
}
```

---

## 6. Async/Await

### Основы

Async/await в Rust - это zero-cost abstraction для асинхронного программирования.

```rust
// Async функция возвращает Future
async fn fetch_data() -> Result<String, Error> {
    let response = reqwest::get("https://api.example.com")
        .await?; // .await приостанавливает выполнение

    let text = response.text().await?;
    Ok(text)
}
```

### Tokio Runtime

Для выполнения async кода нужен runtime:

```rust
#[tokio::main]
async fn main() {
    let result = fetch_data().await;
    println!("{:?}", result);
}
```

### tokio::spawn

Запуск async task в фоне:

```rust
let handle = tokio::spawn(async {
    // async работа
    fetch_data().await
});

// Ждем завершения
let result = handle.await.unwrap();
```

### select! макрос

Ожидание нескольких futures:

```rust
tokio::select! {
    result1 = future1 => {
        println!("Future1 completed first: {:?}", result1);
    }
    result2 = future2 => {
        println!("Future2 completed first: {:?}", result2);
    }
}
```

### В нашем коде

```rust
// src/main.rs
#[tokio::main]
async fn main() -> anyhow::Result<()> {
    // Запуск health checks в фоне
    let health_manager_bg = health_manager.clone();
    tokio::spawn(async move {
        health_manager_bg.start_periodic_checks().await;
    });

    // Graceful shutdown
    tokio::signal::ctrl_c().await?;
    Ok(())
}

// src/health/mod.rs
pub async fn start_periodic_checks(&self) {
    let mut interval = time::interval(self.check_interval);

    loop {
        interval.tick().await; // Ждем следующего тика
        self.check_all().await;
    }
}
```

---

## 7. Concurrency

### Send и Sync traits

- `Send` - тип можно передавать между threads
- `Sync` - тип безопасен для concurrent access (`&T` is `Send`)

```rust
// Автоматически Send + Sync
struct MyStruct {
    data: i32,
}

// Требуется для trait objects в multi-threaded контексте
trait HealthChecker: Send + Sync {
    // ...
}
```

### Arc<T> - Atomic Reference Counting

Для shared ownership между threads:

```rust
use std::sync::Arc;

let data = Arc::new(vec![1, 2, 3]);

let data_clone = data.clone(); // Дешевая операция (инкремент счетчика)
tokio::spawn(async move {
    println!("{:?}", data_clone);
});

println!("{:?}", data); // Оригинал доступен
```

### Mutex<T> и RwLock<T>

Для мутабельного shared state:

```rust
use std::sync::{Arc, Mutex};

let counter = Arc::new(Mutex::new(0));

let counter_clone = counter.clone();
tokio::spawn(async move {
    let mut num = counter_clone.lock().await;
    *num += 1;
});
```

**RwLock** - multiple readers или one writer:

```rust
use tokio::sync::RwLock;

let data = Arc::new(RwLock::new(HashMap::new()));

// Multiple readers
let data1 = data.clone();
let reader1 = tokio::spawn(async move {
    let map = data1.read().await;
    println!("{:?}", map);
});

// One writer
let data2 = data.clone();
let writer = tokio::spawn(async move {
    let mut map = data2.write().await;
    map.insert("key", "value");
});
```

### В нашем коде

```rust
// src/health/mod.rs
pub struct HealthManager {
    checkers: Vec<Box<dyn HealthChecker>>, // Send + Sync required
    status: Arc<RwLock<HashMap<String, bool>>>, // Shared mutable state
    check_interval: Duration,
}

impl HealthManager {
    pub async fn check_all(&self) {
        for checker in &self.checkers {
            let result = checker.check().await;

            // Write lock
            let mut status_map = self.status.write().await;
            status_map.insert(checker.name().to_string(), result.unwrap_or(false));
        }
    }

    pub async fn get_health_status(&self) -> HashMap<String, bool> {
        // Read lock
        self.status.read().await.clone()
    }
}
```

---

## 8. Pattern Matching

### match expression

Exhaustive pattern matching:

```rust
let x = Some(5);

match x {
    Some(val) => println!("Value: {}", val),
    None => println!("No value"),
}

// С enum
match dep_type {
    DependencyType::Http => create_http_checker(),
    DependencyType::Redis => create_redis_checker(),
    DependencyType::Postgres => create_postgres_checker(),
    DependencyType::Grpc => create_grpc_checker(),
}
```

### Деструктуризация

```rust
// Tuple
let (x, y) = (1, 2);

// Struct
struct Point { x: i32, y: i32 }
let Point { x, y } = point;

// В match
match result {
    Ok(value) => println!("Success: {}", value),
    Err(e) => println!("Error: {}", e),
}
```

### Guards

```rust
match value {
    x if x < 0 => println!("Negative"),
    x if x > 0 => println!("Positive"),
    _ => println!("Zero"),
}
```

### if let

Для случаев когда интересует только один pattern:

```rust
// Вместо:
match optional {
    Some(value) => println!("{}", value),
    None => {}
}

// Используем:
if let Some(value) = optional {
    println!("{}", value);
}
```

### В нашем коде

```rust
// src/config/mod.rs
fn parse_single_dependency(pair: &str) -> Result<Dependency, ConfigError> {
    let parts: Vec<&str> = pair.splitn(2, ':').collect();

    match parts.as_slice() {
        [name, type_str] if !name.is_empty() && !type_str.is_empty() => {
            // Деструктуризация + guard
            let dep_type = Self::parse_dependency_type(type_str)?;
            Self::load_dependency_params(name, dep_type)
        }
        _ => Err(ConfigError::InvalidDependencyFormat(pair.to_string())),
    }
}

fn parse_dependency_type(s: &str) -> Result<DependencyType, ConfigError> {
    match s.to_lowercase().as_str() {
        "http" => Ok(DependencyType::Http),
        "redis" => Ok(DependencyType::Redis),
        "postgres" => Ok(DependencyType::Postgres),
        "grpc" => Ok(DependencyType::Grpc),
        _ => Err(ConfigError::UnsupportedType(s.to_string())),
    }
}
```

---

## 9. Lifetimes

### Что такое lifetime

Lifetime - это механизм для отслеживания **как долго живут references**.

```rust
// Compiler выводит lifetimes автоматически
fn first_word(s: &str) -> &str {
    s.split_whitespace().next().unwrap()
}

// Эквивалентно:
fn first_word<'a>(s: &'a str) -> &'a str {
    s.split_whitespace().next().unwrap()
}
```

### Когда нужны явные lifetimes

Когда у функции несколько references и compiler не может вывести отношения:

```rust
// Ошибка компиляции - непонятно какой lifetime у результата
fn longest(x: &str, y: &str) -> &str {
    if x.len() > y.len() { x } else { y }
}

// Правильно - явно указываем, что результат живет столько же, сколько x и y
fn longest<'a>(x: &'a str, y: &'a str) -> &'a str {
    if x.len() > y.len() { x } else { y }
}
```

### 'static lifetime

`'static` - lifetime на весь runtime программы:

```rust
let s: &'static str = "hello"; // String literal живет всегда
```

### В нашем коде

В большинстве случаев compiler выводит lifetimes автоматически:

```rust
// src/health/mod.rs
pub trait HealthChecker: Send + Sync {
    fn name(&self) -> &str; // Lifetime не указан, но compiler выводит его
}

// Эквивалентно:
pub trait HealthChecker: Send + Sync {
    fn name<'a>(&'a self) -> &'a str;
}
```

---

## 10. Smart Pointers

### Box<T>

Heap allocation, single owner:

```rust
let b = Box::new(5);
println!("{}", b); // 5
// b автоматически drop когда выходит из scope
```

**Использование**:
- Рекурсивные типы
- Trait objects
- Большие данные (избежать stack overflow)

```rust
// Trait object
let animals: Vec<Box<dyn Animal>> = vec![
    Box::new(Dog { name: "Rex".to_string() }),
    Box::new(Cat { name: "Whiskers".to_string() }),
];
```

### Rc<T> и Arc<T>

Reference counting для shared ownership:

```rust
use std::rc::Rc; // Single-threaded
use std::sync::Arc; // Multi-threaded (Atomic)

let data = Arc::new(vec![1, 2, 3]);
let data_clone = data.clone(); // Инкремент счетчика

// Когда последний Arc drop - данные освобождаются
```

### RefCell<T> и Mutex<T>

Interior mutability - изменение через immutable reference:

```rust
use std::cell::RefCell;

let data = RefCell::new(5);
*data.borrow_mut() = 10; // Мутабельный borrow через immutable reference

// Thread-safe версия:
use std::sync::Mutex;

let data = Mutex::new(5);
*data.lock().unwrap() = 10;
```

### В нашем коде

```rust
// src/health/mod.rs
pub struct HealthManager {
    // Box<dyn Trait> - trait objects на heap
    checkers: Vec<Box<dyn HealthChecker>>,

    // Arc<RwLock<T>> - shared mutable state
    status: Arc<RwLock<HashMap<String, bool>>>,
}

impl HealthManager {
    fn create_checker(dep: &Dependency) -> Result<Box<dyn HealthChecker>, String> {
        match dep.dep_type {
            DependencyType::Http => Ok(Box::new(HttpChecker::new(dep)?)),
            // Box помещает checker на heap
        }
    }
}

// src/main.rs
let health_manager = Arc::new(HealthManager::new(...));

let health_manager_bg = health_manager.clone();
tokio::spawn(async move {
    // Arc позволяет shared ownership
    health_manager_bg.start_periodic_checks().await;
});
```

---

## 📊 Сравнительная таблица: Go vs Rust

| Концепция | Go | Rust |
|-----------|-----|------|
| **Ownership** | GC | Ownership + Borrowing |
| **Strings** | `string` | `String` + `&str` |
| **Nullable** | `nil` | `Option<T>` |
| **Errors** | `error` | `Result<T, E>` |
| **Interface** | `interface{}` | `trait` |
| **Concurrency** | `goroutine` + `channel` | `async/await` + `tokio` |
| **Shared state** | `*sync.Mutex{}` | `Arc<Mutex<T>>` |
| **Generics** | Есть (1.18+) | Есть (с bounds) |
| **Pattern matching** | `switch` | `match` |
| **NULL safety** | Нет | `Option<T>` |
| **Memory safety** | GC + runtime checks | Compile-time проверки |

---

## 🎯 Практические упражнения

### 1. Ownership

Попробуйте исправить эту ошибку:
```rust
let s = String::from("hello");
let s2 = s;
println!("{}", s); // Ошибка! s moved
```

<details>
<summary>Решение</summary>

```rust
let s = String::from("hello");
let s2 = s.clone(); // Клонируем вместо move
println!("{}", s);
```
</details>

### 2. Result и ?

Напишите функцию, которая читает две env переменные и возвращает их сумму:

```rust
fn sum_env_vars() -> Result<i32, Box<dyn std::error::Error>> {
    // TODO: Реализовать
}
```

<details>
<summary>Решение</summary>

```rust
fn sum_env_vars() -> Result<i32, Box<dyn std::error::Error>> {
    let a = env::var("VAR_A")?.parse::<i32>()?;
    let b = env::var("VAR_B")?.parse::<i32>()?;
    Ok(a + b)
}
```
</details>

### 3. Async

Напишите async функцию, которая делает HTTP запрос и возвращает status code:

```rust
async fn fetch_status(url: &str) -> Result<u16, reqwest::Error> {
    // TODO: Реализовать
}
```

<details>
<summary>Решение</summary>

```rust
async fn fetch_status(url: &str) -> Result<u16, reqwest::Error> {
    let response = reqwest::get(url).await?;
    Ok(response.status().as_u16())
}
```
</details>

---

## 📚 Дополнительные ресурсы

### Официальная документация
- [The Rust Book](https://doc.rust-lang.org/book/)
- [Rust by Example](https://doc.rust-lang.org/rust-by-example/)
- [Async Book](https://rust-lang.github.io/async-book/)

### Интерактивное обучение
- [Rustlings](https://github.com/rust-lang/rustlings)
- [Exercism Rust Track](https://exercism.org/tracks/rust)

### Продвинутые темы
- [Too Many Lists](https://rust-unofficial.github.io/too-many-lists/)
- [Rust Nomicon](https://doc.rust-lang.org/nomicon/) - unsafe Rust

---

**Удачи в изучении Rust! 🦀**
