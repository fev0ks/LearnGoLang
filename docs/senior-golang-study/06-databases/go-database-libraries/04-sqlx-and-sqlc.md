# sqlx And sqlc

`sqlx` и `sqlc` — оба сохраняют SQL-first подход, но решают разные проблемы: один убирает boilerplate в runtime, другой генерирует типобезопасный код в compile time.

## Содержание

- [sqlx — runtime helper](#sqlx--runtime-helper)
- [sqlc — compile-time generator](#sqlc--compile-time-generator)
- [Главное отличие](#главное-отличие)
- [Когда что выбирать](#когда-что-выбирать)
- [Interview-ready answer](#interview-ready-answer)

---

## sqlx — runtime helper

`sqlx` — расширение над `database/sql`. Не заменяет его, а добавляет удобные методы поверх.

### Подключение

```go
import (
    "github.com/jmoiron/sqlx"
    _ "github.com/jackc/pgx/v5/stdlib"
)

db, err := sqlx.Open("pgx", dsn)
// или обернуть существующий *sql.DB
db = sqlx.NewDb(existingDB, "pgx")
```

### Select — список в slice

```go
// database/sql: ручной loop + rows.Next() + Scan
// sqlx:
var users []User
err := db.SelectContext(ctx, &users, `
    SELECT id, email, created_at
    FROM users
    WHERE active = $1
`, true)
```

Поля struct маппятся по тегу `db:"column_name"`:

```go
type User struct {
    ID        int64     `db:"id"`
    Email     string    `db:"email"`
    CreatedAt time.Time `db:"created_at"`
}
```

### Get — одна строка

```go
var user User
err := db.GetContext(ctx, &user, `
    SELECT id, email, created_at FROM users WHERE id = $1
`, id)
if errors.Is(err, sql.ErrNoRows) {
    return User{}, ErrNotFound
}
```

### NamedExec — struct как параметры

Вместо позиционных `$1, $2, $3` — именованные параметры из struct или map:

```go
type CreateUserParams struct {
    Email string `db:"email"`
    Name  string `db:"name"`
}

result, err := db.NamedExecContext(ctx, `
    INSERT INTO users (email, name)
    VALUES (:email, :name)
`, CreateUserParams{Email: "a@b.com", Name: "Alice"})

// Или через map
result, err = db.NamedExecContext(ctx, `
    UPDATE users SET name = :name WHERE id = :id
`, map[string]any{"name": "Bob", "id": 42})
```

### sqlx.In — IN-clause из slice

```go
ids := []int64{1, 2, 3}

// Генерирует: WHERE id IN ($1, $2, $3)
query, args, err := sqlx.In(`
    SELECT id, email FROM users WHERE id IN (?)
`, ids)
if err != nil {
    return nil, err
}

// Rebind меняет ? на $1, $2 для PostgreSQL
query = db.Rebind(query)

var users []User
err = db.SelectContext(ctx, &users, query, args...)
```

### Transaction с sqlx

```go
tx, err := db.BeginTxx(ctx, nil)
if err != nil {
    return err
}
defer tx.Rollback()

var id int64
err = tx.GetContext(ctx, &id, `
    INSERT INTO users (email) VALUES ($1) RETURNING id
`, email)
if err != nil {
    return err
}

_, err = tx.NamedExecContext(ctx, `
    INSERT INTO profiles (user_id, bio) VALUES (:user_id, :bio)
`, map[string]any{"user_id": id, "bio": ""})
if err != nil {
    return err
}

return tx.Commit()
```

### Ограничения sqlx

- SQL остаётся строками → ошибки в запросах только в runtime
- Маппинг тегов `db:` — runtime reflect, возможны опечатки
- Нет проверки соответствия колонок схеме БД
- `sqlx.In` работает только с `?`-плейсхолдерами, нужен `Rebind`

---

## sqlc — compile-time generator

`sqlc` принимает SQL-запросы как вход и генерирует типобезопасный Go-код. Нет SQL в runtime-строках — только сгенерированные методы.

### Структура проекта

```
db/
├── sqlc.yaml          # конфиг генератора
├── schema.sql         # миграции / CREATE TABLE
├── queries/
│   └── users.sql      # SQL с аннотациями sqlc
└── generated/
    ├── db.go          # интерфейс
    ├── models.go      # Go structs
    └── users.sql.go   # сгенерированные методы
```

### sqlc.yaml — конфиг с pgx v5

```yaml
version: "2"
sql:
  - engine: "postgresql"
    queries: "queries/"
    schema: "schema.sql"
    gen:
      go:
        package: "db"
        out: "generated"
        sql_package: "pgx/v5"          # использовать pgx, не database/sql
        emit_json_tags: true
        emit_db_tags: true
        emit_empty_slices: true
        emit_interface: true            # генерирует Querier interface
        emit_pointers_for_null_types: true
```

### queries/users.sql

```sql
-- name: GetUser :one
SELECT id, email, created_at
FROM users
WHERE id = $1;

-- name: ListActiveUsers :many
SELECT id, email, created_at
FROM users
WHERE active = true
ORDER BY created_at DESC;

-- name: CreateUser :one
INSERT INTO users (email, created_at)
VALUES ($1, NOW())
RETURNING *;

-- name: UpdateUserEmail :exec
UPDATE users
SET email = $2
WHERE id = $1;

-- name: DeleteUser :execrows
DELETE FROM users WHERE id = $1;
```

### Что генерирует sqlc

```go
// generated/models.go
type User struct {
    ID        int64
    Email     string
    CreatedAt pgtype.Timestamptz
}

// generated/users.sql.go
type Queries struct {
    db DBTX  // интерфейс: *pgx.Conn, *pgxpool.Pool или pgx.Tx
}

func New(db DBTX) *Queries {
    return &Queries{db: db}
}

func (q *Queries) GetUser(ctx context.Context, id int64) (User, error) {
    row := q.db.QueryRow(ctx, getUserSQL, id)
    var u User
    err := row.Scan(&u.ID, &u.Email, &u.CreatedAt)
    return u, err
}

func (q *Queries) ListActiveUsers(ctx context.Context) ([]User, error) {
    rows, err := q.db.Query(ctx, listActiveUsersSQL)
    // ... pgx.CollectRows внутри
}

func (q *Queries) CreateUser(ctx context.Context, email string) (User, error) {
    // ...
}
```

### Использование сгенерированного кода

```go
pool, _ := pgxpool.New(ctx, dsn)
queries := db.New(pool)

// Compile-time проверка типов
user, err := queries.GetUser(ctx, 42)
users, err := queries.ListActiveUsers(ctx)
newUser, err := queries.CreateUser(ctx, "hello@example.com")

// В транзакции
tx, _ := pool.Begin(ctx)
defer tx.Rollback(ctx)

qtx := queries.WithTx(tx)  // тот же Queries, но с tx
qtx.UpdateUserEmail(ctx, 42, "new@example.com")
tx.Commit(ctx)
```

### Тестирование с sqlc

`sqlc` генерирует `Querier` interface — легко мокировать:

```go
// generated/db.go
type Querier interface {
    GetUser(ctx context.Context, id int64) (User, error)
    ListActiveUsers(ctx context.Context) ([]User, error)
    CreateUser(ctx context.Context, email string) (User, error)
    // ...
}

// В тесте
type mockQuerier struct {
    users map[int64]User
}

func (m *mockQuerier) GetUser(_ context.Context, id int64) (User, error) {
    u, ok := m.users[id]
    if !ok {
        return User{}, pgx.ErrNoRows
    }
    return u, nil
}
```

---

## Главное отличие

| | sqlx | sqlc |
|---|---|---|
| Принцип | runtime helper | compile-time codegen |
| SQL ошибки | runtime | compile-time (при генерации) |
| Type safety | частичная (reflect) | полная |
| Гибкость | высокая (dynamic queries) | ниже (статические SQL) |
| Setup | просто (`go get`) | нужен generation step |
| Dynamic queries | легко | сложнее |
| Поддержка pgx v5 | через `stdlib.OpenDBFromPool` | нативно (`sql_package: pgx/v5`) |

## Когда что выбирать

**sqlx:**
- быстрый migration path от `database/sql`
- нужна гибкость для dynamic queries
- небольшой проект, queries простые
- не хочется generation step в CI

**sqlc:**
- SQL — контракт между командой и БД
- нужна type-safety и compile-time проверка
- команда хочет хранить SQL отдельно от Go-кода
- проект растёт и runtime SQL errors становятся дорогими
- хорошо сочетается с pgx v5 через native backend

## Interview-ready answer

`sqlx` делает `database/sql` удобнее в runtime: `SelectContext` в slice, `NamedExec` со struct-параметрами, `sqlx.In` для IN-clause — без codegen, но ошибки остаются runtime. `sqlc` генерирует типобезопасный Go-код из SQL-файлов: ошибки запросов ловятся при генерации, а не при выполнении. Оба подхода оставляют SQL читаемым и явным — в отличие от ORM. Для нового PostgreSQL-проекта `sqlc + pgx/v5` даёт лучшую type-safety и производительность.
