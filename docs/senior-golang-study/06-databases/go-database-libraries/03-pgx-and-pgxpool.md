# pgx And pgxpool

`pgx` — PostgreSQL driver и toolkit для Go. Не просто driver для `database/sql`, а полноценный клиент с PostgreSQL-specific возможностями.

## Содержание

- [pgx vs pgxpool](#pgx-vs-pgxpool)
- [Подключение и pool config](#подключение-и-pool-config)
- [Основные операции](#основные-операции)
- [Обработка ошибок — pgconn.PgError](#обработка-ошибок--pgconnpgerror)
- [Batch queries](#batch-queries)
- [CopyFrom — bulk insert](#copyfrom--bulk-insert)
- [Кастомные типы — pgtype](#кастомные-типы--pgtype)
- [Pool metrics](#pool-metrics)
- [pgx как driver для database/sql](#pgx-как-driver-для-databasesql)
- [Interview-ready answer](#interview-ready-answer)

## pgx vs pgxpool

| | pgx.Conn | pgxpool.Pool |
|---|---|---|
| Соединения | одно | пул |
| Concurrency | не thread-safe | thread-safe |
| Lifecycle | вручную | автоматически |
| Использование | тесты, одноразовые задачи | production сервисы |

Для production: всегда `pgxpool`.

## Подключение и pool config

```go
import (
    "github.com/jackc/pgx/v5/pgxpool"
)

func NewPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
    config, err := pgxpool.ParseConfig(dsn)
    if err != nil {
        return nil, fmt.Errorf("parse config: %w", err)
    }

    // Pool settings
    config.MaxConns = 25                           // максимум соединений
    config.MinConns = 5                            // держать минимум open
    config.MaxConnLifetime = 5 * time.Minute       // пересоздавать через N
    config.MaxConnIdleTime = 2 * time.Minute       // закрывать idle через N
    config.HealthCheckPeriod = 1 * time.Minute     // проверять живые соединения

    // Хук после установки соединения (типы, search_path и т.д.)
    config.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
        // Здесь можно регистрировать кастомные типы
        return nil
    }

    pool, err := pgxpool.NewWithConfig(ctx, config)
    if err != nil {
        return nil, fmt.Errorf("create pool: %w", err)
    }

    if err := pool.Ping(ctx); err != nil {
        return nil, fmt.Errorf("ping: %w", err)
    }

    return pool, nil
}
```

## Основные операции

```go
type UserRepository struct {
    pool *pgxpool.Pool
}

// QueryRow — одна строка
func (r *UserRepository) GetByID(ctx context.Context, id int64) (User, error) {
    var u User
    err := r.pool.QueryRow(ctx, `
        SELECT id, email, created_at
        FROM users WHERE id = $1
    `, id).Scan(&u.ID, &u.Email, &u.CreatedAt)
    if errors.Is(err, pgx.ErrNoRows) {
        return User{}, ErrNotFound
    }
    return u, err
}

// Query — несколько строк; pgx.CollectRows упрощает loop
func (r *UserRepository) ListActive(ctx context.Context) ([]User, error) {
    rows, err := r.pool.Query(ctx, `
        SELECT id, email FROM users WHERE active = true
    `)
    if err != nil {
        return nil, err
    }

    return pgx.CollectRows(rows, pgx.RowToStructByName[User])
}

// Exec — INSERT/UPDATE/DELETE
func (r *UserRepository) Deactivate(ctx context.Context, id int64) error {
    tag, err := r.pool.Exec(ctx, `
        UPDATE users SET active = false WHERE id = $1
    `, id)
    if err != nil {
        return err
    }
    if tag.RowsAffected() == 0 {
        return ErrNotFound
    }
    return nil
}

// Транзакция
func (r *UserRepository) CreateWithProfile(ctx context.Context, email string) error {
    return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
        var userID int64
        err := tx.QueryRow(ctx, `
            INSERT INTO users (email) VALUES ($1) RETURNING id
        `, email).Scan(&userID)
        if err != nil {
            return err
        }
        _, err = tx.Exec(ctx, `
            INSERT INTO profiles (user_id) VALUES ($1)
        `, userID)
        return err
        // Rollback автоматически если error; Commit если nil
    })
}
```

## Обработка ошибок — pgconn.PgError

`pgx` оборачивает PostgreSQL ошибки в `*pgconn.PgError`, которая содержит SQLSTATE-код.

```go
import "github.com/jackc/pgx/v5/pgconn"

// PostgreSQL error codes (SQLSTATE)
const (
    PgErrUniqueViolation     = "23505"
    PgErrForeignKeyViolation = "23503"
    PgErrNotNullViolation    = "23502"
    PgErrCheckViolation      = "23514"
    PgErrSerializationFailure = "40001"
    PgErrDeadlockDetected    = "40P01"
)

func handleDBError(err error) error {
    var pgErr *pgconn.PgError
    if !errors.As(err, &pgErr) {
        return err
    }

    switch pgErr.Code {
    case PgErrUniqueViolation:
        // pgErr.ConstraintName — имя нарушенного constraint
        return fmt.Errorf("already exists (constraint: %s): %w", pgErr.ConstraintName, ErrConflict)
    case PgErrForeignKeyViolation:
        return fmt.Errorf("referenced record not found: %w", ErrNotFound)
    case PgErrSerializationFailure:
        return fmt.Errorf("serialization failure, retry: %w", ErrRetryable)
    default:
        return fmt.Errorf("database error [%s]: %w", pgErr.Code, err)
    }
}
```

## Batch queries

Batch позволяет отправить несколько запросов в одном round-trip.

```go
func (r *UserRepository) GetMultiple(ctx context.Context, ids []int64) ([]User, error) {
    batch := &pgx.Batch{}
    for _, id := range ids {
        batch.Queue(`SELECT id, email FROM users WHERE id = $1`, id)
    }

    results := r.pool.SendBatch(ctx, batch)
    defer results.Close()

    users := make([]User, 0, len(ids))
    for range ids {
        var u User
        if err := results.QueryRow().Scan(&u.ID, &u.Email); err != nil {
            if errors.Is(err, pgx.ErrNoRows) {
                continue
            }
            return nil, err
        }
        users = append(users, u)
    }

    return users, results.Close()
}
```

**Когда полезен Batch:**
- N отдельных запросов по одному → N round-trips
- Один Batch → 1 round-trip, результаты обрабатываются последовательно
- Хорошо подходит для получения N независимых записей

## CopyFrom — bulk insert

Для массовой вставки данных — использует PostgreSQL COPY protocol, значительно быстрее INSERT.

```go
func (r *UserRepository) BulkCreate(ctx context.Context, users []User) error {
    rows := make([][]any, len(users))
    for i, u := range users {
        rows[i] = []any{u.Email, u.CreatedAt}
    }

    _, err := r.pool.CopyFrom(
        ctx,
        pgx.Identifier{"users"},           // таблица
        []string{"email", "created_at"},   // колонки
        pgx.CopyFromRows(rows),
    )
    return err
}
```

## Кастомные типы — pgtype

`pgtype` — пакет в составе `pgx` для работы с PostgreSQL-специфичными типами.

```go
import "github.com/jackc/pgx/v5/pgtype"

// UUID
var id pgtype.UUID
rows.Scan(&id)
// id.Bytes — [16]byte, id.Valid — bool

// Timestamptz
var ts pgtype.Timestamptz
rows.Scan(&ts)
// ts.Time — time.Time

// JSONB
type Metadata struct {
    Tags []string `json:"tags"`
}

var meta pgtype.Text  // JSONB сканируется как text, затем json.Unmarshal

// Числовой тип numeric/decimal
var price pgtype.Numeric
rows.Scan(&price)
```

Для `uuid` из `github.com/google/uuid` можно сканировать напрямую — pgx поддерживает это через `pgtype.UUID`:

```go
import "github.com/google/uuid"

var id uuid.UUID
rows.Scan(&id)  // работает с pgx v5
```

## Pool metrics

```go
stat := pool.Stat()

// Метрики для Prometheus/логирования
fmt.Printf("total=%d idle=%d acquired=%d\n",
    stat.TotalConns(),     // всего соединений
    stat.IdleConns(),      // idle прямо сейчас
    stat.AcquiredConns(),  // активно используемых
)

// Экспорт в Prometheus
func recordPoolMetrics(pool *pgxpool.Pool, gauge *prometheus.GaugeVec) {
    stat := pool.Stat()
    gauge.WithLabelValues("total").Set(float64(stat.TotalConns()))
    gauge.WithLabelValues("idle").Set(float64(stat.IdleConns()))
    gauge.WithLabelValues("acquired").Set(float64(stat.AcquiredConns()))
}
```

## pgx как driver для database/sql

Если нужна совместимость с библиотеками, ожидающими `*sql.DB`:

```go
import (
    "database/sql"
    "github.com/jackc/pgx/v5/stdlib"
)

// Через pgxpool
config, _ := pgxpool.ParseConfig(dsn)
db := stdlib.OpenDBFromPool(pool)

// Или через pgx.ConnConfig
connConfig, _ := pgx.ParseConfig(dsn)
db = stdlib.OpenDB(*connConfig)
```

**Когда нужна совместимость:**
- `sqlx` поверх pgx
- `sqlc` с pgx-compatible `*sql.DB`
- `bun` или другие query builders поверх pgx

## Interview-ready answer

`pgxpool` — стандартный выбор для Go + PostgreSQL production сервисов. В отличие от `database/sql`, он предоставляет нативный PostgreSQL API: типизированные ошибки через `pgconn.PgError` с SQLSTATE-кодами, batch запросы за один round-trip, bulk insert через COPY protocol, `pgx.CollectRows` для удобного маппинга, `pgx.BeginFunc` для автоматического commit/rollback. Для мониторинга — `pool.Stat()`. Минус — жёсткая привязка к PostgreSQL. Если нужна совместимость с `database/sql` ecosystem, используют `stdlib.OpenDBFromPool`.
