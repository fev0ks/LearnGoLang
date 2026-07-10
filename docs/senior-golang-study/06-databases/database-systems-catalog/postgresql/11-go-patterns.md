# PostgreSQL в Go: паттерны с pgx

## Содержание

- [pgx v5: основы](#pgx-v5-основы)
- [pgxpool: конфигурация](#pgxpool-конфигурация)
- [Query паттерны](#query-паттерны)
- [Scan паттерны](#scan-паттерны)
- [Транзакции](#транзакции)
- [Batch запросы](#batch-запросы)
- [COPY protocol: bulk insert](#copy-protocol-bulk-insert)
- [Обработка ошибок PostgreSQL](#обработка-ошибок-postgresql)
- [Context и timeout](#context-и-timeout)
- [pgtype: нативные типы PostgreSQL](#pgtype-нативные-типы-postgresql)
- [Партиционированные таблицы из Go](#партиционированные-таблицы-из-go)
- [Interview-ready answer](#interview-ready-answer)

---

## pgx v5: основы

`pgx` — рекомендуемый драйвер для Go. Быстрее `database/sql`, нативно поддерживает типы PostgreSQL.

```go
import (
    "context"
    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/jackc/pgx/v5/pgconn"
)
```

Два интерфейса для работы:
- `*pgxpool.Pool` — пул соединений, используется в большинстве случаев.
- `*pgx.Conn` — единственное соединение (когда нужен session-level state).

---

## pgxpool: конфигурация

```go
func NewPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
    cfg, err := pgxpool.ParseConfig(dsn)
    if err != nil {
        return nil, fmt.Errorf("parse dsn: %w", err)
    }

    cfg.MaxConns = 20
    cfg.MinConns = 2
    cfg.MaxConnLifetime = 1 * time.Hour
    cfg.MaxConnIdleTime = 30 * time.Minute
    cfg.HealthCheckPeriod = 1 * time.Minute

    // Для работы через PgBouncer в transaction mode
    cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeCacheDescribe

    cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
        // инициализация соединения: SET search_path, type registration
        return nil
    }

    pool, err := pgxpool.NewWithConfig(ctx, cfg)
    if err != nil {
        return nil, fmt.Errorf("create pool: %w", err)
    }

    // проверить что база доступна
    if err := pool.Ping(ctx); err != nil {
        pool.Close()
        return nil, fmt.Errorf("ping: %w", err)
    }

    return pool, nil
}
```

---

## Query паттерны

### QueryRow — одна строка

```go
var (
    id    int64
    email string
)
err := pool.QueryRow(ctx, "SELECT id, email FROM users WHERE id = $1", userID).
    Scan(&id, &email)
if err != nil {
    if errors.Is(err, pgx.ErrNoRows) {
        return nil, ErrNotFound
    }
    return nil, fmt.Errorf("get user: %w", err)
}
```

### Query — несколько строк

```go
rows, err := pool.Query(ctx,
    "SELECT id, email, created_at FROM users WHERE status = $1 ORDER BY created_at DESC LIMIT $2",
    "active", 50,
)
if err != nil {
    return nil, err
}
defer rows.Close()  // ВСЕГДА

for rows.Next() {
    var u User
    if err := rows.Scan(&u.ID, &u.Email, &u.CreatedAt); err != nil {
        return nil, err
    }
    users = append(users, u)
}
// проверить ошибку после итерации
if err := rows.Err(); err != nil {
    return nil, err
}
```

Если не вызвать `rows.Close()` — соединение не вернётся в пул.

### Exec — без возврата строк

```go
result, err := pool.Exec(ctx,
    "UPDATE users SET status = $1 WHERE id = $2",
    "inactive", userID,
)
if err != nil {
    return err
}
// число затронутых строк
rowsAffected := result.RowsAffected()
```

---

## Scan паттерны

### pgx.CollectRows — удобнее rows.Next()

```go
rows, err := pool.Query(ctx, "SELECT id, email FROM users WHERE status = $1", "active")
if err != nil {
    return nil, err
}

// автоматически закрывает rows
users, err := pgx.CollectRows(rows, pgx.RowToStructByName[User])
if err != nil {
    return nil, err
}
```

`RowToStructByName` сканирует по именам столбцов в struct tag `db`:

```go
type User struct {
    ID        int64     `db:"id"`
    Email     string    `db:"email"`
    CreatedAt time.Time `db:"created_at"`
}
```

### pgx.CollectOneRow

```go
row, err := pool.Query(ctx, "SELECT id, email FROM users WHERE id = $1", id)
if err != nil {
    return nil, err
}

user, err := pgx.CollectOneRow(row, pgx.RowToStructByName[User])
if errors.Is(err, pgx.ErrNoRows) {
    return nil, ErrNotFound
}
```

### pgx.RowToMap

```go
users, err := pgx.CollectRows(rows, pgx.RowToMap)
// []map[string]any
```

---

## Транзакции

### Базовый паттерн

```go
func Transfer(ctx context.Context, pool *pgxpool.Pool, fromID, toID int64, amount int64) error {
    tx, err := pool.Begin(ctx)
    if err != nil {
        return err
    }
    defer tx.Rollback(ctx)  // no-op если уже Commit

    _, err = tx.Exec(ctx,
        "UPDATE accounts SET balance = balance - $1 WHERE id = $2",
        amount, fromID,
    )
    if err != nil {
        return err
    }

    _, err = tx.Exec(ctx,
        "UPDATE accounts SET balance = balance + $1 WHERE id = $2",
        amount, toID,
    )
    if err != nil {
        return err
    }

    return tx.Commit(ctx)
}
```

### Транзакция с isolation level

```go
tx, err := pool.BeginTx(ctx, pgx.TxOptions{
    IsoLevel:   pgx.RepeatableRead,
    AccessMode: pgx.ReadWrite,
})
```

### Функция-обёртка для транзакций

```go
func WithTx(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
    tx, err := pool.Begin(ctx)
    if err != nil {
        return err
    }
    defer tx.Rollback(ctx)

    if err := fn(tx); err != nil {
        return err
    }
    return tx.Commit(ctx)
}

// использование
err := WithTx(ctx, pool, func(tx pgx.Tx) error {
    _, err := tx.Exec(ctx, "INSERT INTO orders ...", ...)
    return err
})
```

### Retry для SERIALIZABLE

```go
func RunSerializable(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
    for {
        tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
        if err != nil {
            return err
        }

        err = fn(tx)
        if err != nil {
            tx.Rollback(ctx)
            if isSerializationFailure(err) {
                continue
            }
            return err
        }

        if err := tx.Commit(ctx); err != nil {
            if isSerializationFailure(err) {
                continue
            }
            return err
        }
        return nil
    }
}

func isSerializationFailure(err error) bool {
    var pgErr *pgconn.PgError
    return errors.As(err, &pgErr) && pgErr.Code == "40001"
}
```

---

## Batch запросы

`pgx.Batch` — отправить несколько запросов за один round-trip.

```go
batch := &pgx.Batch{}
batch.Queue("INSERT INTO orders (user_id, total) VALUES ($1, $2)", userID, total)
batch.Queue("UPDATE users SET order_count = order_count + 1 WHERE id = $1", userID)
batch.Queue("SELECT balance FROM accounts WHERE user_id = $1", userID)

results := pool.SendBatch(ctx, batch)
defer results.Close()

// прочитать результаты в порядке добавления
_, err := results.Exec()           // INSERT
if err != nil { return err }

_, err = results.Exec()            // UPDATE
if err != nil { return err }

var balance int64
err = results.QueryRow().Scan(&balance)  // SELECT
if err != nil { return err }
```

Batch внутри транзакции:

```go
tx, _ := pool.Begin(ctx)
defer tx.Rollback(ctx)

batch := &pgx.Batch{}
for _, item := range items {
    batch.Queue("INSERT INTO ...", item.Field1, item.Field2)
}

results := tx.SendBatch(ctx, batch)
for range items {
    if _, err := results.Exec(); err != nil {
        results.Close()
        return err
    }
}
results.Close()
return tx.Commit(ctx)
```

---

## COPY protocol: bulk insert

COPY — самый быстрый способ вставить большое количество строк (в 5–50x быстрее batch INSERT).

```go
import "github.com/jackc/pgx/v5/pgxpool"

func BulkInsert(ctx context.Context, pool *pgxpool.Pool, users []User) error {
    rows := make([][]any, len(users))
    for i, u := range users {
        rows[i] = []any{u.Email, u.Status, u.CreatedAt}
    }

    _, err := pool.CopyFrom(ctx,
        pgx.Identifier{"users"},
        []string{"email", "status", "created_at"},
        pgx.CopyFromRows(rows),
    )
    return err
}
```

COPY с кастомным источником данных (streaming):

```go
type userSource struct {
    users []User
    idx   int
}

func (s *userSource) Next() bool { s.idx++; return s.idx <= len(s.users) }
func (s *userSource) Values() ([]any, error) {
    u := s.users[s.idx-1]
    return []any{u.Email, u.Status, u.CreatedAt}, nil
}
func (s *userSource) Err() error { return nil }

_, err := pool.CopyFrom(ctx,
    pgx.Identifier{"users"},
    []string{"email", "status", "created_at"},
    &userSource{users: users},
)
```

---

## Обработка ошибок PostgreSQL

`pgconn.PgError` содержит структурированную информацию об ошибке PostgreSQL:

```go
import "github.com/jackc/pgx/v5/pgconn"

func handlePgError(err error) error {
    var pgErr *pgconn.PgError
    if !errors.As(err, &pgErr) {
        return err
    }

    switch pgErr.Code {
    case "23505": // unique_violation
        return ErrAlreadyExists
    case "23503": // foreign_key_violation
        return ErrInvalidReference
    case "23502": // not_null_violation
        return ErrMissingField
    case "40001": // serialization_failure
        return ErrSerializationFailure
    case "40P01": // deadlock_detected
        return ErrDeadlock
    case "42P01": // undefined_table
        return ErrSchemaError
    default:
        return fmt.Errorf("db error %s: %w", pgErr.Code, err)
    }
}
```

Полный список кодов: [PostgreSQL Error Codes](https://www.postgresql.org/docs/current/errcodes-appendix.html).

Ограничение уникальности — определить какое именно:

```go
var pgErr *pgconn.PgError
if errors.As(err, &pgErr) && pgErr.Code == "23505" {
    // pgErr.ConstraintName содержит имя constraint
    switch pgErr.ConstraintName {
    case "users_email_key":
        return ErrEmailTaken
    case "users_phone_key":
        return ErrPhoneTaken
    }
}
```

---

## Context и timeout

Всегда передавать context — он отменяет запрос при deadline/cancellation:

```go
// timeout на запрос
ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
defer cancel()

rows, err := pool.Query(ctx, "SELECT * FROM large_table WHERE ...")
```

При отмене контекста pgx отправляет cancel request к PostgreSQL — запрос завершается на сервере.

Отдельные timeout'ы через `lock_timeout` / `statement_timeout`:

```go
// statement timeout для конкретного запроса
const query = `
    SET LOCAL statement_timeout = '3000';  -- 3 seconds
    SELECT * FROM orders WHERE user_id = $1;
`
// нельзя в пуле без транзакции, т.к. SET LOCAL требует транзакцию
tx, _ := pool.Begin(ctx)
defer tx.Rollback(ctx)
tx.Exec(ctx, "SET LOCAL statement_timeout = '3000'")
rows, err := tx.Query(ctx, "SELECT * FROM orders WHERE user_id = $1", userID)
```

Или через DSN:

```
postgres://user:pass@host/db?statement_timeout=5000
```

---

## pgtype: нативные типы PostgreSQL

pgx поддерживает нативные типы PG без промежуточной сериализации:

```go
import "github.com/jackc/pgx/v5/pgtype"

// UUID
var id pgtype.UUID
err := pool.QueryRow(ctx, "SELECT id FROM users WHERE email = $1", email).Scan(&id)
// или стандартный uuid.UUID из github.com/google/uuid

// Nullable поля
var deletedAt pgtype.Timestamptz
if deletedAt.Valid {
    // поле заполнено
}

// Массивы
var roles pgtype.Array[string]
err := pool.QueryRow(ctx, "SELECT roles FROM users WHERE id = $1", id).Scan(&roles)

// JSONB
type Meta struct {
    Color string `json:"color"`
}
var meta Meta
err = pool.QueryRow(ctx, "SELECT metadata FROM products WHERE id = $1", id).
    Scan(&meta)  // pgx автоматически unmarshals JSONB в struct
```

Вставка JSONB:

```go
meta := Meta{Color: "red"}
_, err = pool.Exec(ctx,
    "INSERT INTO products (name, metadata) VALUES ($1, $2)",
    "product", meta,  // pgx автоматически marshals struct в JSONB
)
```

---

## Партиционированные таблицы из Go

Со стороны приложения партиционированная таблица — это **обычная таблица**: `INSERT`/`SELECT` идут в родителя, а PostgreSQL сам маршрутизирует строку в нужную партицию и отсекает лишние при чтении (partition pruning). Отдельного API в pgx для этого не нужно. Что действительно делают из Go — это **обслуживание партиций по расписанию**: заранее создать будущую, отцепить и удалить старую (retention). Механику самого партиционирования см. в [05-partitioning.md](./05-partitioning.md).

### Вставка: маршрутизация прозрачна

```go
// INSERT в РОДИТЕЛЯ — PostgreSQL сам положит строку в orders_2024_06 по created_at
_, err := pool.Exec(ctx,
    "INSERT INTO orders (user_id, total, created_at) VALUES ($1, $2, $3)",
    userID, total, createdAt,
)
```

Подвох: если строка не попадает ни в одну партицию и нет `DEFAULT`-партиции — будет ошибка `23514 check_violation` («no partition of relation found for row»). Стоит обработать её явно (обычно = «забыли создать партицию на новый период»):

```go
var pgErr *pgconn.PgError
if errors.As(err, &pgErr) && pgErr.Code == "23514" {
    return fmt.Errorf("нет партиции под дату %s: %w", createdAt, err)
}
```

### COPY в партиционированную таблицу

`CopyFrom` в родителя работает — маршрутизация по партициям выполняется на сервере:

```go
_, err := pool.CopyFrom(ctx,
    pgx.Identifier{"orders"},                       // родитель, не конкретная партиция
    []string{"user_id", "total", "created_at"},
    pgx.CopyFromRows(rows),
)
```

Для максимальной скорости массовой загрузки за один период грузят **прямо в партицию** (`pgx.Identifier{"orders_2024_06"}`) — меньше накладных расходов на routing.

### Создание будущих партиций по расписанию

Имена партиций — это идентификаторы, их нельзя передать как `$1`-параметр, поэтому DDL собирается строкой. Значения границ при этом **обязательно** экранировать через `pgx.Identifier.Sanitize()` для имени и форматировать даты безопасно:

```go
// ensureMonthlyPartition создаёт партицию на месяц вперёд, если её ещё нет.
// Запускать кроном (например, раз в день): идемпотентно благодаря IF NOT EXISTS.
func ensureMonthlyPartition(ctx context.Context, pool *pgxpool.Pool, month time.Time) error {
    start := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, time.UTC)
    end := start.AddDate(0, 1, 0)
    name := fmt.Sprintf("orders_%04d_%02d", start.Year(), start.Month())

    // имя таблицы — идентификатор, экранируем; границы — литералы дат (формат фиксирован, не из пользователя)
    sql := fmt.Sprintf(
        `CREATE TABLE IF NOT EXISTS %s PARTITION OF orders
             FOR VALUES FROM ('%s') TO ('%s')`,
        pgx.Identifier{name}.Sanitize(),
        start.Format("2006-01-02"),
        end.Format("2006-01-02"),
    )
    _, err := pool.Exec(ctx, sql)
    return err
}
```

### Retention: отцепить и удалить старые партиции

```go
// dropOldPartition архивирует и удаляет старую партицию.
// DETACH CONCURRENTLY нельзя внутри транзакции — выполняем отдельным Exec.
func dropOldPartition(ctx context.Context, pool *pgxpool.Pool, name, archiveDir string) error {
    ident := pgx.Identifier{name}.Sanitize()

    // 1. отцепить без долгого лока (партиция станет обычной таблицей)
    if _, err := pool.Exec(ctx,
        fmt.Sprintf("ALTER TABLE orders DETACH PARTITION %s CONCURRENTLY", ident),
    ); err != nil {
        return fmt.Errorf("detach %s: %w", name, err)
    }

    // 2. выгрузить в архив: COPY ... TO STDOUT идёт в io.Writer (файл) через
    //    нижележащее соединение — высокий уровень pgxpool такого метода не даёт
    f, err := os.Create(filepath.Join(archiveDir, name+".csv"))
    if err != nil {
        return fmt.Errorf("create archive %s: %w", name, err)
    }
    defer f.Close()

    conn, err := pool.Acquire(ctx)
    if err != nil {
        return err
    }
    _, err = conn.Conn().PgConn().CopyTo(ctx, f,
        fmt.Sprintf("COPY %s TO STDOUT (FORMAT csv, HEADER)", ident))
    conn.Release()
    if err != nil {
        return fmt.Errorf("copy out %s: %w", name, err)
    }
    if err := f.Sync(); err != nil { // дождаться записи на диск до DROP
        return fmt.Errorf("flush archive %s: %w", name, err)
    }

    // 3. теперь данные сохранены — удаляем таблицу
    if _, err := pool.Exec(ctx, fmt.Sprintf("DROP TABLE %s", ident)); err != nil {
        return fmt.Errorf("drop %s: %w", name, err)
    }
    return nil
}
```

### Запрос: pruning включает ключ партиционирования

```go
// фильтр по created_at (ключ партиционирования) → план затронет только нужные партиции
rows, err := pool.Query(ctx,
    `SELECT id, user_id, total FROM orders
     WHERE created_at >= $1 AND created_at < $2`,
    monthStart, monthEnd,
)
// без условия по created_at PostgreSQL просканирует ВСЕ партиции — проверять EXPLAIN
```

> Перечисление партиций для обслуживания (что создавать/дропать) удобно тянуть из каталога: `SELECT inhrelid::regclass FROM pg_inherits WHERE inhparent = 'orders'::regclass;`

---

## Interview-ready answer

**1. Почему pgx, а не database/sql?**

- pgx работает с нативным протоколом PostgreSQL напрямую: быстрее, поддерживает родные типы (JSONB, массивы, UUID, `numeric`) без прослойки `driver.Value`, имеет свой пул `pgxpool`; `database/sql` остаётся для совместимости с общим интерфейсом.

**2. Какие обязательные практики при чтении строк?**

- Всегда `defer rows.Close()` (иначе соединение не вернётся в пул), проверять `rows.Err()` после цикла `rows.Next()`, передавать `context.Context` для отмены по timeout; удобнее — `pgx.CollectRows` + `RowToStructByName`, который закрывает rows сам.

**3. Как правильно оформить транзакцию?**

- `tx, _ := pool.Begin(ctx)`, сразу `defer tx.Rollback(ctx)`, в конце `tx.Commit(ctx)` — Rollback после успешного Commit безопасен (no-op), поэтому defer не вредит; удобно обернуть в хелпер `WithTx(ctx, pool, func(tx) error {...})`.

**4. Чем ускорить много операций?**

- `pgx.Batch` + `SendBatch` — несколько запросов за один round-trip (меньше сетевых задержек); `pool.CopyFrom` (COPY-протокол) — bulk insert в 5–50× быстрее, чем построчный INSERT.

**5. Как разбирать ошибки PostgreSQL?**

- Через `errors.As` в `*pgconn.PgError` и `pgErr.Code`: `23505` unique violation (плюс `ConstraintName`, чтобы понять какой), `23503` FK, `40001` serialization failure, `40P01` deadlock; на `40001`/`40P01` нужен retry.

**6. Что с JSONB и нативными типами?**

- pgx сам marshals/unmarshals Go-struct в JSONB и обратно (`Scan(&meta)` / передача struct в `Exec`); для nullable — `pgtype.Timestamptz{Valid:bool}`, для массивов — `pgtype.Array[T]`.

**7. Как работать через PgBouncer в transaction mode?**

- Именованные prepared statements не живут между транзакциями, поэтому `cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeCacheDescribe` (или `SimpleProtocol`); см. [09-connection-pooling.md](./09-connection-pooling.md).

**8. Как из Go обслуживать партиционированные таблицы?**

- `INSERT`/`SELECT`/`CopyFrom` идут в родителя — маршрутизация и pruning на сервере; из приложения по расписанию создают будущие партиции (`CREATE TABLE IF NOT EXISTS ... PARTITION OF`, имя через `pgx.Identifier.Sanitize()`) и снимают старые (`DETACH PARTITION ... CONCURRENTLY` + `DROP`, вне транзакции).
