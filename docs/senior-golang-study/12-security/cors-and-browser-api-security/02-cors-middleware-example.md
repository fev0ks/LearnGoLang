# CORS middleware в Go

## Содержание

- [Безопасная реализация](#безопасная-реализация)
- [Готовые библиотеки](#готовые-библиотеки)
- [Конфигурация для разных сред](#конфигурация-для-разных-сред)

---

## Безопасная реализация

```go
type CORSConfig struct {
    AllowedOrigins []string
    AllowedHeaders []string
    MaxAge         int
}

func CORSMiddleware(cfg CORSConfig) func(http.Handler) http.Handler {
    allowed := make(map[string]struct{}, len(cfg.AllowedOrigins))
    for _, o := range cfg.AllowedOrigins {
        allowed[o] = struct{}{}
    }

    allowedHeaders := strings.Join(cfg.AllowedHeaders, ", ")

    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            origin := r.Header.Get("Origin")
            if origin == "" {
                // Не браузерный запрос или same-origin — CORS не нужен
                next.ServeHTTP(w, r)
                return
            }

            // Добавить к Vary, не перезаписывать
            w.Header().Add("Vary", "Origin")

            if _, ok := allowed[origin]; !ok {
                // Неизвестный origin — отклонить preflight, пропустить обычный
                if r.Method == http.MethodOptions {
                    w.WriteHeader(http.StatusForbidden)
                    return
                }
                next.ServeHTTP(w, r)
                return
            }

            w.Header().Set("Access-Control-Allow-Origin", origin)
            w.Header().Set("Access-Control-Allow-Credentials", "true")
            w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
            w.Header().Set("Access-Control-Allow-Headers", allowedHeaders)
            w.Header().Set("Access-Control-Expose-Headers", "X-Request-Id, X-Total-Count")

            if r.Method == http.MethodOptions {
                w.Header().Add("Vary", "Access-Control-Request-Method")
                w.Header().Add("Vary", "Access-Control-Request-Headers")
                w.Header().Set("Access-Control-Max-Age", strconv.Itoa(cfg.MaxAge))
                w.WriteHeader(http.StatusNoContent)
                return
            }

            next.ServeHTTP(w, r)
        })
    }
}
```

**Ключевые отличия от небезопасного варианта:**

- `allowed[origin]` — allowlist вместо слепого отражения любого Origin
- `w.Header().Add("Vary", ...)` — не перезаписывает Vary от других middleware
- preflight отдельно обрабатывается для разрешённых origins
- `AllowedHeaders` — явный список, не зеркало `Access-Control-Request-Headers`

**Использование:**

```go
cors := CORSMiddleware(CORSConfig{
    AllowedOrigins: []string{
        "https://app.example.com",
        "http://localhost:3000",
    },
    AllowedHeaders: []string{
        "Content-Type", "Authorization", "X-Request-Id",
    },
    MaxAge: 600,
})

mux := http.NewServeMux()
// ...
http.ListenAndServe(":8080", cors(mux))
```

---

## Готовые библиотеки

Для нетривиальных случаев (несколько origins на env, разные правила по routes) лучше использовать проверенные библиотеки:

**[rs/cors](https://github.com/rs/cors)** — наиболее распространённый вариант:

```go
import "github.com/rs/cors"

c := cors.New(cors.Options{
    AllowedOrigins:   []string{"https://app.example.com"},
    AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE"},
    AllowedHeaders:   []string{"Authorization", "Content-Type", "X-Request-Id"},
    ExposedHeaders:   []string{"X-Request-Id", "X-Total-Count"},
    AllowCredentials: true,
    MaxAge:           600,
})

handler := c.Handler(mux)
```

**Когда самописный middleware достаточен:**
- один сервис, простая политика
- origins и headers не меняются между deploys

**Когда лучше библиотека или gateway:**
- несколько сервисов с одинаковой политикой → gateway (nginx/Envoy/Traefik)
- сложные allowlists, разные origins на env → `rs/cors` с конфигом из env
- нужен per-route CORS → `rs/cors` с `AllowOriginFunc`

---

## Конфигурация для разных сред

```go
func corsFromEnv() CORSConfig {
    origins := os.Getenv("CORS_ALLOWED_ORIGINS")
    if origins == "" {
        // local dev — разрешить localhost
        return CORSConfig{
            AllowedOrigins: []string{"http://localhost:3000", "http://localhost:5173"},
            AllowedHeaders: []string{"Content-Type", "Authorization", "X-Request-Id"},
            MaxAge:         60,
        }
    }
    return CORSConfig{
        AllowedOrigins: strings.Split(origins, ","),
        AllowedHeaders: []string{"Content-Type", "Authorization", "X-Request-Id"},
        MaxAge:         600,
    }
}
```

```bash
# production
CORS_ALLOWED_ORIGINS=https://app.example.com,https://admin.example.com

# staging
CORS_ALLOWED_ORIGINS=https://staging.example.com,http://localhost:3000
```
