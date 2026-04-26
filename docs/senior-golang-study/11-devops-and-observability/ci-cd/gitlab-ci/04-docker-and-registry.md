# GitLab CI: Docker и Container Registry

## Содержание

- [GitLab Container Registry](#gitlab-container-registry)
- [Docker-in-Docker (DinD)](#docker-in-docker-dind)
- [Kaniko: сборка без Docker daemon](#kaniko-сборка-без-docker-daemon)
- [BuildKit без DinD (buildx + buildkitd)](#buildkit-без-dind-buildx--buildkitd)
- [Кеш слоёв Docker](#кеш-слоёв-docker)
- [Когда что выбирать](#когда-что-выбирать)

---

## GitLab Container Registry

GitLab предоставляет встроенный Container Registry для каждого проекта.

Встроенные переменные:
- `CI_REGISTRY` — адрес реестра (например, `registry.gitlab.com`)
- `CI_REGISTRY_IMAGE` — путь к образу проекта (`registry.gitlab.com/group/project`)
- `CI_REGISTRY_USER` — username для логина
- `CI_REGISTRY_PASSWORD` — пароль (CI_JOB_TOKEN или personal access token)

```yaml
build-image:
  image: docker:26
  services:
    - docker:26-dind
  variables:
    DOCKER_TLS_CERTDIR: "/certs"
  before_script:
    - docker login -u "${CI_REGISTRY_USER}" -p "${CI_REGISTRY_PASSWORD}" "${CI_REGISTRY}"
  script:
    - docker build -t "${CI_REGISTRY_IMAGE}/app:${CI_COMMIT_SHA}" .
    - docker push "${CI_REGISTRY_IMAGE}/app:${CI_COMMIT_SHA}"
```

Именование образов:
```
registry.gitlab.com/group/project/app:sha256abc
registry.gitlab.com/group/project/app:main-latest
registry.gitlab.com/group/project/worker:sha256abc
```

---

## Docker-in-Docker (DinD)

DinD запускает Docker daemon как service внутри CI job. Стандартный подход, поддерживается "из коробки".

```yaml
build-image:
  image: docker:26
  services:
    - name: docker:26-dind
      alias: docker
  variables:
    DOCKER_TLS_CERTDIR: "/certs"    # TLS между client и daemon
    DOCKER_HOST: tcp://docker:2376
    DOCKER_TLS_VERIFY: "1"
    DOCKER_CERT_PATH: "/certs/client"
  before_script:
    - docker login -u "${CI_REGISTRY_USER}" -p "${CI_REGISTRY_PASSWORD}" "${CI_REGISTRY}"
  script:
    - docker build --tag "${CI_REGISTRY_IMAGE}/app:${CI_COMMIT_SHA}" .
    - docker push "${CI_REGISTRY_IMAGE}/app:${CI_COMMIT_SHA}"
```

**Требования к runner**: privileged mode (`privileged = true` в config.toml).

```toml
# /etc/gitlab-runner/config.toml
[[runners]]
  executor = "docker"
  [runners.docker]
    privileged = true     # нужно для DinD
    volumes = ["/certs/client", "/cache"]
```

**Недостатки DinD**:
- Требует `privileged` — security risk на shared runners.
- Нет layer cache между runs (каждый раз с нуля если не настроен external cache).
- Медленнее Kaniko при первой сборке.

---

## Kaniko: сборка без Docker daemon

Kaniko собирает образы внутри контейнера без Docker daemon и без `privileged`. Разработан Google для Kubernetes.

```yaml
build-image:
  image:
    name: gcr.io/kaniko-project/executor:v1.23.0-debug
    entrypoint: [""]
  script:
    - |
      /kaniko/executor \
        --context "${CI_PROJECT_DIR}" \
        --dockerfile "${CI_PROJECT_DIR}/Dockerfile" \
        --destination "${CI_REGISTRY_IMAGE}/app:${CI_COMMIT_SHA}" \
        --destination "${CI_REGISTRY_IMAGE}/app:main-latest" \
        --cache=true \
        --cache-repo "${CI_REGISTRY_IMAGE}/app/cache" \
        --build-arg "VERSION=${CI_COMMIT_SHA}"
  before_script:
    # Создать credentials для GitLab Registry
    - mkdir -p /kaniko/.docker
    - |
      echo "{\"auths\":{\"${CI_REGISTRY}\":{\"auth\":\"$(printf '%s:%s' "${CI_REGISTRY_USER}" "${CI_REGISTRY_PASSWORD}" | base64 | tr -d '\n')\"}}}" \
        > /kaniko/.docker/config.json
```

### Kaniko cache

Kaniko поддерживает кеш слоёв через registry:

```yaml
script:
  - |
    /kaniko/executor \
      --context "${CI_PROJECT_DIR}" \
      --dockerfile "${CI_PROJECT_DIR}/Dockerfile" \
      --destination "${CI_REGISTRY_IMAGE}/app:${CI_COMMIT_SHA}" \
      --cache=true \
      --cache-repo "${CI_REGISTRY_IMAGE}/app/kaniko-cache" \
      --cache-ttl 24h
```

Kaniko пушит каждый слой как отдельный тег в `--cache-repo`. При следующей сборке проверяет есть ли уже скомпилированный слой.

**Преимущества Kaniko**:
- Не нужен `privileged`.
- Работает в обычных Kubernetes pods.
- Встроенный layer cache через registry.

**Недостатки**:
- Медленнее DinD при холодном кеше.
- Нельзя использовать обычные docker команды (только `executor`).
- Debug сложнее.

---

## BuildKit без DinD (buildx + buildkitd)

Альтернатива DinD — запустить BuildKit daemon отдельно и использовать `docker buildx`.

```yaml
build-image:
  image: docker:26
  services:
    - name: moby/buildkit:v0.15.0-rootless
      alias: buildkitd
      command: ["--addr", "tcp://0.0.0.0:1234", "--oci-worker-no-process-sandbox"]
  variables:
    BUILDKIT_HOST: tcp://buildkitd:1234
  before_script:
    - docker login -u "${CI_REGISTRY_USER}" -p "${CI_REGISTRY_PASSWORD}" "${CI_REGISTRY}"
    - docker buildx create --driver remote --name remote-buildkitd "${BUILDKIT_HOST}"
    - docker buildx use remote-buildkitd
  script:
    - |
      docker buildx build \
        --push \
        --platform linux/amd64,linux/arm64 \
        --tag "${CI_REGISTRY_IMAGE}/app:${CI_COMMIT_SHA}" \
        --cache-from type=registry,ref="${CI_REGISTRY_IMAGE}/app:buildcache" \
        --cache-to type=registry,ref="${CI_REGISTRY_IMAGE}/app:buildcache",mode=max \
        .
```

Rootless buildkitd — не нужен `privileged`, но нужен `--oci-worker-no-process-sandbox` в некоторых окружениях.

---

## Кеш слоёв Docker

### Registry cache (работает с DinD и Kaniko)

```yaml
# DinD + BuildKit cache через registry
build-image:
  image: docker:26
  services:
    - docker:26-dind
  variables:
    DOCKER_TLS_CERTDIR: "/certs"
    DOCKER_BUILDKIT: "1"
  script:
    - |
      docker build \
        --build-arg BUILDKIT_INLINE_CACHE=1 \
        --cache-from "${CI_REGISTRY_IMAGE}/app:main-latest" \
        --tag "${CI_REGISTRY_IMAGE}/app:${CI_COMMIT_SHA}" \
        --tag "${CI_REGISTRY_IMAGE}/app:main-latest" \
        .
    # Пушим main-latest последним — он будет cache source для следующей сборки
    - docker push "${CI_REGISTRY_IMAGE}/app:${CI_COMMIT_SHA}"
    - docker push "${CI_REGISTRY_IMAGE}/app:main-latest"
```

`BUILDKIT_INLINE_CACHE=1` — вшивает метаданные кеша в образ, чтобы `--cache-from` работал.

### Оптимизация Dockerfile для кеша

```dockerfile
FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS builder

WORKDIR /src

# Шаг 1: только go.mod и go.sum (меняются редко)
# Этот layer кешируется до изменения зависимостей
COPY go.mod go.sum ./
RUN go mod download

# Шаг 2: исходники (меняются часто)
# Этот layer инвалидируется при каждом коммите
COPY . .

ARG VERSION=dev
ARG TARGETOS=linux
ARG TARGETARCH=amd64

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags="-s -w -X main.version=${VERSION}" \
    -trimpath -o /bin/app ./cmd/app

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /bin/app /app
ENTRYPOINT ["/app"]
```

---

## Когда что выбирать

| Сценарий | Рекомендация |
|---|---|
| Shared GitLab runners | Kaniko (нет privileged) |
| Self-hosted runner, простота | DinD (проще настроить) |
| Multi-platform builds | buildx + buildkitd |
| Kubernetes runner | Kaniko или buildkitd |
| Максимальный cache hit | Kaniko с `--cache-repo` в Registry |
| CI без интернета | DinD с local registry |

### Сравнение

| | DinD | Kaniko | buildkitd |
|---|---|---|---|
| Privileged нужен | Да | Нет | Нет (rootless) |
| Layer cache | Через registry (`BUILDKIT_INLINE_CACHE`) | Встроенный через registry | Через registry |
| Multi-platform | Через buildx | Нет (только один target) | Да |
| Сложность настройки | Низкая | Средняя | Высокая |
| Скорость (с кешем) | Высокая | Высокая | Высокая |
