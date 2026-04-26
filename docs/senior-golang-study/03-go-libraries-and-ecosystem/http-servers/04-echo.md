# echo

`labstack/echo` — фреймворк уровня gin с одним принципиальным архитектурным отличием: **handlers возвращают `error`**. Это меняет паттерн обработки ошибок по всему приложению.

## Содержание

- [Ключевое отличие от gin: error return](#ключевое-отличие-от-gin-error-return)
- [Инициализация и routing](#инициализация-и-routing)
- [echo.Context](#echocontext)
- [Binding и validation](#binding-и-validation)
- [Middleware](#middleware)
- [Centralized error handling](#centralized-error-handling)
- [Группы](#группы)
- [Trade-offs и сравнение с gin](#trade-offs-и-сравнение-с-gin)

---

## Ключевое отличие от gin: error return

В gin handler не возвращает ничего:
```go
// gin
func getUser(c *gin.Context) {
    user, err := repo.Get(c.Request.Context(), c.Param("id"))
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
        return  // нужен явный return
    }
    c.JSON(http.StatusOK, user)
}
```

В echo handler возвращает `error`:
```go
// echo
func getUser(c echo.Context) error {
    user, err := repo.Get(c.Request().Context(), c.Param("id"))
    if err != nil {
        return echo.NewHTTPError(http.StatusNotFound, "not found")
        // или просто: return err — echo разберётся через HTTPErrorHandler
    }
    return c.JSON(http.StatusOK, user)
}
```

Это означает:
- Не нужен `return` после каждого `c.JSON(...)` — `return c.JSON(...)` достаточно
- Ошибки можно централизованно обрабатывать через `HTTPErrorHandler`
- Стандартный Go-паттерн `if err != nil { return err }` работает нативно

---

## Инициализация и routing

```go
import "github.com/labstack/echo/v4"

e := echo.New()
e.HideBanner = true   // убрать ASCII banner при старте

e.GET("/users", listUsers)
e.POST("/users", createUser)
e.GET("/users/:id", getUser)
e.PUT("/users/:id", updateUser)
e.DELETE("/users/:id", deleteUser)

e.Logger.Fatal(e.Start(":8080"))
```

Параметры пути:
```go
func getUser(c echo.Context) error {
    id := c.Param("id")         // path param :id
    return c.JSON(http.StatusOK, gin.H{"id": id})
}

// Query params
func listUsers(c echo.Context) error {
    page  := c.QueryParam("page")
    limit := c.QueryParamOrDefault("limit", "20")
    return c.JSON(http.StatusOK, map[string]string{"page": page})
}
```

---

## echo.Context

Как и gin, echo оборачивает запрос/ответ в собственный тип `echo.Context`. Не совместим с `net/http` handler интерфейсом.

```go
// Доступ к стандартным объектам
c.Request()        // *http.Request
c.Response()       // *echo.Response (обёртка над http.ResponseWriter)
c.Response().Writer  // оригинальный http.ResponseWriter

// Хранение данных
c.Set("userID", "abc")
val := c.Get("userID").(string)

// Для стандартных библиотек — использовать Request().Context()
result, err := repo.Get(c.Request().Context(), id)
```

---

## Binding и validation

Echo поддерживает binding из JSON, XML, form, query params, path params:

```go
type CreateUserRequest struct {
    Name  string `json:"name"  validate:"required,min=2"`
    Email string `json:"email" validate:"required,email"`
    Age   int    `json:"age"   validate:"gte=0,lte=130"`
}

func createUser(c echo.Context) error {
    var req CreateUserRequest
    if err := c.Bind(&req); err != nil {
        return echo.NewHTTPError(http.StatusBadRequest, err.Error())
    }
    if err := c.Validate(req); err != nil {
        return err  // обработается в HTTPErrorHandler
    }
    // ...
    return c.JSON(http.StatusCreated, user)
}
```

**Важно**: echo не включает validator по умолчанию. Нужно зарегистрировать свой:

```go
import "github.com/go-playground/validator/v10"

type CustomValidator struct {
    v *validator.Validate
}

func (cv *CustomValidator) Validate(i interface{}) error {
    if err := cv.v.Struct(i); err != nil {
        return echo.NewHTTPError(http.StatusBadRequest, err.Error())
    }
    return nil
}

e := echo.New()
e.Validator = &CustomValidator{v: validator.New()}
```

Bind по источникам:
```go
// Только JSON body
if err := c.Bind(&req); err != nil { ... }  // автоопределение по Content-Type

// Query params в struct
type Pagination struct {
    Page  int `query:"page"`
    Limit int `query:"limit"`
}
var p Pagination
if err := c.Bind(&p); err != nil { ... }

// Path params в struct
type UserParams struct {
    ID string `param:"id"`
}
var up UserParams
if err := c.Bind(&up); err != nil { ... }
```

---

## Middleware

```go
import "github.com/labstack/echo/v4/middleware"

e := echo.New()
e.Use(middleware.Logger())
e.Use(middleware.Recover())
e.Use(middleware.RequestID())
e.Use(middleware.CORS())

// Middleware — func(echo.HandlerFunc) echo.HandlerFunc
func AuthMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
    return func(c echo.Context) error {
        token := c.Request().Header.Get("Authorization")
        userID, err := validateToken(token)
        if err != nil {
            return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
        }
        c.Set("userID", userID)
        return next(c)
    }
}

e.Use(AuthMiddleware)
```

Встроенные middleware:
- `middleware.Logger()` — structured logging с форматом
- `middleware.Recover()` — panic recovery
- `middleware.RequestID()` — X-Request-ID
- `middleware.CORS()` — CORS headers
- `middleware.RateLimiter(...)` — rate limiting
- `middleware.Gzip()` — gzip compression
- `middleware.JWT(...)` — JWT auth
- `middleware.BasicAuth(...)` — Basic auth
- `middleware.BodyLimit("2M")` — ограничение размера body
- `middleware.TimeoutWithConfig(...)` — timeout per request

---

## Centralized error handling

Главное преимущество echo — всё что возвращают handlers идёт через один `HTTPErrorHandler`:

```go
e := echo.New()
e.HTTPErrorHandler = func(err error, c echo.Context) {
    var he *echo.HTTPError
    if errors.As(err, &he) {
        c.JSON(he.Code, map[string]any{
            "error":   he.Message,
            "code":    he.Code,
        })
        return
    }

    // Кастомные ошибки приложения
    var appErr *AppError
    if errors.As(err, &appErr) {
        c.JSON(appErr.HTTPStatus(), map[string]any{
            "error": appErr.Message,
            "code":  appErr.Code,
        })
        return
    }

    // Любые другие ошибки — 500
    c.Logger().Error(err)
    c.JSON(http.StatusInternalServerError, map[string]any{
        "error": "internal server error",
    })
}
```

Теперь handlers могут просто `return err` — формат ответа определяется централизованно:
```go
func getUser(c echo.Context) error {
    user, err := repo.Get(c.Request().Context(), c.Param("id"))
    if err != nil {
        return &AppError{Code: "USER_NOT_FOUND", HTTPCode: 404}
    }
    return c.JSON(http.StatusOK, user)
}
```

---

## Группы

```go
e := echo.New()
e.Use(middleware.Logger())

// Открытые эндпоинты
e.GET("/healthz", healthCheck)

// API группа
api := e.Group("/api/v1")
api.Use(AuthMiddleware)

users := api.Group("/users")
users.GET("", listUsers)
users.POST("", createUser)
users.GET("/:id", getUser)
users.PUT("/:id", updateUser)

posts := api.Group("/posts")
posts.GET("", listPosts)
```

---

## Trade-offs и сравнение с gin

| Аспект | gin | echo |
|---|---|---|
| Handler signature | `func(c *gin.Context)` | `func(c echo.Context) error` |
| Error handling | ручной `return` после каждого ответа | centralized via `HTTPErrorHandler` |
| Validation | встроена (go-playground) | нужно зарегистрировать самому |
| net/http совместимость | нет | нет (оба несовместимы) |
| Производительность | схожая | схожая |
| Популярность | выше | ниже, но существенная |
| Встроенный JWT | нет | есть middleware |
| Context API | `c.Set/Get` | `c.Set/Get` |

**Выбирай echo когда:**
- Команда предпочитает `return error` паттерн — он более Go-идиоматичен
- Нужен centralized error handler с полным контролем над форматом ошибок
- Хочешь меньше `return` после каждой ошибки в handlers

**Выбирай gin когда:**
- Команда уже знает gin
- Важна максимальная простота без настройки validator
- Большая экосистема gin-специфичных middleware нужна
