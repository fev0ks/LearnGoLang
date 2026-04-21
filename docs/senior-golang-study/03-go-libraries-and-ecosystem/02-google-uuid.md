# google/uuid

`github.com/google/uuid` — стандартная библиотека для UUID в Go экосистеме.

## Основные операции

```go
import "github.com/google/uuid"

// Генерация
id := uuid.New()               // UUID v4 (random), panic при ошибке
id, err := uuid.NewRandom()    // UUID v4, явная ошибка
id, err = uuid.NewV7()         // UUID v7 (time-ordered, Go 1.21+)

// Строковое представление
fmt.Println(id.String())       // "550e8400-e29b-41d4-a716-446655440000"
fmt.Println(id)                // то же — UUID реализует fmt.Stringer

// Парсинг
id, err = uuid.Parse("550e8400-e29b-41d4-a716-446655440000")
id, err = uuid.ParseBytes(b)

// Тип — [16]byte
var id uuid.UUID               // zero value = nil UUID
id == uuid.Nil                 // проверка на nil (00000000-...)

// Сравнение — прямое через ==
id1 == id2
```

## UUID v4 vs UUID v7

| | UUID v4 | UUID v7 |
|---|---|---|
| Содержимое | полностью random | Unix timestamp (ms) + random |
| Сортировка | случайная | хронологическая |
| B-tree индекс | фрагментация | sequential insert |
| Range queries | неэффективны | эффективны |
| Утечка времени | нет | timestamp виден в UUID |
| Использование | general purpose | primary keys, event IDs |

**UUID v7 предпочтительнее для primary keys** — sequential insert уменьшает фрагментацию B-tree индекса в PostgreSQL, что улучшает производительность при высокой нагрузке.

```go
// v7: первые 48 бит — миллисекунды Unix timestamp
// 550e8400-...  ← v4: random
// 01947c3f-...  ← v7: начинается с timestamp
```

## PostgreSQL

Использовать тип `UUID`, не `VARCHAR(36)`:
- 16 байт vs 36 байт
- быстрее сравнение на уровне БД
- нативная поддержка в pgx

```go
// pgx v5 — сканирует UUID напрямую
var id uuid.UUID
err := row.Scan(&id)

// INSERT
newID := uuid.New()
_, err = pool.Exec(ctx,
    "INSERT INTO users (id, email) VALUES ($1, $2)",
    newID, email,
)

// pgtype.UUID — если нужно различать NULL
import "github.com/jackc/pgx/v5/pgtype"
var id pgtype.UUID
row.Scan(&id)
// id.Bytes — [16]byte, id.Valid — bool
```

## Схема PostgreSQL

```sql
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),  -- v4 на уровне БД
    email TEXT NOT NULL UNIQUE
);

-- Или генерировать v7 в Go и вставлять явно
CREATE TABLE events (
    id UUID PRIMARY KEY,  -- v7 из приложения
    payload JSONB
);
```

## JSON

```go
type User struct {
    ID    uuid.UUID `json:"id"`
    Email string    `json:"email"`
}

// JSON: {"id": "550e8400-e29b-41d4-a716-446655440000", "email": "..."}
// uuid.UUID реализует json.Marshaler — сериализуется как строка
```

## Типичные ошибки

```go
// uuid.New() паникует при ошибке чтения из rand
// В тестах и критичном коде лучше использовать NewRandom()
id, err := uuid.NewRandom()
if err != nil {
    return fmt.Errorf("generate uuid: %w", err)
}

// Не хранить UUID как string в Go-коде
type User struct {
    ID string  // плохо — теряем тип, нет валидации
}
type User struct {
    ID uuid.UUID  // хорошо
}
```

## Interview-ready answer

`google/uuid` — стандарт для UUID в Go. UUID v4 — полностью random, UUID v7 содержит Unix timestamp в первых битах, что даёт хронологическую сортировку. Для primary keys в PostgreSQL v7 лучше: sequential insert снижает фрагментацию B-tree индекса. Тип хранить в PostgreSQL как `UUID`, не `VARCHAR`. pgx v5 поддерживает нативный скан в `uuid.UUID`.
