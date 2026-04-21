# Docker

Практическое использование Docker для Go-сервисов: не только как собрать image, но и как понимать runtime, изоляцию, сигналы и настройку Go runtime под контейнерное окружение.

## Материалы

- [Container vs Virtual Machine](./container-vs-virtual-machine.md) — Linux namespaces и cgroups под капотом, OCI, Kata/gVisor
- [Docker For Go Services](./docker-for-go-services.md) — multi-stage Dockerfile, layer caching, PID 1 / SIGTERM, GOMEMLIMIT, GOMAXPROCS, security

## Что важно уметь объяснить

- Контейнер — это Linux-процесс с namespaces (изоляция видимости) и cgroups (ограничение ресурсов). Ядро хоста shared.
- Почему `ENTRYPOINT ["/app/server"]` (exec form), а не `ENTRYPOINT /app/server` (shell form) — PID 1 и SIGTERM.
- Почему без `GOMEMLIMIT` контейнер получает OOMKilled, а без `automaxprocs` — CPU throttling.
- Multi-stage Dockerfile: builder (go toolchain) → runtime (distroless/scratch). Размер: 10–20 MB.
- Порядок слоёв для эффективного кэша: `go.mod`/`go.sum` → `go mod download` → исходники.
- Секреты нельзя класть в Dockerfile ENV или COPY — они попадают в image history.
