# Standard Library database/sql

`database/sql` — стандартный Go abstraction layer для SQL databases. Не сам драйвер, а интерфейс поверх драйвера.

## Содержание

- [Архитектура](#архитектура)
- [Подключение и pool config](#подключение-и-pool-config)
- [Основные операции](#основные-операции)
- [Transaction pattern](#transaction-pattern)
- [Nullable types](#nullable-types)
- [rows.Err() — типичный баг](#rowserr--типичный-баг)
- [lib/pq — PostgreSQL driver](#libpq--postgresql-driver)
- [Когда выбирать](#когда-выбирать)
- [Interview-ready answer](#interview-ready-answer)

## Архитектура

```
database/sql (stdlib)
    └── driver.Driver (interface)
            ├── lib/pq          (PostgreSQL, legacy)
            ├── pgx/stdlib      (PostgreSQL, modern)
            ├── go-sqlite3      (SQLite)
            └── go-sql-driver   (MySQL)
```

`sql.Open` не открывает соединение — только регистрирует конфиг. Реальное соединение открывается при первом запросе или `db.Ping`.

## Подключение и pool config

```go
db, err := sql.Open("pgx", dsn)
if err != nil {
    return fmt.Errorf("open db: %w", err)
}

// Pool settings — обязательно для production
db.SetMaxOpenConns(25)           // максимум открытых соединений
db.SetMaxIdleConns(10)           // держать idle в pool
db.SetConnMaxLifetime(5 * time.Minute)  // пересоздавать соединение через N
db.SetConnMaxIdleTime(2 * time.Minute)  // закрывать idle через N

// Verify на старте
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
if err := db.PingContext(ctx); err != nil {
    return fmt.Errorf("ping db: %w", err)
}
```

**Типичные ошибки с pool:**
- `MaxOpenConns` не задан → неограниченный рост соединений под нагрузкой
- `MaxIdleConns > MaxOpenConns` → idle connections никогда не будут использованы
- `ConnMaxLifetime` не задан → «мёртвые» соединения не пересоздаются (firewall timeout)

## Основные операции

```go
// QueryRow — один результат
func (r *UserRepository) GetByID(ctx context.Context, id int64) (User, error) {
    var u User
    err := r.db.QueryRowContext(ctx, `
        SELECT id, email, created_at
        FROM users
        WHERE id = $1
    `, id).Scan(&u.ID, &u.Email, &u.CreatedAt)
    if errors.Is(err, sql.ErrNoRows) {
        return User{}, ErrNotFound
    }
    return u, err
}

// Query — несколько строк
func (r *UserRepository) ListActive(ctx context.Context) ([]User, error) {
    rows, err := r.db.QueryContext(ctx, `
        SELECT id, email FROM users WHERE active = true
    `)
    if err != nil {
        return nil, err
    }
    defer rows.Close()  // обязательно

    var users []User
    for rows.Next() {
        var u User
        if err := rows.Scan(&u.ID, &u.Email); err != nil {
            return nil, err
        }
        users = append(users, u)
    }
    return users, rows.Err()  // rows.Err() — обязательно
}

// Exec — INSERT/UPDATE/DELETE
func (r *UserRepository) Create(ctx context.Context, email string) (int64, error) {
    var id int64
    err := r.db.QueryRowContext(ctx, `
        INSERT INTO users (email) VALUES ($1) RETURNING id
    `, email).Scan(&id)
    return id, err
}
```

## Transaction pattern

```go
func (r *UserRepository) Transfer(ctx context.Context, fromID, toID int64, amount int) error {
    tx, err := r.db.BeginTx(ctx, &sql.TxOptions{
        Isolation: sql.LevelReadCommitted,
    })
    if err != nil {
        return fmt.Errorf("begin tx: %w", err)
    }
    defer tx.Rollback() // no-op после Commit; откатит при panic или ранней return

    if _, err := tx.ExecContext(ctx, `
        UPDATE accounts SET balance = balance - $1 WHERE id = $2
    `, amount, fromID); err != nil {
        return fmt.Errorf("debit: %w", err)
    }

    if _, err := tx.ExecContext(ctx, `
        UPDATE accounts SET balance = balance + $1 WHERE id = $2
    `, amount, toID); err != nil {
        return fmt.Errorf("credit: %w", err)
    }

    return tx.Commit()
}
```

**Правило:** `defer tx.Rollback()` сразу после `BeginTx`. После `Commit` Rollback вернёт ошибку `sql.ErrTxDone`, которую мы игнорируем.

## Nullable types

Если колонка в БД может быть NULL, нельзя сканировать в `string` или `int64` — паника/ошибка.

```go
// Неправильно — паника если email NULL
var email string
rows.Scan(&email)

// Правильно — sql.NullXxx
var email sql.NullString
rows.Scan(&email)
if email.Valid {
    user.Email = email.String
}

// Стандартные nullable типы
sql.NullString
sql.NullInt64
sql.NullInt32
sql.NullFloat64
sql.NullBool
sql.NullTime

// Альтернатива — указатели
var email *string
rows.Scan(&email)
// email == nil если NULL в БД
```

## rows.Err() — типичный баг

```go
// Неправильно — пропускаем ошибку итерации
for rows.Next() {
    rows.Scan(...)
}
return users, nil  // ошибка network/timeout потеряна

// Правильно
for rows.Next() {
    rows.Scan(...)
}
return users, rows.Err()
```

`rows.Next()` возвращает `false` как при конце результата, так и при ошибке. Ошибку можно узнать только через `rows.Err()` после выхода из цикла.

## lib/pq — PostgreSQL driver

`github.com/lib/pq` — legacy PostgreSQL driver для `database/sql`. Сейчас рекомендуется `pgx/stdlib` вместо него, но `lib/pq` встречается в старых проектах.

```go
import (
    "database/sql"
    _ "github.com/lib/pq"  // side-effect import — регистрирует driver
)

db, err := sql.Open("postgres", "host=localhost user=app dbname=mydb sslmode=disable")
```

**Работа с PostgreSQL-specific ошибками через lib/pq:**

```go
import "github.com/lib/pq"

func isUniqueViolation(err error) bool {
    var pqErr *pq.Error
    if errors.As(err, &pqErr) {
        return pqErr.Code == "23505" // unique_violation
    }
    return false
}

// Коды ошибок PostgreSQL
// 23505 — unique_violation
// 23503 — foreign_key_violation
// 23502 — not_null_violation
// 23514 — check_violation
// 40001 — serialization_failure (для SERIALIZABLE транзакций)
```

**Почему pgx вместо lib/pq:**
- `lib/pq` не поддерживает `pgx`-native features: batch queries, copy protocol, extended query protocol
- `lib/pq` maintenance mode — активно не развивается
- `pgx` быстрее и поддерживает `pgtype` для сложных типов

## Когда выбирать

`database/sql` хорошо подходит, если:
- нужна база под нескольких SQL drivers (MySQL, PostgreSQL, SQLite)
- минимум внешних зависимостей
- команда уверенно пишет SQL и хочет полный контроль
- используется как основа для `sqlx` или `sqlc`

Когда смотреть в другую сторону:
- PostgreSQL-only → `pgxpool` нативнее и мощнее
- много ручного `Scan` → `sqlx` или `sqlc`

## Interview-ready answer

`database/sql` — стандартный abstraction layer, который предоставляет общий API и connection pooling, не привязывая к конкретной БД. Сам по себе не является драйвером — нужен `lib/pq`, `pgx/stdlib` и т.д. Для production важно: задать pool limits (`SetMaxOpenConns`, `ConnMaxLifetime`), всегда передавать context с timeout, закрывать `rows.Close()`, проверять `rows.Err()` после цикла и использовать `defer tx.Rollback()` в транзакциях. Сейчас для PostgreSQL-проектов чаще выбирают pgxpool напрямую, потому что он предоставляет нативный PostgreSQL API с поддержкой batch queries и pgtype.
