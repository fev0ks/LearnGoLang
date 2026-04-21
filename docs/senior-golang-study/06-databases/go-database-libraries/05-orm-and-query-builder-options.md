# ORM And Query Builder Options

В Go часто спорят про ORM. Важно не занимать религиозную позицию, а понимать trade-offs.

## Содержание

- [Squirrel — query builder](#squirrel--query-builder)
- [GORM](#gorm)
- [Ent](#ent)
- [Bun](#bun)
- [Главный риск ORM](#главный-риск-orm)
- [Interview-ready answer](#interview-ready-answer)

---

## Squirrel — query builder

`github.com/Masterminds/squirrel` — библиотека для построения SQL-запросов программно. Не ORM, не кодогенератор — просто типобезопасный builder.

**Главная ценность:** dynamic queries, где WHERE-условия собираются в runtime в зависимости от входных параметров.

### Базовый пример

```go
import sq "github.com/Masterminds/squirrel"

// Squirrel с PostgreSQL: используем sq.Dollar (нумерует $1, $2, ...)
psql := sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

// Простой SELECT
query, args, err := psql.
    Select("id", "email", "created_at").
    From("users").
    Where(sq.Eq{"active": true}).
    OrderBy("created_at DESC").
    Limit(10).
    ToSql()
// → SELECT id, email, created_at FROM users WHERE active = $1 ORDER BY created_at DESC LIMIT 10
// args: [true]
```

### Dynamic WHERE — главный use case

```go
type UserFilter struct {
    Email  string
    Active *bool
    FromID int64
}

func buildUserQuery(f UserFilter) (string, []any, error) {
    psql := sq.StatementBuilder.PlaceholderFormat(sq.Dollar)
    b := psql.Select("id", "email").From("users")

    if f.Email != "" {
        b = b.Where(sq.Like{"email": "%" + f.Email + "%"})
    }
    if f.Active != nil {
        b = b.Where(sq.Eq{"active": *f.Active})
    }
    if f.FromID > 0 {
        b = b.Where(sq.Gt{"id": f.FromID})
    }

    return b.OrderBy("id ASC").ToSql()
}
```

Без squirrel это — ручная конкатенация строк и отслеживание индексов `$1, $2, ...`:

```go
// Без squirrel — ошибкоёмко
where := "WHERE 1=1"
args := []any{}
i := 1
if f.Email != "" {
    where += fmt.Sprintf(" AND email LIKE $%d", i)
    args = append(args, "%"+f.Email+"%")
    i++
}
// и т.д.
```

### INSERT, UPDATE

```go
// INSERT
sql, args, err := psql.
    Insert("users").
    Columns("email", "name").
    Values("a@b.com", "Alice").
    Suffix("RETURNING id").
    ToSql()

// UPDATE
sql, args, err = psql.
    Update("users").
    Set("email", "new@b.com").
    Set("updated_at", time.Now()).
    Where(sq.Eq{"id": 42}).
    ToSql()
```

### Интеграция с pgxpool

```go
query, args, err := psql.Select("id", "email").From("users").
    Where(sq.Eq{"active": true}).ToSql()
if err != nil {
    return nil, err
}

rows, err := pool.Query(ctx, query, args...)
```

### Squirrel и sqlc — взаимодополняющие подходы

- `sqlc` — для статических, хорошо известных queries (CRUD, стандартные выборки)
- `squirrel` — для dynamic queries (фильтры, поиск, pagination)

### Когда выбирать squirrel

- Нужен filtering API с 5+ опциональных параметров
- Нельзя заранее знать, какие WHERE-условия будут применены
- Хочется избежать manual string concatenation и ошибок с нумерацией `$N`
- Не нужен ORM, но нужен удобный builder

---

## GORM

`GORM` — популярный ORM для Go.

### Что дает

- model mapping через struct теги
- associations (HasMany, BelongsTo, ManyToMany)
- hooks (BeforeCreate, AfterUpdate)
- транзакции
- eager loading через `Preload`
- AutoMigrate
- CRUD API

### Основные паттерны

```go
// Определение модели
type User struct {
    gorm.Model        // ID, CreatedAt, UpdatedAt, DeletedAt (soft delete)
    Email   string    `gorm:"uniqueIndex;not null"`
    Orders  []Order   `gorm:"foreignKey:UserID"`
}

// Получение с association
var user User
db.Preload("Orders").First(&user, id)

// Создание
db.Create(&User{Email: "a@b.com"})

// Обновление
db.Model(&user).Update("email", "new@b.com")

// Удаление (soft delete если есть DeletedAt)
db.Delete(&user)

// Raw SQL — когда GORM API не хватает
var result []map[string]any
db.Raw("SELECT ... complex query").Scan(&result)
```

### N+1 — главная ловушка GORM

```go
// N+1 баг:
var users []User
db.Find(&users)  // 1 запрос
for _, u := range users {
    db.Find(&u.Orders, "user_id = ?", u.ID)  // N запросов!
}

// Правильно — Preload
var users []User
db.Preload("Orders").Find(&users)
// → 2 запроса: SELECT users; SELECT orders WHERE user_id IN (1,2,3,...)

// Preload с условием
db.Preload("Orders", "status = ?", "active").Find(&users)

// Joins для фильтрации, не для загрузки
db.Joins("JOIN orders ON orders.user_id = users.id").
    Where("orders.status = ?", "pending").
    Find(&users)
```

### Плюсы GORM

- быстро стартовать
- удобно для CRUD-heavy приложений
- много готовых возможностей
- низкий порог входа

### Минусы GORM

- много магии, сложнее контролировать SQL
- лёгко получить N+1 если не смотреть generated SQL
- GORM сам решает, какие поля обновлять — `Updates` vs `Save` поведение неочевидно
- soft delete через `DeletedAt` — нужно учитывать везде

### Когда уместен

- internal admin
- CRUD-heavy сервис
- быстрый MVP
- команда понимает ORM trade-offs и смотрит generated SQL

---

## Ent

`Ent` — entity framework для Go с schema-first и code generation.

```go
// Schema определяется в Go-коде
func (User) Fields() []ent.Field {
    return []ent.Field{
        field.Int64("id"),
        field.String("email").Unique(),
        field.Time("created_at").Default(time.Now),
    }
}

func (User) Edges() []ent.Edge {
    return []ent.Edge{
        edge.Has("orders", Order.Type),
    }
}

// Сгенерированный typed API
user, err := client.User.
    Query().
    Where(user.Email("a@b.com")).
    Only(ctx)

users, err := client.User.
    Query().
    WithOrders().
    Limit(10).
    All(ctx)
```

### Плюсы

- сильная type-safety
- понятная schema-as-code модель
- compile-time проверка запросов

### Минусы

- framework lock-in
- code generation step
- raw SQL path менее прямой
- нужно учить Ent conventions

### Когда уместен

- сложная domain модель с множеством relationships
- хочется typed API поверх DB
- команда готова к framework lock-in

---

## Bun

`Bun` — SQL-first ORM/query builder с поддержкой PostgreSQL, MySQL, SQLite.

```go
type User struct {
    bun.BaseModel `bun:"table:users"`

    ID    int64  `bun:"id,pk,autoincrement"`
    Email string `bun:"email,notnull,unique"`
}

// SELECT
var users []User
err := db.NewSelect().Model(&users).
    Where("active = ?", true).
    OrderExpr("created_at DESC").
    Scan(ctx)

// INSERT
_, err = db.NewInsert().Model(&user).Exec(ctx)

// Raw SQL
var ids []int64
err = db.NewRaw("SELECT id FROM users WHERE active = ?", true).
    Scan(ctx, &ids)
```

### Плюсы

- ближе к SQL, чем классический ORM
- меньше magic, чем GORM
- query builder + struct mapping

### Минусы

- ещё один abstraction layer
- надо понимать generated SQL
- не заменяет знание индексов и query plans

### Когда уместен

- нужен query builder + mapping
- не хочется полностью уходить от SQL
- нужна поддержка нескольких БД

---

## Главный риск ORM

ORM удобен, пока:
- запросы простые
- нагрузка умеренная
- команда смотрит, какой SQL генерируется

ORM начинает вредить, когда:
- никто не понимает generated SQL
- N+1 проходит в production незамеченным
- сложные joins превращаются в нечитаемый builder-код
- query plan никто не смотрит

**Правило:** включи query logging в ORM с первого дня. Если SQL выглядит неожиданно — это сигнал.

```go
// GORM: включить logging
db, _ := gorm.Open(postgres.Open(dsn), &gorm.Config{
    Logger: logger.Default.LogMode(logger.Info),
})

// Посмотреть один запрос
db.Debug().Find(&users)
```

## Interview-ready answer

ORM не плох сам по себе. GORM ускоряет CRUD и снижает boilerplate, но добавляет abstraction cost и риск N+1. Для production важно: включить query logging, знать про `Preload` vs `Joins`, понимать когда GORM обновляет все поля, а когда только изменённые. Squirrel решает конкретную задачу — dynamic WHERE без ручной конкатенации строк — и хорошо дополняет sqlc. Ent и Bun — хорошие альтернативы когда нужен typed query API без полного GORM-style magic.
