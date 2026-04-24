# GraphQL

GraphQL — язык запросов к API от Facebook (2015). Клиент сам определяет форму ответа. Решает проблему over-fetching и under-fetching REST, но создаёт новые сложности.

---

## Основные концепции

### Schema, Query, Mutation, Subscription

```graphql
# Schema — типы данных и операции
type User {
    id: ID!              # ! = non-null
    name: String!
    email: String!
    orders: [Order!]!    # список
    profile: Profile     # nullable
}

type Order {
    id: ID!
    total: Float!
    status: OrderStatus!
    items: [OrderItem!]!
}

enum OrderStatus { PENDING PROCESSING SHIPPED DELIVERED CANCELLED }

# Query — чтение данных
type Query {
    user(id: ID!): User
    users(filter: UserFilter, limit: Int, offset: Int): [User!]!
    order(id: ID!): Order
}

# Mutation — изменение данных
type Mutation {
    createUser(input: CreateUserInput!): User!
    updateUser(id: ID!, input: UpdateUserInput!): User!
    deleteUser(id: ID!): Boolean!
}

# Subscription — real-time (WebSocket)
type Subscription {
    orderStatusChanged(orderID: ID!): Order!
    newMessage(roomID: ID!): Message!
}
```

### Запрос: клиент выбирает поля

```graphql
# Запрос — только нужные поля
query GetUserWithOrders {
    user(id: "123") {
        id
        name
        orders {
            id
            status
            total
        }
        # profile НЕ запрашиваем — не вернётся
    }
}

# Ответ
{
    "data": {
        "user": {
            "id": "123",
            "name": "Alice",
            "orders": [
                {"id": "o1", "status": "SHIPPED", "total": 99.99}
            ]
        }
    }
}
```

### Fragments — переиспользование полей

```graphql
fragment UserFields on User {
    id
    name
    email
}

query {
    user(id: "1") { ...UserFields }
    users { ...UserFields }
}
```

### Variables — параметризованные запросы

```graphql
query GetUser($id: ID!, $includeOrders: Boolean = false) {
    user(id: $id) {
        id
        name
        orders @include(if: $includeOrders) {
            id
            status
        }
    }
}
```

```json
{ "id": "123", "includeOrders": true }
```

---

## N+1 проблема и DataLoader

### Проблема

```graphql
query {
    orders {          # 1 SQL запрос → возвращает 100 orders
        id
        user {        # 100 SQL запросов → один на каждого user!
            name
        }
    }
}
```

```
Total: 1 + N запросов = N+1 problem
```

### DataLoader как решение

DataLoader батчирует запросы в рамках одного GraphQL resolve цикла:

```go
// Вместо 100 запросов SELECT * FROM users WHERE id = $1
// один батч: SELECT * FROM users WHERE id IN ($1, $2, ... $100)

import "github.com/graph-gophers/dataloader/v7"

func newUserLoader(db *sql.DB) *dataloader.Loader[string, *User] {
    return dataloader.NewBatchedLoader(func(ctx context.Context, keys []string) []*dataloader.Result[*User] {
        // Один SQL запрос для всех ключей
        users, err := db.QueryUsersIn(ctx, keys)
        
        results := make([]*dataloader.Result[*User], len(keys))
        userMap := make(map[string]*User, len(users))
        for _, u := range users {
            userMap[u.ID] = u
        }
        for i, key := range keys {
            if u, ok := userMap[key]; ok {
                results[i] = &dataloader.Result[*User]{Data: u}
            } else {
                results[i] = &dataloader.Result[*User]{Error: ErrNotFound}
            }
        }
        return results
    })
}

// Resolver использует loader вместо прямого DB вызова
func (r *orderResolver) User(ctx context.Context) (*User, error) {
    return r.loaders.Users.Load(ctx, r.order.UserID)
}
```

---

## Go: gqlgen

[gqlgen](https://github.com/99designs/gqlgen) — code-first подход: schema → сгенерированный Go код.

```bash
go run github.com/99designs/gqlgen generate
```

### Структура проекта

```
graph/
  schema.graphqls    ← schema
  model/
    models_gen.go    ← сгенерированные типы
  resolver.go        ← точка входа resolvers
  schema.resolvers.go ← заглушки для реализации
gqlgen.yml           ← конфигурация генерации
```

```yaml
# gqlgen.yml
schema:
  - graph/*.graphqls
exec:
  filename: graph/generated.go
model:
  filename: graph/model/models_gen.go
resolver:
  layout: follow-schema
  dir: graph
  package: graph
```

### Resolver реализация

```go
// graph/schema.resolvers.go

func (r *queryResolver) User(ctx context.Context, id string) (*model.User, error) {
    user, err := r.db.Users.FindByID(ctx, id)
    if err != nil {
        if errors.Is(err, ErrNotFound) {
            return nil, nil // GraphQL: null value
        }
        return nil, err
    }
    return toGraphQLUser(user), nil
}

// Nested resolver (N+1 solution via DataLoader)
func (r *userResolver) Orders(ctx context.Context, obj *model.User) ([]*model.Order, error) {
    return r.loaders.OrdersByUserID.Load(ctx, obj.ID)
}
```

### HTTP handler

```go
import (
    "github.com/99designs/gqlgen/graphql/handler"
    "github.com/99designs/gqlgen/graphql/playground"
)

srv := handler.NewDefaultServer(generated.NewExecutableSchema(generated.Config{
    Resolvers: &graph.Resolver{
        DB:      db,
        Loaders: newLoaders(db),
    },
}))

mux.Handle("/graphql", srv)
mux.Handle("/playground", playground.Handler("GraphQL playground", "/graphql"))
```

---

## Introspection и persisted queries

### Introspection

GraphQL API документирует себя через introspection:

```graphql
# Узнать все типы
query {
    __schema {
        types { name kind }
    }
}

# Узнать поля типа
query {
    __type(name: "User") {
        fields { name type { name } }
    }
}
```

В production часто **отключают** introspection (security — утечка schema):

```go
srv := handler.NewDefaultServer(schema)
srv.AddTransport(transport.POST{})
// Убираем introspection в production
if os.Getenv("GRAPHQL_INTROSPECTION") != "true" {
    srv.Use(extension.DisableIntrospection{})
}
```

### Persisted queries

Клиент отправляет hash запроса вместо полного текста → меньше трафик:

```
Обычный запрос: POST /graphql {"query": "query GetUser($id: ID!) { user(id: $id) { ... } }"}
Persisted:      POST /graphql {"extensions": {"persistedQuery": {"sha256Hash": "abc123..."}}}
```

```go
srv.Use(apollotracing.Tracer{})
srv.Use(extension.FixedQueryCache(lru.New(100)))
```

---

## Когда GraphQL, когда REST

| Критерий | GraphQL | REST |
|---|---|---|
| Клиент диктует форму ответа | ✅ | ❌ (фиксированный ответ) |
| Несколько клиентов с разными нуждами | ✅ | ❌ (разные endpoints) |
| Over/under-fetching | ❌ | ✅ GraphQL решает |
| Кеширование HTTP | ❌ (все POST) | ✅ (GET с URL) |
| File upload | Сложно (multipart) | Просто |
| Real-time | Subscriptions | SSE / WebSocket |
| Простота освоения | Выше порог | Ниже |
| Introspection / документация | ✅ автоматически | OpenAPI (вручную) |
| N+1 без DataLoader | ❌ частая проблема | Контролируется |
| Mobile/web с разными экранами | ✅ отлично | Нужна доработка |

**Выбирай GraphQL когда:**
- Mobile + web + third-party с разными нуждами
- Разработчики хотят быструю итерацию без backend изменений
- Данные — граф с много связями

**Выбирай REST когда:**
- Простой CRUD API
- Важен HTTP caching (CDN)
- Небольшая команда без опыта с GraphQL
- File upload интенсивен

---

## Interview-ready answer

**Q: Что такое N+1 проблема и как DataLoader её решает?**

Если resolver для каждого Order запрашивает его User в БД отдельно — N orders = N+1 запросов (1 для списка + N для user). DataLoader буферизует запросы в рамках одного tick event loop и делает один батч-запрос: `SELECT * FROM users WHERE id IN (...)`. Ключ — resolve цикл GraphQL: DataLoader собирает все ключи за один такт, затем батчит.

**Q: Когда REST, когда GraphQL?**

REST — для простых CRUD API, где важен HTTP caching и простота. GraphQL — когда несколько типов клиентов (mobile/web) нуждаются в разных полях, когда данные — сложный граф. Главный минус GraphQL — нет HTTP GET caching, N+1 без DataLoader, более сложная операционная картина (introspection security, persisted queries для production).
