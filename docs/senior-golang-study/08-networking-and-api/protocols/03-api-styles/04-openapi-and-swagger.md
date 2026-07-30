# OpenAPI и Swagger

## Содержание

- [Простая аналогия](#простая-аналогия)
- [Зачем нужен OpenAPI](#зачем-нужен-openapi)
- [Структура OpenAPI документа](#структура-openapi-документа)
- [Базовый пример](#базовый-пример)
- [Schemas: переиспользуемые типы](#schemas-переиспользуемые-типы)
- [Два подхода: code-first vs spec-first](#два-подхода-code-first-vs-spec-first)
- [Spec-first: oapi-codegen в Go](#spec-first-oapi-codegen-в-go)
- [Code-first: swag из аннотаций](#code-first-swag-из-аннотаций)
- [Validation против спецификации](#validation-против-спецификации)
- [Contract testing](#contract-testing)
- [Backward compatibility](#backward-compatibility)
- [Swagger UI: документация для клиентов](#swagger-ui-документация-для-клиентов)
- [OpenAPI для AI-агентов](#openapi-для-ai-агентов)
- [Anti-patterns](#anti-patterns)
- [Interview-ready answer](#interview-ready-answer)
- [Полезные ссылки](#полезные-ссылки)

OpenAPI — это формальное описание HTTP API в YAML/JSON. Один файл, который описывает: какие endpoint'ы есть, какие у них параметры, какие body, какие коды ответа, какие схемы данных. Из этого файла можно автоматически генерировать: документацию, клиентов на любых языках, серверные stub'ы, тесты, mock-серверы.
**Swagger** — это исторически связанный набор инструментов (Swagger UI, Swagger Editor, Swagger Codegen). OpenAPI 3.0 — это **спецификация формата**, развивающаяся независимо. На практике слова "Swagger" и "OpenAPI" часто используются взаимозаменяемо.
Главная идея — single source of truth для контракта API. Backend, frontend, и mobile команды работают с одним и тем же документом, а не "что сказал Иван в Слаке".

---

## Простая аналогия

Представь меню в ресторане. У хорошего меню есть структура: блюдо, ингредиенты, цена, время приготовления, аллергены. Все официанты, повара, кассиры пользуются одним меню. Если шеф решит изменить состав — обновляет меню, и все видят то же самое.

Без меню: официант помнит "из головы" что в супе. Повар — своё. Кассир — свою цену. Клиент получает не то что заказал.

OpenAPI — это меню API. Backend, frontend, mobile, тестировщики, AI-инструменты — все смотрят в один файл. Изменилось — изменилось у всех.

---

## Зачем нужен OpenAPI

### 1. Контракт между командами

Backend и frontend часто работают параллельно. До OpenAPI это значило:
- Backend пишет код
- Backend говорит frontend в Slack "endpoint готов, POST /users, тело такое-то"
- Frontend копирует руками в свой код
- На production узнают что backend сменил имя поля и не сказал

С OpenAPI:
- Backend пишет/обновляет spec
- Spec в git, видят все
- Frontend генерирует client из spec автоматически
- Изменение поля → автогенерация → typescript ошибка компиляции у frontend

### 2. Документация без расхождений с кодом

Документация всегда устаревает в Markdown. С OpenAPI:
- Spec файл → автогенерируется Swagger UI (интерактивная документация в браузере)
- Можно попробовать запрос прямо из доки
- Документация всегда соответствует actual схеме

### 3. Кодогенерация

Из одного spec'а можно сгенерировать:
- Server stubs (Go, Python, Java, ...)
- Clients для любых языков (TypeScript, Swift, Kotlin, ...)
- Mock-серверы для тестов
- Postman collections

Это часы работы → минуты.

### 4. Contract testing

Проверка что реальный ответ сервера соответствует обещанному в spec'е. Catch'ит breaking changes автоматически.

### 5. Корм для AI-агентов

В 2026 году LLM-агенты делают tool calls. OpenAPI spec — стандартный формат, на котором AI понимает "что может делать API". Включается в Function Calling API напрямую.

---

## Структура OpenAPI документа

Документ OpenAPI 3.0 — YAML или JSON. Верхнеуровневая структура:

```yaml
openapi: 3.0.3
info:
  title: My API
  version: 1.0.0
  description: Описание моего API

servers:
  - url: https://api.example.com/v1
    description: Production
  - url: https://staging.example.com/v1
    description: Staging

paths:
  /users/{id}:
    get:
      # ... описание endpoint'а
    delete:
      # ...

components:
  schemas:
    User:
      # ... описание схемы данных
  securitySchemes:
    bearerAuth:
      # ... как авторизация

security:
  - bearerAuth: []
```

**Главные секции:**

| Секция | Что |
|---|---|
| `openapi` | Версия спецификации (3.0.x — стабильная, 3.1 — новая) |
| `info` | Metadata: название, версия, описание API |
| `servers` | URL'ы среды (prod, staging, dev) |
| `paths` | Endpoint'ы и методы |
| `components` | Переиспользуемые схемы и параметры |
| `security` | Глобальная авторизация |

---

## Базовый пример

<details>
<summary>Спецификация примера целиком: пути, параметры, ответы, схемы компонентов</summary>

```yaml
openapi: 3.0.3
info:
  title: Users API
  version: 1.0.0

paths:
  /users:
    get:
      summary: List users
      operationId: listUsers
      parameters:
        - name: limit
          in: query
          required: false
          schema:
            type: integer
            minimum: 1
            maximum: 100
            default: 20
        - name: cursor
          in: query
          schema:
            type: string
      responses:
        '200':
          description: Successful response
          content:
            application/json:
              schema:
                type: object
                properties:
                  users:
                    type: array
                    items:
                      $ref: '#/components/schemas/User'
                  next_cursor:
                    type: string
                    nullable: true
        '401':
          $ref: '#/components/responses/Unauthorized'

    post:
      summary: Create user
      operationId: createUser
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/CreateUserRequest'
      responses:
        '201':
          description: User created
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/User'
        '400':
          $ref: '#/components/responses/BadRequest'
        '409':
          description: User already exists

  /users/{id}:
    get:
      summary: Get user by ID
      operationId: getUserById
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: string
            format: uuid
      responses:
        '200':
          description: User found
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/User'
        '404':
          $ref: '#/components/responses/NotFound'

components:
  schemas:
    User:
      type: object
      required: [id, email, name]
      properties:
        id:
          type: string
          format: uuid
        email:
          type: string
          format: email
        name:
          type: string
          minLength: 1
          maxLength: 100
        created_at:
          type: string
          format: date-time

    CreateUserRequest:
      type: object
      required: [email, name]
      properties:
        email:
          type: string
          format: email
        name:
          type: string
          minLength: 1
          maxLength: 100

    Error:
      type: object
      required: [code, message]
      properties:
        code:
          type: string
        message:
          type: string

  responses:
    NotFound:
      description: Resource not found
      content:
        application/json:
          schema:
            $ref: '#/components/schemas/Error'

    BadRequest:
      description: Bad request
      content:
        application/json:
          schema:
            $ref: '#/components/schemas/Error'

    Unauthorized:
      description: Unauthorized
      content:
        application/json:
          schema:
            $ref: '#/components/schemas/Error'

  securitySchemes:
    bearerAuth:
      type: http
      scheme: bearer
      bearerFormat: JWT

security:
  - bearerAuth: []
```

</details>

---

## Schemas: переиспользуемые типы

Главная сила OpenAPI — переиспользование через `$ref`.

```yaml
components:
  schemas:
    User:
      type: object
      properties:
        id: { type: string }
        email: { type: string }

    UserList:
      type: object
      properties:
        items:
          type: array
          items:
            $ref: '#/components/schemas/User'  # ← переиспользуем
        total:
          type: integer
```

Это даёт DRY и единый источник правды для типов.

### Типы данных

OpenAPI поддерживает базовые типы:

```yaml
type: string
type: integer    # int32
type: number     # float
type: boolean
type: array
type: object
type: null       # OpenAPI 3.1
```

С `format` уточняется:

```yaml
type: string
format: uuid           # 8400e290-...
format: email          # foo@bar.com
format: date           # 2026-01-15
format: date-time      # 2026-01-15T10:30:00Z (RFC3339)
format: byte           # base64-encoded
format: binary         # raw bytes
format: uri
format: ipv4
format: ipv6

type: integer
format: int64          # 64-bit
format: int32
```

### Constraints

```yaml
type: string
minLength: 1
maxLength: 100
pattern: '^[a-z]+$'

type: integer
minimum: 0
maximum: 1000
exclusiveMinimum: true

type: array
minItems: 1
maxItems: 50
uniqueItems: true
```

### Enums

```yaml
status:
  type: string
  enum: [active, inactive, banned]
```

### nullable

В OpenAPI 3.0:
```yaml
created_at:
  type: string
  format: date-time
  nullable: true
```

В OpenAPI 3.1 (более стандартный JSON Schema):
```yaml
created_at:
  type: [string, "null"]
  format: date-time
```

### Polymorphism: oneOf, anyOf, allOf

```yaml
# Один из вариантов
PaymentMethod:
  oneOf:
    - $ref: '#/components/schemas/CreditCard'
    - $ref: '#/components/schemas/BankTransfer'
    - $ref: '#/components/schemas/Cryptocurrency'
  discriminator:
    propertyName: type

# Несколько типов одновременно (наследование)
ExtendedUser:
  allOf:
    - $ref: '#/components/schemas/User'
    - type: object
      properties:
        permissions:
          type: array
          items:
            type: string
```

---

## Два подхода: code-first vs spec-first

### Code-first

Пишешь Go-код. Из аннотаций/комментариев генерируется OpenAPI spec.

**Плюсы:**
- Spec всегда соответствует коду
- Не нужно отдельно обновлять spec
- Удобно когда код уже есть

**Минусы:**
- Frontend и mobile должны ждать пока backend закодит, чтобы получить spec
- Меньше контроля над структурой spec'а
- Сложно ревьюить контракт API в PR

### Spec-first

Сначала пишешь OpenAPI spec, потом из него генерируется server stub в Go.

**Плюсы:**
- Контракт обсуждается до написания кода
- Frontend и mobile могут начать работу параллельно (с моков)
- Spec — единый источник правды
- Легко делать review PR с изменением контракта

**Минусы:**
- Нужно обновлять spec при каждом изменении
- Кодогенерация может создавать "неудобный" Go код
- Учить YAML/syntax OpenAPI

### Что выбирать

**Spec-first** считается best practice для больших проектов:
- Multiple teams (frontend, mobile, backend)
- Public API
- AI integrations

**Code-first** окей для:
- Internal API одной команды
- Прототипов и MVP
- Когда команда уже знает Go и не хочет учить OpenAPI

---

## Spec-first: oapi-codegen в Go

[oapi-codegen](https://github.com/oapi-codegen/oapi-codegen) — самый популярный Go-инструмент для генерации серверов из OpenAPI spec'а.

### Установка

```bash
go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest
```

### Генерация

```bash
# Server types + handlers interface
oapi-codegen -generate types,chi-server -package api openapi.yaml > api/api.gen.go
```

### Сгенерированный код

```go
// Из spec'а выше — генерируются типы:
type User struct {
    Id        openapi_types.UUID `json:"id"`
    Email     openapi_types.Email `json:"email"`
    Name      string `json:"name"`
    CreatedAt *time.Time `json:"created_at,omitempty"`
}

type CreateUserRequest struct {
    Email openapi_types.Email `json:"email"`
    Name  string `json:"name"`
}

// Интерфейс, который нужно реализовать:
type ServerInterface interface {
    ListUsers(w http.ResponseWriter, r *http.Request, params ListUsersParams)
    CreateUser(w http.ResponseWriter, r *http.Request)
    GetUserById(w http.ResponseWriter, r *http.Request, id openapi_types.UUID)
}
```

### Реализация

<details>
<summary>Реализация сгенерированного интерфейса целиком</summary>

```go
type API struct {
    users UserRepository
}

func (a *API) ListUsers(w http.ResponseWriter, r *http.Request, params ListUsersParams) {
    limit := 20
    if params.Limit != nil {
        limit = *params.Limit
    }

    users, nextCursor, err := a.users.List(r.Context(), limit, params.Cursor)
    if err != nil {
        writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
        return
    }

    response := struct {
        Users      []User  `json:"users"`
        NextCursor *string `json:"next_cursor,omitempty"`
    }{
        Users:      users,
        NextCursor: nextCursor,
    }

    json.NewEncoder(w).Encode(response)
}

func (a *API) CreateUser(w http.ResponseWriter, r *http.Request) {
    var req CreateUserRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
        return
    }

    user, err := a.users.Create(r.Context(), string(req.Email), req.Name)
    if err != nil {
        // ...
    }

    w.WriteHeader(http.StatusCreated)
    json.NewEncoder(w).Encode(user)
}

func (a *API) GetUserById(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
    user, err := a.users.GetByID(r.Context(), id.String())
    // ...
}
```

</details>

### Подключение к роутеру

```go
import "github.com/go-chi/chi/v5"

func main() {
    r := chi.NewRouter()
    api := &API{users: ...}
    HandlerFromMux(api, r)  // сгенерированная функция

    http.ListenAndServe(":8080", r)
}
```

### Что даёт

- **Типобезопасность:** структуры запросов/ответов сгенерированы из spec
- **Валидация:** middleware из oapi-codegen может валидировать payload против spec
- **Автоматизация:** меняешь spec → `go generate` → новые типы, ошибки компиляции в местах несовместимости

### //go:generate директива

```go
//go:generate oapi-codegen -config gen-config.yaml openapi.yaml
package api
```

```yaml
# gen-config.yaml
package: api
output: api.gen.go
generate:
  - types
  - chi-server
  - strict-server
  - spec
```

Теперь `go generate ./...` обновит сгенерированный код.

---

## Code-first: swag из аннотаций

[swaggo/swag](https://github.com/swaggo/swag) — генерация OpenAPI 2.0 из аннотаций в Go-коде. OpenAPI 2.0 — старая версия (Swagger), но широко используется.

```go
// @title Users API
// @version 1.0
// @host api.example.com
// @BasePath /v1

// @Summary List users
// @Tags users
// @Param limit query int false "Limit" default(20) maximum(100)
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} UserListResponse
// @Failure 401 {object} ErrorResponse
// @Router /users [get]
func listUsers(c *gin.Context) {
    // ...
}

type User struct {
    ID    string `json:"id" example:"8400e290-..."`
    Email string `json:"email" example:"alice@example.com"`
    Name  string `json:"name" example:"Alice"`
}
```

```bash
swag init  # генерирует docs/swagger.yaml
```

**Минусы:**
- OpenAPI 2.0 (не 3.0)
- Аннотации засоряют код
- Легко забыть обновить при изменении логики

В современных Go-проектах spec-first предпочтительнее. Но swag популярен в legacy.

---

## Validation против спецификации

OpenAPI можно использовать как runtime валидатор запросов:

```go
import "github.com/getkin/kin-openapi/openapi3"
import "github.com/getkin/kin-openapi/openapi3filter"

func openAPIValidationMiddleware(spec *openapi3.T) func(http.Handler) http.Handler {
    router, _ := gorillamux.NewRouter(spec)

    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            route, pathParams, _ := router.FindRoute(r)
            if route == nil {
                http.Error(w, "route not in spec", http.StatusNotFound)
                return
            }

            requestValidationInput := &openapi3filter.RequestValidationInput{
                Request:    r,
                PathParams: pathParams,
                Route:      route,
            }
            if err := openapi3filter.ValidateRequest(r.Context(), requestValidationInput); err != nil {
                http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
                return
            }

            next.ServeHTTP(w, r)
        })
    }
}
```

Преимущество: вся валидация — в spec'е. Не нужно дублировать в коде.

oapi-codegen с флагом `strict-server` делает похожее автоматически.

---

## Contract testing

Проверяет: реальный ответ сервера соответствует spec'у?

### Подход 1: автогенерация тестов

Из spec'а генерируются тесты типа "POST /users с валидным body → 201 с правильной схемой ответа".

Tools: [Schemathesis](https://schemathesis.io/), [Dredd](https://github.com/apiaryio/dredd).

```bash
schemathesis run --base-url=http://localhost:8080 openapi.yaml
```

Schemathesis генерирует случайные валидные запросы по spec'у и проверяет ответы. Находит крайние случаи, которые не покрыты тестами.

### Подход 2: проверка в integration tests

```go
func TestGetUser(t *testing.T) {
    resp := testRequest(t, "GET", "/users/123", nil)

    // Валидация против spec'а
    var responseValidator openapi3filter.ResponseValidator
    err := openapi3filter.ValidateResponse(ctx, responseValidator.WithRequest(req).WithResponse(resp))
    require.NoError(t, err)

    // Дополнительная проверка содержимого
    var user User
    json.Unmarshal(resp.Body, &user)
    assert.Equal(t, "alice", user.Name)
}
```

### Подход 3: consumer-driven contracts (Pact)

Frontend описывает что ожидает: "GET /users возвращает массив с полями id, name". Backend проверяет что реальный ответ удовлетворяет этому контракту.

Полезно при independent deployment.

---

## Backward compatibility

Изменения API — breaking или non-breaking.

### Non-breaking changes (безопасно)

- Добавить **необязательный** параметр запроса
- Добавить **необязательное** поле в response
- Добавить новый endpoint
- Добавить новый код ответа (если клиент handle'ит unknown)
- Расширить enum (если клиент handle'ит unknown values)

### Breaking changes (требуют versioning)

- Удалить endpoint или поле
- Сделать optional поле required
- Изменить тип поля (string → integer)
- Изменить URL endpoint'а
- Сузить enum (удалить значение)
- Сменить семантику (status code'а или формата)

### Стратегии для breaking changes

**1. URL versioning:**
```
/v1/users
/v2/users      ← новая версия
```

**2. Header versioning:**
```
Accept: application/vnd.myapi.v2+json
```

**3. Deprecation header:**
```yaml
/users:
  get:
    deprecated: true
    description: |
      DEPRECATED: используйте /v2/users.
      Этот endpoint будет удалён 2026-12-01.
```

### Tools для detect breaking changes

[oasdiff](https://github.com/Tufin/oasdiff) — сравнивает две версии spec'а и говорит что breaking:

```bash
oasdiff breaking spec-v1.yaml spec-v2.yaml
```

Запускать в CI на каждый PR с изменением spec'а.

---

## Swagger UI: документация для клиентов

Swagger UI — интерактивная документация. Открываешь URL → видишь все endpoint'ы → можешь сделать запрос прямо из браузера.

```go
// Раздать spec и UI со своего сервера
import _ "embed"

//go:embed openapi.yaml
var openAPISpec []byte

func setupRoutes(r *chi.Mux) {
    // Раздать spec
    r.Get("/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/yaml")
        w.Write(openAPISpec)
    })

    // Раздать Swagger UI HTML
    r.Get("/docs", swaggerUIHandler)
}

const swaggerUIHTML = `<!DOCTYPE html>
<html>
<head>
    <title>API Docs</title>
    <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
</head>
<body>
    <div id="swagger-ui"></div>
    <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
    <script>
        SwaggerUIBundle({
            url: "/openapi.yaml",
            dom_id: "#swagger-ui"
        });
    </script>
</body>
</html>`

func swaggerUIHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/html")
    w.Write([]byte(swaggerUIHTML))
}
```

Открываешь `http://localhost:8080/docs` — видишь красивую интерактивную доку.

Альтернатива: [Redoc](https://github.com/Redocly/redoc) — другой UI, многим нравится больше.

---

## OpenAPI для AI-агентов

В 2026 OpenAI/Anthropic API поддерживают function calling с OpenAPI spec'ом.

Идея: LLM получает описание API → понимает, что может делать → может сам вызывать его endpoint'ы.

```python
import openai

# Загружаем spec
with open('openapi.yaml') as f:
    spec = yaml.safe_load(f)

# Используем как описание tools для LLM
response = openai.chat.completions.create(
    model="gpt-4o-mini",
    messages=[{"role": "user", "content": "Найди мне пользователя Alice"}],
    tools=convert_openapi_to_tools(spec),
)

# LLM решает: "нужно вызвать listUsers с фильтром name=Alice"
# Возвращает tool call → приложение его исполняет → отдаёт результат назад
```

Это даёт огромное преимущество для AI-интеграций:
- LLM сам понимает контракт API
- Не нужно описывать каждый endpoint вручную
- Updates spec'а → LLM автоматически знает новые возможности

В PDF "Топ-1% Backend Roadmap" это упоминается как "кормёжка для AI-агентов" — становится стандартной фичей хорошо спроектированных API.

---

## Anti-patterns

**1. Spec и код расходятся.**
Spec говорит "поле `email` обязательное", код возвращает без него. → Контракт сломан. Решение: contract testing в CI.

**2. Любая ошибка возвращает {"error": "..."}.**
Spec говорит про 5 разных типов ошибок, реально все возвращают одинаково. Бесполезно. Используй разные схемы для разных ошибок, или хотя бы поле `code`.

**3. Слишком "свободный" spec.**
```yaml
data:
  type: object
  # ничего больше — "любой object"
```
Бесполезно. Опиши поля. Если действительно любой — отметь явно.

**4. Inline schemas вместо $ref.**
Дублирование. Если `User` используется в 10 endpoint'ах — в `components/schemas/User` один раз и `$ref` везде.

**5. Не использовать examples.**
```yaml
example: "alice@example.com"
```
В Swagger UI пользователи видят пример и понимают формат. Без examples — гадают.

**6. Path versioning без рассуждений.**
`/v1`, `/v2`, `/v3` без чёткого процесса депрекации старых. Накапливается зоопарк, никто не знает где что.

**7. Игнорировать nullable.**
JSON может прислать `"field": null`. Если в spec'е не nullable — клиент сломается. Будь явным: `nullable: true` или `type: [string, "null"]`.

**8. Required без причины.**
Если поле может быть пустым в некоторых сценариях — не делай его required. Иначе клиенту приходится слать пустые строки чтобы валидировать.

**9. Запутанные имена.**
`POST /user-actions` с body `{"action": "delete", "user_id": 42}` вместо `DELETE /users/42`. Лучше REST-conventional, OpenAPI это явно поддерживает.

**10. Не валидировать против spec'а на сервере.**
Spec говорит "limit max 100", сервер принимает limit=1000000. → DOS-уязвимость или просто баг. Валидируй runtime (oapi-codegen middleware, kin-openapi).

---

## Interview-ready answer

**1. Зачем нужен OpenAPI, если есть документация в вики?**

- Формат — машиночитаемое описание контракта, поэтому по нему генерируются клиенты, серверные заглушки, тесты и UI документации, а не только текст для человека.
- Синхронность — документ в вики устаревает молча, спецификация в репозитории ломает сборку или контрактный тест, если разошлась с кодом.
- Общий язык — фронтенд, мобильные и внешние потребители работают с одним артефактом, а не с тремя пересказами.
- Побочный эффект — спецификация становится входом для линтеров, проверок совместимости и валидации запросов на рантайме.

**2. Code-first или spec-first?**

- Spec-first — сначала контракт, потом код: контракт можно обсудить с потребителями до реализации, а сервер и клиент генерируются из одного источника.
- Code-first — сначала код, спецификация генерируется из аннотаций: быстрее стартовать, но контракт становится следствием реализации и легко протекает наружу деталями.
- Практическое правило — публичный или межкомандный API делают spec-first, внутренний сервис одной команды допустимо вести code-first.
- Общее требование для обоих — спецификация лежит в репозитории и проверяется в CI, иначе оба подхода вырождаются в устаревший файл.

**3. Что считается ломающим изменением контракта?**

- Ломающие — удалить или переименовать поле, сделать необязательное поле обязательным, сузить тип или набор допустимых значений, изменить формат ошибки, убрать эндпоинт.
- Неломающие — добавить необязательный параметр запроса, добавить поле в ответ, добавить новый эндпоинт или новое значение в перечисление, которое клиент обязан игнорировать при незнании.
- Проверка — сравнение спецификаций между версиями инструментом вроде oasdiff в CI, а не глазами в ревью.
- Когда ломать всё-таки надо — новая версия пути или медиатипа с периодом параллельной работы и явным сроком отключения старой.

**4. Как спецификация помогает в тестировании?**

- Валидация на рантайме — middleware проверяет входящие запросы против схемы, поэтому неверный тип или превышенный лимит отсекаются до бизнес-логики.
- Контрактные тесты — проверяют, что реальные ответы сервера соответствуют обещанному в спецификации, и ловят расхождение до релиза.
- Property-based проверки — генератор строит валидные по схеме запросы и находит краевые случаи, которых нет в написанных вручную тестах.
- Граница возможностей — спецификация проверяет форму, а не смысл: бизнес-правила и права доступа ей не описываются.

**5. Чем OpenAPI отличается от контракта в Protobuf?**

- Область — OpenAPI описывает HTTP-семантику: пути, методы, коды ответов, заголовки, медиатипы; Protobuf описывает сообщения и вызовы, а транспорт задаёт gRPC.
- Строгость — Protobuf по построению не даёт разойтись коду и схеме, потому что код всегда генерируется; в OpenAPI при code-first расхождение возможно.
- Эволюция — в Protobuf правила совместимости встроены в модель (номера полей, зарезервированные значения), в OpenAPI их приходится обеспечивать дисциплиной и проверками.
- Совмещение — частая схема: gRPC внутри с proto-контрактом и REST-фасад наружу с OpenAPI, сгенерированный из того же описания или транскодингом.

---

## Полезные ссылки

- [OpenAPI Specification 3.0](https://spec.openapis.org/oas/v3.0.3) — официальная спецификация
- [OpenAPI Specification 3.1](https://spec.openapis.org/oas/v3.1.0) — новая версия, ближе к JSON Schema
- [Swagger Editor](https://editor.swagger.io/) — онлайн редактор spec'а с live preview
- [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen) — основной Go-инструмент для spec-first
- [kin-openapi](https://github.com/getkin/kin-openapi) — Go библиотека для парсинга и validation
- [oasdiff](https://github.com/Tufin/oasdiff) — diff между версиями spec
- [Schemathesis](https://schemathesis.io/) — property-based contract testing
- [Spectral](https://stoplight.io/open-source/spectral) — линтер для OpenAPI (best practices, naming)
- [Redoc](https://github.com/Redocly/redoc) — альтернативный UI
