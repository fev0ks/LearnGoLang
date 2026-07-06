# gin

`gin-gonic/gin` — самый популярный Go HTTP-фреймворк по числу звёзд и реальному использованию. Полный набор возможностей из коробки: routing, binding, validation, middleware, response helpers.

## Содержание

- [Инициализация](#инициализация)
- [Routing и параметры](#routing-и-параметры)
- [gin.Context](#gincontext)
- [Binding и validation](#binding-и-validation)
- [Middleware](#middleware)
- [Response helpers](#response-helpers)
- [Группы и структура проекта](#группы-и-структура-проекта)
- [Производительность](#производительность)
- [Trade-offs](#trade-offs)

---

## Инициализация

```go
import "github.com/gin-gonic/gin"

// Default = Engine + Logger + Recovery middleware
r := gin.Default()

// New = чистый Engine без middleware
r := gin.New()
r.Use(gin.Logger())
r.Use(gin.Recovery())
```

**Разница важна**: `gin.Default()` добавляет Logger и Recovery автоматически. Для production часто используют `gin.New()` + свои middleware (zap/slog logger вместо gin-дефолтного).

```go
// Режим: debug (dev) / release (prod) / test
gin.SetMode(gin.ReleaseMode)   // или env GIN_MODE=release
```

В release mode gin не выводит routing table при старте и отключает debug-логирование.

---

## Routing и параметры

```go
r.GET("/users", listUsers)
r.POST("/users", createUser)
r.GET("/users/:id", getUser)        // path param
r.PUT("/users/:id", updateUser)
r.DELETE("/users/:id", deleteUser)
r.PATCH("/users/:id", patchUser)

// Wildcard — захватывает остаток пути
r.GET("/files/*filepath", serveFile)

// Любой метод
r.Any("/webhook", handleWebhook)

// Нет встроенного метода — через Handle
r.Handle("PROPFIND", "/dav/*path", davHandler)
```

Извлечение параметров:
```go
func getUser(c *gin.Context) {
    id := c.Param("id")            // path param :id

    page := c.Query("page")        // ?page=2
    limit := c.DefaultQuery("limit", "20")  // default value

    // POST form
    name := c.PostForm("name")
    role := c.DefaultPostForm("role", "user")
}

func serveFile(c *gin.Context) {
    filepath := c.Param("filepath")  // wildcard *filepath, включает ведущий /
}
```

---

## gin.Context

`*gin.Context` — центральный объект в gin. Не совместим с `http.Handler` — это **не** обёртка над net/http интерфейсами, это самостоятельный тип.

```go
// gin.Context содержит:
c.Request    // *http.Request — оригинальный запрос
c.Writer     // gin.ResponseWriter (обёртка над http.ResponseWriter)

// Проброс в net/http middleware невозможен напрямую:
// someStdlibMiddleware(c.Writer, c.Request) — технически работает,
// но gin.ResponseWriter несовместим с http.ResponseWriter интерфейсом как handler
```

Хранение данных в context (аналог context.WithValue):
```go
// Установить в middleware
c.Set("userID", "abc123")
c.Set("user", &User{ID: "abc123"})

// Получить в handler
userID := c.GetString("userID")
user, exists := c.Get("user")
if !exists {
    c.AbortWithStatus(http.StatusUnauthorized)
    return
}
typedUser := user.(*User)
```

**Важно**: `c.Set/Get` — это gin-специфичный механизм, не стандартный `context.Context`. Для совместимости с обычными Go-библиотеками (например, OpenTelemetry, database drivers) нужно использовать `c.Request.Context()`:

```go
// Правильно для передачи в стандартные библиотеки
result, err := repo.Get(c.Request.Context(), id)

// Не для межпакетного использования
val := c.GetString("userID")  // только внутри gin middleware/handler
```

---

## Binding и validation

Binding — автоматическое чтение данных из запроса и десериализация в struct. Gin использует `go-playground/validator` для валидации.

```go
type CreateUserRequest struct {
    Name     string `json:"name"     binding:"required,min=2,max=100"`
    Email    string `json:"email"    binding:"required,email"`
    Age      int    `json:"age"      binding:"gte=0,lte=130"`
    Role     string `json:"role"     binding:"omitempty,oneof=admin user viewer"`
    Password string `json:"password" binding:"required,min=8"`
}

func createUser(c *gin.Context) {
    var req CreateUserRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    // req заполнен и провалидирован
}
```

Виды binding:
```go
c.ShouldBindJSON(&req)      // Content-Type: application/json
c.ShouldBindXML(&req)       // Content-Type: application/xml
c.ShouldBindQuery(&req)     // query string (?key=value)
c.ShouldBindForm(&req)      // application/x-www-form-urlencoded или multipart
c.ShouldBindHeader(&req)    // HTTP headers
c.ShouldBindUri(&req)       // path params (struct теги: uri:"id")

// BindJSON (без Should) — пишет 400 автоматически при ошибке, не рекомендуется
c.BindJSON(&req)
```

**Разница `ShouldBind*` vs `Bind*`**: `Bind*` вызывает `c.AbortWithError(400, err)` при ошибке, что мешает контролировать формат ответа. Всегда используй `ShouldBind*`.

Привязка path params через struct:
```go
type UserURI struct {
    ID string `uri:"id" binding:"required,uuid"`
}

func getUser(c *gin.Context) {
    var uri UserURI
    if err := c.ShouldBindUri(&uri); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
}
```

---

## Middleware

```go
// Глобальные middleware
r := gin.New()
r.Use(gin.Logger())
r.Use(gin.Recovery())
r.Use(CORSMiddleware())

// Middleware принимает *gin.Context
func AuthMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        token := c.GetHeader("Authorization")
        userID, err := validateToken(token)
        if err != nil {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
                "error": "unauthorized",
            })
            return  // return нужен только для читаемости, AbortWith уже останавливает chain
        }
        c.Set("userID", userID)
        c.Next()  // продолжить обработку chain
    }
}

func LoggingMiddleware(logger *slog.Logger) gin.HandlerFunc {
    return func(c *gin.Context) {
        start := time.Now()
        c.Next()  // сначала выполнить handler
        logger.Info("request",
            "method", c.Request.Method,
            "path",   c.Request.URL.Path,
            "status", c.Writer.Status(),
            "dur",    time.Since(start),
        )
    }
}
```

`c.Abort()` / `c.AbortWithStatus()` / `c.AbortWithStatusJSON()` — останавливают выполнение remaining handlers в chain, но **не** текущий handler. Для выхода из текущей функции нужен явный `return`.

---

## Response helpers

```go
// JSON
c.JSON(http.StatusOK, gin.H{"id": "123", "name": "Alice"})
c.JSON(http.StatusCreated, user)

// Статус без тела
c.Status(http.StatusNoContent)

// String
c.String(http.StatusOK, "hello %s", name)

// Redirect
c.Redirect(http.StatusMovedPermanently, "/new-path")

// File
c.File("./static/index.html")
c.FileFromFS("index.html", http.FS(embeddedFS))

// Stream (SSE, chunked)
c.Stream(func(w io.Writer) bool {
    w.Write([]byte("data: message\n\n"))
    return true  // продолжать стриминг
})

// Abort + JSON в одном вызове
c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
```

---

## Группы и структура проекта

```go
r := gin.New()
r.Use(globalMiddleware)

// Группы с общим prefix
v1 := r.Group("/api/v1")
{
    v1.Use(AuthMiddleware())

    users := v1.Group("/users")
    {
        users.GET("", listUsers)
        users.POST("", createUser)
        users.GET("/:id", getUser)
        users.PUT("/:id", updateUser)
        users.DELETE("/:id", deleteUser)
    }

    posts := v1.Group("/posts")
    {
        posts.GET("", listPosts)
        posts.POST("", createPost)
    }
}

// Открытые эндпоинты (без AuthMiddleware)
r.GET("/healthz", healthCheck)
r.GET("/metrics", prometheusHandler)
```

Структура с handlers как методы struct:
```go
type UserHandler struct {
    service UserService
    logger  *slog.Logger
}

func (h *UserHandler) Register(r *gin.RouterGroup) {
    g := r.Group("/users")
    g.GET("", h.list)
    g.POST("", h.create)
    g.GET("/:id", h.get)
}

func (h *UserHandler) get(c *gin.Context) {
    id := c.Param("id")
    user, err := h.service.Get(c.Request.Context(), id)
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
        return
    }
    c.JSON(http.StatusOK, user)
}
```

---

## Производительность

Gin использует `julienschmidt/httprouter` под капотом — radix tree роутер. Это быстро, но это деталь реализации.

Gin также использует свой JSON encoder (sonic/go-json в зависимости от сборки) вместо stdlib `encoding/json`.

**Реальность**: производительность роутера редко является узким местом. Если сервис обрабатывает 10k RPS на одном инстансе — разница между gin и chi не измеримая в production. Bottleneck обычно: IO (DB, сеть), сериализация больших payload, GC давление.

Бенчмарки роутеров (req/sec) — полезны для академического понимания, бесполезны для большинства production-решений.

---

## Trade-offs

**Плюсы:**
- Самый популярный — много примеров, middleware, туториалов
- Binding + validation из коробки (go-playground/validator)
- Response helpers удобны для быстрой разработки
- Большая экосистема готовых gin-middleware

**Минусы:**
- `*gin.Context` несовместим с `net/http` интерфейсом — нельзя использовать stdlib/chi middleware без адаптеров
- `c.Set/Get` для passing данных вместо `context.Context` — нестандартный подход
- Lock-in на gin: код с `*gin.Context` сложно переносить
- Сообщения ошибок от validator не production-ready из коробки — нужна своя обработка
- Некоторые дефолты неудобны (Bind vs ShouldBind, gin.Default logformat)

**Идеальный выбор когда:**
- Команда уже знает gin и нет причин менять
- Нужен быстрый старт с батарейками из коробки
- Много эндпоинтов с разными форматами binding
- Внешний API с extensive validation требованиями
