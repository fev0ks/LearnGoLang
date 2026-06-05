# GitHub Actions: Docker и реестры образов

## Содержание

- [Buildx: почему обязателен](#buildx-почему-обязателен)
- [GitHub Container Registry (GHCR)](#github-container-registry-ghcr)
- [Docker Hub](#docker-hub)
- [Теги и метаданные образа](#теги-и-метаданные-образа)
- [Multi-platform builds](#multi-platform-builds)
- [Build secrets (не путать с GHA secrets)](#build-secrets-не-путать-с-gha-secrets)
- [Минималистичный Dockerfile для Go](#минималистичный-dockerfile-для-go)
- [Полный пример: build + push + deploy digest](#полный-пример-build--push--deploy-digest)

---

## Buildx: почему обязателен

`docker/setup-buildx-action` настраивает BuildKit — расширенный движок сборки Docker.

Что даёт BuildKit:
- Параллельная сборка независимых слоёв.
- Inline cache (`--cache-from`, `--cache-to`).
- Multi-platform builds (`--platform linux/amd64,linux/arm64`).
- Build secrets (не попадают в историю слоёв).
- `COPY --link` — быстрое копирование без перестройки слоёв.

```yaml
- name: Set up Docker Buildx
  uses: docker/setup-buildx-action@v3
  with:
    driver-opts: |
      image=moby/buildkit:latest
      network=host             # ускоряет если runner в той же сети
```

---

## GitHub Container Registry (GHCR)

GHCR (`ghcr.io`) — встроенный реестр GitHub. Аутентификация через `GITHUB_TOKEN`.

```yaml
permissions:
  packages: write    # нужно для push в GHCR

jobs:
  build:
    steps:
      - name: Log in to GHCR
        uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}   # не нужен отдельный secret

      - name: Build and push
        uses: docker/build-push-action@v6
        with:
          push: true
          tags: ghcr.io/${{ github.repository }}/app:${{ github.sha }}
```

Имя образа: `ghcr.io/<owner>/<repo>/<image>:<tag>`.

По умолчанию образ приватный если репо приватное. Сделать публичным: GitHub → Packages → Change visibility.

---

## Docker Hub

```yaml
- name: Log in to Docker Hub
  uses: docker/login-action@v3
  with:
    username: ${{ secrets.DOCKERHUB_USERNAME }}
    password: ${{ secrets.DOCKERHUB_TOKEN }}     # токен, не пароль
```

Docker Hub: создать Access Token в настройках аккаунта (Settings → Security → Access Tokens). Использовать токен с минимальными правами (Read & Write для CI).

---

## Теги и метаданные образа

`docker/metadata-action` автоматически генерирует теги по стандарту и добавляет OCI labels.

```yaml
- name: Extract metadata
  id: meta
  uses: docker/metadata-action@v5
  with:
    images: |
      ghcr.io/${{ github.repository }}/app
    tags: |
      # тег по commit SHA (иммутабельный, всегда)
      type=sha,prefix=,format=long

      # тег "main-latest" при push в main
      type=raw,value=main-latest,enable=${{ github.ref == 'refs/heads/main' }}

      # тег по версии при push тега v1.2.3
      type=semver,pattern={{version}}
      type=semver,pattern={{major}}.{{minor}}

      # тег по имени ветки
      type=ref,event=branch

      # тег pr-123 для PR
      type=ref,event=pr

- name: Build and push
  uses: docker/build-push-action@v6
  with:
    tags: ${{ steps.meta.outputs.tags }}
    labels: ${{ steps.meta.outputs.labels }}
```

Пример сгенерированных тегов для push в main с тегом `v1.2.3`:
```
ghcr.io/org/repo/app:1af3bc2d...   ← SHA
ghcr.io/org/repo/app:main-latest
ghcr.io/org/repo/app:1.2.3
ghcr.io/org/repo/app:1.2
```

**Важно**: деплоить всегда по SHA/digest, не по `main-latest`. Тег изменяется — digest нет.

---

## Multi-platform builds

Для деплоя на ARM (Graviton, Apple Silicon):

```yaml
- name: Set up QEMU
  uses: docker/setup-qemu-action@v3     # эмуляция ARM на x86 runner

- name: Set up Docker Buildx
  uses: docker/setup-buildx-action@v3

- name: Build and push
  uses: docker/build-push-action@v6
  with:
    platforms: linux/amd64,linux/arm64
    push: true
    tags: ${{ steps.meta.outputs.tags }}
    cache-from: type=gha,scope=app-amd64
    # для multi-platform нужен отдельный scope per платформа
```

Для Go: убедиться что Dockerfile использует `BUILDPLATFORM` и `TARGETOS`/`TARGETARCH`:

```dockerfile
FROM --platform=$BUILDPLATFORM golang:1.22 AS builder

ARG TARGETOS
ARG TARGETARCH

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -o /app ./cmd/app
```

---

## Build secrets (не путать с GHA secrets)

BuildKit поддерживает секреты при сборке — они не попадают в слои образа.

```yaml
- name: Build with secret
  uses: docker/build-push-action@v6
  with:
    secrets: |
      github_token=${{ secrets.GITHUB_TOKEN }}
    secret-envs: |
      NPM_TOKEN
```

```dockerfile
# Dockerfile
RUN --mount=type=secret,id=github_token \
    GITHUB_TOKEN=$(cat /run/secrets/github_token) \
    go mod download
```

Секрет доступен только во время `RUN` шага и не записывается в layer.

---

## Минималистичный Dockerfile для Go

```dockerfile
# Многоэтапная сборка: builder + minimal runtime

# ── Builder ──────────────────────────────────────────────────────────
FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS builder

ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG VERSION=dev

WORKDIR /src

# Сначала зависимости (кеш слой)
COPY go.mod go.sum ./
RUN go mod download

# Потом исходники
COPY . .

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -trimpath \
    -o /bin/app \
    ./cmd/app

# ── Runtime ───────────────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot
# Альтернатива: FROM scratch (если нет runtime зависимостей)
# Альтернатива: FROM alpine:3.20 (если нужен shell для debugging)

COPY --from=builder /bin/app /app

USER nonroot:nonroot
EXPOSE 8080

ENTRYPOINT ["/app"]
```

Размер образа:
- `golang:1.22` → ~800MB (только для сборки)
- `distroless/static:nonroot` → ~2MB runtime
- Итоговый образ: ~10-15MB

`-ldflags="-s -w"` — убрать debug info и symbol table → меньше бинарь.
`-trimpath` — убрать пути файловой системы из бинаря → reproducible builds.

---

## Полный пример: build + push + deploy digest

```yaml
name: build-and-deploy

on:
  push:
    branches: [main]

env:
  REGISTRY: ghcr.io
  IMAGE: ghcr.io/${{ github.repository }}/app

permissions:
  contents: read
  packages: write
  id-token: write

jobs:
  build:
    runs-on: ubuntu-latest
    outputs:
      digest: ${{ steps.push.outputs.digest }}

    steps:
      - uses: actions/checkout@v4

      - uses: docker/setup-buildx-action@v3

      - uses: docker/login-action@v3
        with:
          registry: ${{ env.REGISTRY }}
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - id: meta
        uses: docker/metadata-action@v5
        with:
          images: ${{ env.IMAGE }}
          tags: |
            type=sha,prefix=,format=long
            type=raw,value=main-latest

      - id: push
        uses: docker/build-push-action@v6
        with:
          context: .
          push: true
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
          build-args: |
            VERSION=${{ github.sha }}
          cache-from: |
            type=gha,scope=app
            type=registry,ref=${{ env.IMAGE }}:main-latest
          cache-to: type=gha,mode=max,scope=app,ignore-error=true
          provenance: false      # отключить SBOM (уменьшает размер manifest)

      - run: |
          echo "## Image" >> "${GITHUB_STEP_SUMMARY}"
          echo "\`${{ env.IMAGE }}@${{ steps.push.outputs.digest }}\`" >> "${GITHUB_STEP_SUMMARY}"

  deploy:
    needs: build
    runs-on: ubuntu-latest
    environment: production

    steps:
      - uses: actions/checkout@v4

      # Деплой по иммутабельному digest
      - run: |
          ./scripts/deploy.sh \
            --image "${{ env.IMAGE }}@${{ needs.build.outputs.digest }}"
```
