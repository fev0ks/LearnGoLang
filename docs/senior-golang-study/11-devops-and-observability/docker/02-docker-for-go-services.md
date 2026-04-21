# Docker For Go Services

Практическое руководство: от Dockerfile до production-ready контейнера. Go имеет особенности, которые делают контейнеризацию проще или сложнее, чем в других языках.

## Содержание

- [Multi-stage Dockerfile](#multi-stage-dockerfile)
- [scratch vs distroless vs alpine](#scratch-vs-distroless-vs-alpine)
- [Layer caching: правильный порядок слоёв](#layer-caching-правильный-порядок-слоёв)
- [PID 1 проблема и SIGTERM](#pid-1-проблема-и-sigterm)
- [GOMEMLIMIT и GOMAXPROCS в контейнере](#gomemlimit-и-gomaxprocs-в-контейнере)
- [.dockerignore](#dockerignore)
- [Docker Networks и service discovery](#docker-networks-и-service-discovery)
- [docker-compose для локальной разработки](#docker-compose-для-локальной-разработки)
- [Безопасность: non-root, read-only, no secrets](#безопасность-non-root-read-only-no-secrets)
- [Health check в Dockerfile](#health-check-в-dockerfile)
- [Типичные ошибки](#типичные-ошибки)
- [Interview-ready answer](#interview-ready-answer)

## Multi-stage Dockerfile

Go компилируется в статический бинарь. Не нужно тащить Go toolchain в production image.

```dockerfile
# ── Стадия 1: сборка ──────────────────────────────────────────────
FROM golang:1.23-alpine AS builder

WORKDIR /build

# Сначала копируем только go.mod и go.sum — для layer cache.
# Если исходники изменились, но зависимости нет — этот слой берётся из кэша.
COPY go.mod go.sum ./
RUN go mod download

# Теперь исходники
COPY . .

# Статический бинарь без CGO.
# -trimpath: убирает локальные пути из бинаря (security, reproducibility).
# -ldflags: -w убирает DWARF, -s убирает symbol table → меньше бинарь.
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath \
    -ldflags="-w -s -X main.version=${VERSION}" \
    -o server ./cmd/server

# ── Стадия 2: runtime ─────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

# Копируем только бинарь
COPY --from=builder /build/server /app/server

# Если нужны TLS CA certs (для исходящих HTTPS вызовов)
# distroless/static уже содержит ca-certificates

EXPOSE 8080

# Exec form обязателен (не shell form) — для правильного SIGTERM
ENTRYPOINT ["/app/server"]
```

Результат:
- Image `gcr.io/distroless/static-debian12`: ~2 MB base.
- Финальный image: ~10–20 MB (бинарь + base).
- Vs `golang:1.23-alpine`: 200+ MB.

## scratch vs distroless vs alpine

| Base image | Размер | Shell | CA certs | Отладка | Когда |
|---|---|---|---|---|---|
| `scratch` | 0 bytes | нет | нет | невозможна | Полностью статический бинарь, не делает HTTPS |
| `distroless/static` | ~2 MB | нет | есть | только через `debug` variant | Production Go без CGO |
| `distroless/base` | ~20 MB | нет | есть | только `debug` variant | Production Go с CGO |
| `alpine` | ~5 MB | sh | нет (add вручную) | есть | Dev, debugging, нужен shell |
| `ubuntu`/`debian` | 50–100 MB | bash | есть | полная | Когда нужны инструменты, CGO, libc |

**scratch**: абсолютный минимум. Но без `/etc/ssl/certs` — HTTPS звонки упадут с `certificate signed by unknown authority`. Нужно копировать certs из builder:
```dockerfile
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
```

**distroless** (Google): нет shell, нет package manager, нет лишних бинарей. Меньше attack surface. Рекомендуется для production.

**alpine**: удобен в разработке (`apk add curl`). В production — лишний shell = лишний attack vector.

## Layer caching: правильный порядок слоёв

Docker строит image слой за слоем. Если слой не изменился — берётся из кэша. Кэш инвалидируется при изменении слоя и всех последующих.

**Плохой порядок** (кэш никогда не работает):
```dockerfile
# Копируем всё сразу
COPY . .
# При любом изменении кода — re-download всех зависимостей
RUN go mod download
RUN go build ...
```

**Правильный порядок** (зависимости кэшируются):
```dockerfile
# 1. go.mod + go.sum изменяются редко
COPY go.mod go.sum ./
RUN go mod download  # ← кэшируется пока не изменится go.mod

# 2. Исходники изменяются часто
COPY . .
RUN go build ...     # ← пересобираем только при изменении кода
```

Аналогично для npm/Python — сначала lock-файл, потом исходники.

**BuildKit** (включён по умолчанию в Docker 23+) поддерживает монтирование кэша для `go mod`:
```dockerfile
RUN --mount=type=cache,target=/root/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -o server ./cmd/server
```
Это кэширует go module cache и build cache между сборками на одной машине.

## PID 1 проблема и SIGTERM

Это критично для graceful shutdown. Первый процесс в контейнере получает PID 1.

**Проблема shell form**:
```dockerfile
# Shell form — НЕПРАВИЛЬНО для Go-сервиса
ENTRYPOINT /app/server
# или
CMD /app/server
```
Shell form запускает: `sh -c "/app/server"`. Процесс дерева: `sh (PID 1)` → `server (PID N)`.

При `docker stop` → контейнер получает **SIGTERM на PID 1 = sh**. Shell получает сигнал, убивает дочерние процессы (`SIGKILL`), не давая им времени на graceful shutdown.

**Решение: exec form**:
```dockerfile
# Exec form — ПРАВИЛЬНО
ENTRYPOINT ["/app/server"]
```
Exec form запускает бинарь напрямую, он становится PID 1 и получает SIGTERM сам.

**Ещё проблема с PID 1**: по умолчанию PID 1 не reap zombie-процессы. Если сервис fork'ает child-процессы (CGO, os/exec), они могут стать зомби. Решение: использовать `tini` как init:
```dockerfile
FROM ghcr.io/nicholasgasior/tini:latest AS tini

FROM distroless/static:nonroot
COPY --from=tini /tini /tini
COPY --from=builder /build/server /app/server
ENTRYPOINT ["/tini", "--", "/app/server"]
```

Для чистых Go сервисов без fork — не нужен.

**Go сторона** — правильная обработка SIGTERM:
```go
func main() {
    ctx, stop := signal.NotifyContext(context.Background(),
        syscall.SIGTERM, syscall.SIGINT)
    defer stop()

    srv := &http.Server{Addr: ":8080", Handler: mux}
    go srv.ListenAndServe()

    <-ctx.Done()
    log.Info("shutting down")

    // grace period для in-flight запросов
    shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    srv.Shutdown(shutdownCtx)
}
```

## GOMEMLIMIT и GOMAXPROCS в контейнере

Go runtime не знает о cgroup лимитах по умолчанию. Это приводит к двум проблемам:

### GOMAXPROCS

Go по умолчанию ставит `GOMAXPROCS = число CPU на хосте`. Если контейнер ограничен 0.25 CPU на ноде с 32 CPU — Go создаст 32 OS-потока. Планировщик будет перекладывать горутины между потоками, cgroup будет троттлить CPU — latency спайки.

Решение: `automaxprocs` — автоматически читает cgroup и ставит GOMAXPROCS:
```go
import _ "go.uber.org/automaxprocs"

func main() {
    // automaxprocs запустился в init(), GOMAXPROCS = ceil(CPU limit)
    // При --cpus=0.5 → GOMAXPROCS=1, при --cpus=2 → GOMAXPROCS=2
}
```

### GOMEMLIMIT

Go GC не знает о memory limit контейнера. Если лимит 512MB, а GC решит не собирать мусор (heap ещё не достиг GOGC threshold) — OOM killer придёт раньше GC.

Решение: `GOMEMLIMIT` ставит мягкий лимит на heap, GC становится агрессивнее:
```dockerfile
# В Dockerfile или docker-compose
ENV GOMEMLIMIT=450MiB   # чуть ниже cgroup limit (512MB)
```

Или в Go коде:
```go
import "runtime/debug"

func main() {
    // Взять 90% от cgroup memory limit
    if limit := cgroupMemoryLimit(); limit > 0 {
        debug.SetMemoryLimit(limit * 9 / 10)
    }
}
```

С Go 1.21+ `GOMEMLIMIT` автоматически учитывается в некоторых инструментах, но явно лучше.

## .dockerignore

Без `.dockerignore` контекст сборки включает `.git`, `vendor`, локальные артефакты — замедляет передачу контекста в Docker daemon.

```text
# .dockerignore
.git
.github
*.md
*.test
vendor/          # если не используется vendor mode
.env
.env.*
*.local
tmp/
dist/
coverage/
```

## Docker Networks и service discovery

При запуске контейнеров в одной сети Docker создаёт DNS-записи по имени сервиса:

```yaml
# docker-compose.yml
services:
  api:
    build: .
    networks:
      - backend

  postgres:
    image: postgres:16
    networks:
      - backend

networks:
  backend:
```

Внутри `api` контейнера: `postgres:5432` резолвится в IP контейнера postgres. Это внутренний DNS Docker (встроен в Docker Engine).

Типы Docker networks:
- **bridge** (default): изолированная сеть, containers в одной bridge могут общаться по имени.
- **host**: контейнер использует сеть хоста напрямую. Производительнее, но нет изоляции портов.
- **overlay**: для Docker Swarm, multi-host networking.
- **none**: полная сетевая изоляция.

```bash
# Посмотреть сети
docker network ls

# Inspect что в сети
docker network inspect bridge
```

## docker-compose для локальной разработки

```yaml
# docker-compose.yml
version: "3.9"

services:
  api:
    build:
      context: .
      dockerfile: Dockerfile
      target: builder     # использовать builder стадию для dev (с Go tools)
    ports:
      - "8080:8080"
    environment:
      - DATABASE_URL=postgres://postgres:postgres@postgres:5432/app?sslmode=disable
      - REDIS_ADDR=redis:6379
      - LOG_LEVEL=debug
    volumes:
      - .:/app           # live reload для dev (с air или CompileDaemon)
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_started

  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: app
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: postgres
    volumes:
      - postgres_data:/var/lib/postgresql/data
      - ./migrations:/docker-entrypoint-initdb.d  # apply migrations on start
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U postgres"]
      interval: 5s
      timeout: 5s
      retries: 5

  redis:
    image: redis:7-alpine
    command: redis-server --maxmemory 128mb --maxmemory-policy allkeys-lru

volumes:
  postgres_data:
```

Команды:
```bash
docker compose up -d          # запустить в фоне
docker compose logs -f api    # tail логи сервиса
docker compose exec api bash  # shell в контейнере (alpine)
docker compose down -v        # остановить и удалить volumes
```

**Live reload** в контейнере с `air`:
```bash
# В Dockerfile builder стадии или отдельном dev Dockerfile
RUN go install github.com/cosmtrek/air@latest
ENTRYPOINT ["air"]
```

## Безопасность: non-root, read-only, no secrets

### Non-root user

По умолчанию контейнер запускается как root (UID 0). Если процесс компрометирован — он имеет root привилегии внутри контейнера.

```dockerfile
# distroless/nonroot уже использует UID 65532
FROM gcr.io/distroless/static-debian12:nonroot

# Или явно в Dockerfile:
RUN addgroup -S appgroup && adduser -S appuser -G appgroup
USER appuser:appgroup
```

### Read-only filesystem

```bash
docker run --read-only -v /tmp:/tmp my-go-service
# Только /tmp writable, всё остальное — read-only
```

В Kubernetes:
```yaml
securityContext:
  readOnlyRootFilesystem: true
```

### Секреты не в image

**Никогда** не класть секреты в Dockerfile ENV или COPY:
```dockerfile
# ПЛОХО — секрет попадёт в image layer history
ENV DATABASE_PASSWORD=secret123

# ПЛОХО — файл с секретами запечётся в layer
COPY .env /app/.env
```

Правильно: секреты передавать через runtime:
```bash
# Через env при запуске
docker run -e DATABASE_PASSWORD=$(vault kv get -field=password secret/db) my-service

# Через Docker secrets (Swarm)
docker service create --secret db_password my-service

# В Kubernetes — через Secret + env injection
```

### Сканирование образов

```bash
# trivy — популярный scanner уязвимостей
trivy image my-go-service:latest

# docker scout (встроен в Docker)
docker scout cves my-go-service:latest
```

## Health check в Dockerfile

```dockerfile
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD ["/app/server", "-healthcheck"]
# или через wget/curl если есть в image:
# CMD wget -qO- http://localhost:8080/healthz || exit 1
```

В Go — отдельный `/healthz` эндпоинт без бизнес-логики:
```go
mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
})

// readiness — может включать проверку DB
mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
    if err := db.PingContext(r.Context()); err != nil {
        http.Error(w, "db not ready", http.StatusServiceUnavailable)
        return
    }
    w.WriteHeader(http.StatusOK)
})
```

## Типичные ошибки

- **Shell form ENTRYPOINT**: `ENTRYPOINT /app/server` вместо `ENTRYPOINT ["/app/server"]` — SIGTERM не доходит до Go процесса.
- **Нет GOMEMLIMIT**: Go GC не знает о cgroup memory limit → OOMKilled.
- **Нет automaxprocs**: GOMAXPROCS = 32 ядра хоста при ограничении 0.25 CPU → CPU throttling и latency spikes.
- **Секреты в ENV в Dockerfile**: попадают в image history (`docker history --no-trunc`).
- **Зависимости не кэшируются**: `COPY . .` перед `go mod download` → пересобираем зависимости при каждом изменении кода.
- **Root user в production**: нарушение принципа least privilege.
- **Нет `.dockerignore`**: `.git` и `vendor` попадают в build context → медленная сборка.
- **Один большой слой**: все `RUN` команды в один layer через `&&` — правильно для минимизации size, но теряем кэш при изменении любой команды.

## Interview-ready answer

Go статически компилируется, поэтому production image — multi-stage: builder с go toolchain, runtime со `scratch` или `distroless` (2–20 MB). Порядок слоёв: `go.mod`/`go.sum` → `go mod download` → исходники → build — кэш зависимостей работает при изменении только кода. ENTRYPOINT обязан быть в exec form (`["/app/server"]`), иначе PID 1 = sh, SIGTERM не доходит до Go процесса. Без GOMEMLIMIT Go GC не знает о cgroup лимите → OOMKilled. Без automaxprocs GOMAXPROCS = число CPU хоста → CPU throttling. Секреты — только через runtime (env inject, Kubernetes Secret), никогда в Dockerfile.
