# chi

`go-chi/chi` — минимальный идиоматичный роутер. Совместим с `net/http`, не добавляет собственный context-тип, строится поверх стандартного middleware-паттерна.

## Содержание

- [Почему chi](#почему-chi)
- [Базовый роутер](#базовый-роутер)
- [URL параметры](#url-параметры)
- [Middleware](#middleware)
- [Группы и subrouters](#группы-и-subrouters)
- [Mount](#mount)
- [Встроенные middleware](#встроенные-middleware)
- [Полный пример](#полный-пример)
- [Trade-offs](#trade-offs)

---

## Почему chi

chi решает главную проблему stdlib до 1.22 — отсутствие удобного routing с path params — и при этом остаётся полностью совместимым с `net/http`.

**Любой** `http.Handler` и `http.HandlerFunc` работает напрямую с chi. Любая net/http-совместимая middleware работает с chi без адаптеров.

```go
import "github.com/go-chi/chi/v5"

r := chi.NewRouter()

// net/http handler — работает напрямую
r.Handle("/static/*", http.FileServer(http.Dir("./static")))

// net/http middleware — работает напрямую
r.Use(someStdlibMiddleware)
```

В отличие от gin/echo, chi не оборачивает `http.Request` и `http.ResponseWriter` в собственный Context — это означает, что весь существующий net/http-код работает без изменений.

---

## Базовый роутер

```go
r := chi.NewRouter()

r.Get("/users", listUsers)
r.Post("/users", createUser)
r.Get("/users/{id}", getUser)
r.Put("/users/{id}", updateUser)
r.Delete("/users/{id}", deleteUser)
r.Patch("/users/{id}", patchUser)

// Любой метод
r.HandleFunc("/webhook", handleWebhook)

// Connect, Options, Trace, Head
r.Options("/users", optionsHandler)
```

---

## URL параметры

```go
import "github.com/go-chi/chi/v5"

func getUser(w http.ResponseWriter, r *http.Request) {
    id := chi.URLParam(r, "id")
    // ...
}

// Несколько параметров
r.Get("/orgs/{orgID}/repos/{repoID}", func(w http.ResponseWriter, r *http.Request) {
    orgID  := chi.URLParam(r, "orgID")
    repoID := chi.URLParam(r, "repoID")
})

// Wildcard остаток пути
r.Get("/files/*", func(w http.ResponseWriter, r *http.Request) {
    path := chi.URLParam(r, "*")
})
```

`chi.URLParam` читает из `context.Context` — значение добавляет chi при маршрутизации. Это стандартный context, не специальный тип.

---

## Middleware

Middleware в chi — стандартный `func(http.Handler) http.Handler`:

```go
func RequestID(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        id := uuid.New().String()
        ctx := context.WithValue(r.Context(), requestIDKey, id)
        w.Header().Set("X-Request-ID", id)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

r := chi.NewRouter()
r.Use(RequestID)
r.Use(middleware.Logger)    // встроенный
r.Use(middleware.Recoverer) // встроенный
```

Порядок `Use` — порядок выполнения (первый добавленный — самый внешний wrapper).

---

## Группы и subrouters

Группы позволяют применить middleware к подмножеству маршрутов:

```go
r := chi.NewRouter()
r.Use(middleware.Logger)

// Группа без дополнительного prefix — только shared middleware
r.Group(func(r chi.Router) {
    r.Use(AuthMiddleware)   // только для маршрутов в этой группе

    r.Get("/profile", getProfile)
    r.Put("/profile", updateProfile)
})

// Маршрут вне группы — без AuthMiddleware
r.Get("/healthz", healthCheck)
```

Subrouter с prefix:
```go
r.Route("/api/v1", func(r chi.Router) {
    r.Use(AuthMiddleware)
    r.Use(RateLimiter)

    r.Route("/users", func(r chi.Router) {
        r.Get("/", listUsers)
        r.Post("/", createUser)

        r.Route("/{userID}", func(r chi.Router) {
            r.Get("/", getUser)
            r.Put("/", updateUser)
            r.Delete("/", deleteUser)

            r.Route("/posts", func(r chi.Router) {
                r.Get("/", listUserPosts)
                r.Post("/", createUserPost)
            })
        })
    })
})
```

---

## Mount

`Mount` позволяет подключить отдельный `http.Handler` (в том числе другой chi Router) под prefix:

```go
adminRouter := chi.NewRouter()
adminRouter.Use(AdminAuthMiddleware)
adminRouter.Get("/users", adminListUsers)
adminRouter.Delete("/users/{id}", adminDeleteUser)

r := chi.NewRouter()
r.Mount("/admin", adminRouter)

// Также mount любого http.Handler:
r.Mount("/debug", middleware.Profiler())  // pprof endpoints
```

Это позволяет строить сервер из независимых модулей, каждый со своим роутером.

---

## Встроенные middleware

`github.com/go-chi/chi/v5/middleware`:

```go
r.Use(middleware.RequestID)        // добавляет X-Request-ID в context и header
r.Use(middleware.RealIP)           // берёт IP из X-Real-IP / X-Forwarded-For
r.Use(middleware.Logger)           // логирует метод, путь, статус, время
r.Use(middleware.Recoverer)        // восстанавливается после panic, возвращает 500
r.Use(middleware.Compress(5))      // gzip compression
r.Use(middleware.Timeout(60 * time.Second))  // deadline на весь запрос
r.Use(middleware.StripSlashes)     // /users/ → /users
r.Use(middleware.RedirectSlashes)  // /users → /users/ (альтернатива)
r.Use(middleware.Heartbeat("/ping"))  // простой health check endpoint
r.Use(middleware.NoCache)          // Cache-Control: no-cache headers

// Throttle — ограничение параллельных запросов
r.Use(middleware.Throttle(100))    // max 100 одновременных запросов

// AllowContentType — валидация Content-Type
r.Use(middleware.AllowContentType("application/json"))
```

Дополнительные пакеты от chi (отдельные модули):
- `github.com/go-chi/cors` — CORS middleware
- `github.com/go-chi/httprate` — rate limiting
- `github.com/go-chi/jwtauth` — JWT auth

---

## Полный пример

```go
package main

import (
    "encoding/json"
    "net/http"
    "time"

    "github.com/go-chi/chi/v5"
    "github.com/go-chi/chi/v5/middleware"
    "github.com/go-chi/cors"
)

type UserHandler struct {
    repo UserRepository
}

func (h *UserHandler) Routes() chi.Router {
    r := chi.NewRouter()
    r.Get("/", h.list)
    r.Post("/", h.create)
    r.Route("/{id}", func(r chi.Router) {
        r.Get("/", h.get)
        r.Put("/", h.update)
        r.Delete("/", h.delete)
    })
    return r
}

func (h *UserHandler) get(w http.ResponseWriter, r *http.Request) {
    id := chi.URLParam(r, "id")
    user, err := h.repo.Get(r.Context(), id)
    if err != nil {
        http.Error(w, "not found", http.StatusNotFound)
        return
    }
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(user)
}

func NewRouter(userRepo UserRepository) http.Handler {
    r := chi.NewRouter()

    r.Use(middleware.RequestID)
    r.Use(middleware.RealIP)
    r.Use(middleware.Logger)
    r.Use(middleware.Recoverer)
    r.Use(middleware.Timeout(30 * time.Second))
    r.Use(cors.Handler(cors.Options{
        AllowedOrigins: []string{"https://*.example.com"},
        AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
    }))

    r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
    })

    r.Route("/api/v1", func(r chi.Router) {
        r.Use(AuthMiddleware)
        r.Mount("/users", (&UserHandler{repo: userRepo}).Routes())
    })

    return r
}
```

---

## Trade-offs

**Плюсы:**
- Полная совместимость с `net/http` — любой stdlib-handler и middleware работает без адаптеров
- Нет собственного Context-типа — `r.Context()` это стандартный `context.Context`
- 0 внешних зависимостей (только stdlib)
- Удобные subrouters и Mount для модульной структуры
- Middleware порядок очевиден и предсказуем

**Минусы:**
- Нет встроенного binding и validation (нужно писать самому или добавлять пакеты)
- Нет встроенного ответа с negotiation (JSON/XML/etc) — только low-level `w.Write`
- Меньше готовых "батареек" чем у gin/echo

**Идеальный выбор когда:**
- Нужен роутер поверх stdlib без lock-in на фреймворк
- Важна совместимость с существующим net/http кодом
- Команда предпочитает идиоматичный Go без магии
- Не нужен тяжёлый binding/validation из коробки
