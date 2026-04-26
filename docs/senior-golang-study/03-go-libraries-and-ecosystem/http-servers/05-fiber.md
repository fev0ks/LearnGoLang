# fiber

`gofiber/fiber` — HTTP-фреймворк построенный на `valyala/fasthttp` вместо стандартного `net/http`. Это фундаментальное архитектурное решение с серьёзными последствиями.

## Содержание

- [fasthttp vs net/http](#fasthttp-vs-nethttp)
- [Базовый синтаксис](#базовый-синтаксис)
- [fiber.Ctx](#fiberctx)
- [Middleware](#middleware)
- [Производительность](#производительность)
- [Несовместимость с net/http экосистемой](#несовместимость-с-nethttp-экосистемой)
- [Когда fiber оправдан](#когда-fiber-оправдан)
- [Trade-offs](#trade-offs)

---

## fasthttp vs net/http

Стандартный `net/http` создаёт новую `http.Request` struct на каждый запрос — heap allocation, GC pressure. fasthttp переиспользует объекты через sync.Pool — zero-allocation per request в типичных случаях.

Это даёт реальный прирост при:
- Очень высоком RPS (> 50-100k на инстансе)
- Маленьких payload (< 1KB)
- Задержка критична на уровне < 1ms

Для большинства backend-сервисов (10-20k RPS, работа с DB) разница между net/http и fasthttp не измеримая в production — bottleneck не в HTTP-парсинге.

---

## Базовый синтаксис

API намеренно похож на Express.js:

```go
import "github.com/gofiber/fiber/v2"

app := fiber.New(fiber.Config{
    ReadTimeout:  5 * time.Second,
    WriteTimeout: 10 * time.Second,
    BodyLimit:    4 * 1024 * 1024,  // 4MB
})

app.Get("/users", listUsers)
app.Post("/users", createUser)
app.Get("/users/:id", getUser)
app.Put("/users/:id", updateUser)
app.Delete("/users/:id", deleteUser)

// Wildcard
app.Get("/files/*", serveFiles)

// Группы
api := app.Group("/api/v1")
api.Use(authMiddleware)
api.Get("/profile", getProfile)

app.Listen(":8080")
```

---

## fiber.Ctx

`*fiber.Ctx` — не совместим с `*http.Request` / `http.ResponseWriter`. Это fasthttp RequestCtx под капотом.

```go
func getUser(c *fiber.Ctx) error {
    // Path params
    id := c.Params("id")

    // Query params
    page  := c.Query("page")
    limit := c.QueryInt("limit", 20)

    // Headers
    token := c.Get("Authorization")

    // Body JSON
    var req CreateUserRequest
    if err := c.BodyParser(&req); err != nil {
        return fiber.NewError(fiber.StatusBadRequest, "invalid JSON")
    }

    // Response
    return c.JSON(user)
    return c.Status(fiber.StatusCreated).JSON(user)
    return c.SendString("ok")
    return c.SendStatus(fiber.StatusNoContent)

    // Locals — аналог c.Set/Get в gin/echo
    c.Locals("userID", "abc")
    val := c.Locals("userID").(string)
}
```

**Критически важно**: `fiber.Ctx` содержит ссылку на fasthttp-объект который **переиспользуется**. Это означает:

```go
// ОПАСНО — ctx будет переиспользован после возврата handler
go func() {
    process(c.Body())  // c.Body() — ссылка, не копия
}()

// ПРАВИЛЬНО — скопировать до передачи в goroutine
body := make([]byte, len(c.Body()))
copy(body, c.Body())
go func() {
    process(body)
}()
```

---

## Middleware

```go
import "github.com/gofiber/fiber/v2/middleware/logger"
import "github.com/gofiber/fiber/v2/middleware/recover"
import "github.com/gofiber/fiber/v2/middleware/cors"
import "github.com/gofiber/fiber/v2/middleware/requestid"

app := fiber.New()
app.Use(logger.New())
app.Use(recover.New())
app.Use(requestid.New())
app.Use(cors.New())

// Своя middleware
func AuthMiddleware(c *fiber.Ctx) error {
    token := c.Get("Authorization")
    userID, err := validateToken(token)
    if err != nil {
        return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
    }
    c.Locals("userID", userID)
    return c.Next()
}

app.Use(AuthMiddleware)
```

Handlers также возвращают `error` — как в echo:
```go
func getUser(c *fiber.Ctx) error {
    user, err := repo.Get(context.Background(), c.Params("id"))
    if err != nil {
        return fiber.NewError(fiber.StatusNotFound, "user not found")
    }
    return c.JSON(user)
}
```

---

## Производительность

Типичные цифры из бенчмарков (TechEmpower):

| Framework | Req/sec (plaintext) |
|---|---|
| fiber (fasthttp) | ~500-600k |
| gin | ~100-150k |
| chi | ~80-120k |
| net/http stdlib | ~70-100k |

Бенчмарки измеряют throughput на синтетических запросах (Hello World, JSON echo). В реальных сервисах с обращениями к PostgreSQL (1-10ms) разница 50k vs 500k req/sec не имеет значения — bottleneck в IO.

**Когда разница реальна**: прокси/gateway сервис без IO, highload балансировщик, CDN edge-логика, in-memory кэш с очень высоким RPS.

---

## Несовместимость с net/http экосистемой

Это главный риск fiber. Всё, что написано под `net/http` — **не работает** с fiber без адаптеров.

**Не работает:**
```go
// net/http middleware — нельзя использовать напрямую
func StdlibAuth(next http.Handler) http.Handler { ... }
app.Use(StdlibAuth)  // ошибка компиляции — несовместимые типы

// OpenTelemetry net/http instrumentation
otelhttp.NewHandler(...)  // не применимо к fiber

// gorilla/sessions, любые net/http-based session libs
// prometheus net/http handler
// pprof (net/http/pprof)
```

Адаптер для net/http middleware существует, но это обёртка с оговорками:
```go
app.Use(adaptor.HTTPMiddleware(stdlibMiddleware))  // работает, но теряет часть fasthttp оптимизаций
```

**Следствие**: если твой проект использует OpenTelemetry, Prometheus, любые auth библиотеки на net/http — переход на fiber требует либо адаптеров (и потеря смысла), либо замены всех этих библиотек на fasthttp-варианты.

---

## Когда fiber оправдан

**Оправдан:**
- Сервис с > 50-100k RPS **без** тяжёлого IO
- Proxy/gateway layer, где латентность HTTP-парсинга измерима
- Изолированный микросервис без зависимости от net/http экосистемы
- Команда уже работала с Express.js и хочет похожий API

**Не оправдан:**
- Обычный CRUD-сервис с PostgreSQL/Redis
- Сервис в экосистеме с net/http middleware (OpenTelemetry, oauth2, etc.)
- Когда нужен pprof endpoint (стандартный net/http/pprof не применим)
- Greenfield проект без измеренного performance bottleneck

---

## Trade-offs

**Плюсы:**
- Реально самый быстрый Go HTTP фреймворк при высоком RPS
- Удобный API, похожий на Express.js
- Handlers возвращают error (как echo)
- Большая экосистема fiber-native middleware

**Минусы:**
- Несовместим с `net/http` — lock-in на fasthttp экосистему
- `fiber.Ctx` переиспользуется — легко словить data race при goroutine + ctx
- `context.Context` из fasthttp имеет ограничения — нет стандартного `context.WithValue` flow
- Нельзя использовать стандартные OpenTelemetry, oauth2, prometheus instrumentation без адаптеров
- Меньше production battle-tested историй чем у gin/chi

**Главный вопрос перед выбором fiber**: ты *измерил* что HTTP-слой является bottleneck? Если нет — вероятно, fiber не нужен.
