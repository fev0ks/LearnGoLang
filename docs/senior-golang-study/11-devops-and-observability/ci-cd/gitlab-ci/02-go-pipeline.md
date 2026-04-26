# GitLab CI: Go Pipeline

Готовый `.gitlab-ci.yml` для Go-проекта: от PR-проверки до деплоя.

## Содержание

- [Полный pipeline](#полный-pipeline)
- [MR Pipeline vs Main Pipeline](#mr-pipeline-vs-main-pipeline)
- [Разбор ключевых решений](#разбор-ключевых-решений)

---

## Полный pipeline

```yaml
# .gitlab-ci.yml

# ── Глобальные настройки ─────────────────────────────────────────────
default:
  image: golang:1.22-alpine
  interruptible: true

variables:
  CGO_ENABLED: "0"
  GOFLAGS: "-mod=readonly"
  # Кладём GOPATH и кеш сборки внутрь проекта — так работает cache в GitLab
  GOPATH: "${CI_PROJECT_DIR}/.cache/go"
  GOCACHE: "${CI_PROJECT_DIR}/.cache/go/build"
  # Registry
  IMAGE: "${CI_REGISTRY_IMAGE}/app"

stages:
  - validate
  - test
  - build
  - deploy

# ── Шаблоны ──────────────────────────────────────────────────────────
.go-base:
  before_script:
    - mkdir -p "${GOPATH}/pkg/mod" "${GOCACHE}"
  cache:
    key:
      files:
        - go.sum
    paths:
      - .cache/go/pkg/mod
      - .cache/go/build
    policy: pull-push

.mr-or-main:
  rules:
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
    - if: $CI_COMMIT_REF_NAME == $CI_DEFAULT_BRANCH

.main-only:
  rules:
    - if: $CI_COMMIT_REF_NAME == $CI_DEFAULT_BRANCH
      when: on_success
    - when: never

# ── VALIDATE ─────────────────────────────────────────────────────────

fmt:
  extends:
    - .go-base
    - .mr-or-main
  stage: validate
  script:
    - |
      unformatted=$(gofmt -l ./...)
      if [[ -n "$unformatted" ]]; then
        echo "Unformatted files:"
        echo "$unformatted"
        exit 1
      fi
      echo "All files formatted correctly."

lint:
  extends:
    - .go-base
    - .mr-or-main
  stage: validate
  image: golangci/golangci-lint:v1.59-alpine
  script:
    - golangci-lint run --timeout=5m --out-format=colored-line-number

# ── TEST ─────────────────────────────────────────────────────────────

test:
  extends:
    - .go-base
    - .mr-or-main
  stage: test
  script:
    - go test -race -coverprofile=coverage.out -covermode=atomic ./...
    - go tool cover -func=coverage.out | tail -1
    # Конвертировать в cobertura для GitLab coverage visualization
    - go install github.com/boumenot/gocover-cobertura@latest
    - "${GOPATH}/bin/gocover-cobertura" < coverage.out > coverage.xml
  coverage: '/total:\s+\(statements\)\s+(\d+\.\d+)%/'
  artifacts:
    when: always
    reports:
      coverage_report:
        coverage_format: cobertura
        path: coverage.xml
      junit: test-results.xml    # если используется gotestsum
    paths:
      - coverage.out
    expire_in: 1 week

# Опционально: запуск интеграционных тестов отдельным job
integration-test:
  extends:
    - .go-base
    - .mr-or-main
  stage: test
  services:
    - name: postgres:16-alpine
      alias: postgres
  variables:
    POSTGRES_DB: testdb
    POSTGRES_USER: test
    POSTGRES_PASSWORD: test
    DATABASE_URL: "postgres://test:test@postgres:5432/testdb?sslmode=disable"
  script:
    - go test -tags=integration ./...

# ── BUILD ─────────────────────────────────────────────────────────────

build-binary:
  extends:
    - .go-base
    - .mr-or-main
  stage: build
  script:
    - |
      go build \
        -ldflags="-s -w -X main.version=${CI_COMMIT_SHORT_SHA}" \
        -trimpath \
        -o bin/app \
        ./cmd/app
  artifacts:
    paths:
      - bin/app
    expire_in: 1 day

build-image:
  extends: .main-only
  stage: build
  image: docker:26
  services:
    - docker:26-dind
  variables:
    DOCKER_TLS_CERTDIR: "/certs"
    DOCKER_BUILDKIT: "1"
  before_script:
    - docker login -u "${CI_REGISTRY_USER}" -p "${CI_REGISTRY_PASSWORD}" "${CI_REGISTRY}"
  script:
    - |
      docker build \
        --build-arg VERSION="${CI_COMMIT_SHA}" \
        --cache-from "${IMAGE}:main-latest" \
        --tag "${IMAGE}:${CI_COMMIT_SHA}" \
        --tag "${IMAGE}:main-latest" \
        .
    - docker push "${IMAGE}:${CI_COMMIT_SHA}"
    - docker push "${IMAGE}:main-latest"
    # Сохранить digest для деплоя
    - docker inspect --format='{{index .RepoDigests 0}}' "${IMAGE}:${CI_COMMIT_SHA}" > image-digest.txt
  artifacts:
    paths:
      - image-digest.txt
    expire_in: 1 week

# ── DEPLOY ────────────────────────────────────────────────────────────

deploy-dev:
  extends: .main-only
  stage: deploy
  image: alpine:3.20
  needs:
    - job: build-image
      artifacts: true    # нужен image-digest.txt
  environment:
    name: dev
    url: https://dev.myapp.com
  script:
    - IMAGE_DIGEST=$(cat image-digest.txt)
    - ./scripts/deploy.sh --env dev --image "${IMAGE_DIGEST}"

deploy-staging:
  extends: .main-only
  stage: deploy
  image: alpine:3.20
  needs:
    - job: deploy-dev    # ждём деплой в dev
    - job: build-image
      artifacts: true
  environment:
    name: staging
    url: https://staging.myapp.com
  when: manual           # вручную из UI
  script:
    - IMAGE_DIGEST=$(cat image-digest.txt)
    - ./scripts/deploy.sh --env staging --image "${IMAGE_DIGEST}"

deploy-prod:
  stage: deploy
  image: alpine:3.20
  needs:
    - job: deploy-staging
    - job: build-image
      artifacts: true
  environment:
    name: production
    url: https://myapp.com
  rules:
    - if: $CI_COMMIT_REF_NAME == $CI_DEFAULT_BRANCH
      when: manual
      allow_failure: false
    - when: never
  script:
    - IMAGE_DIGEST=$(cat image-digest.txt)
    - ./scripts/deploy.sh --env prod --image "${IMAGE_DIGEST}"

# ── SECURITY ─────────────────────────────────────────────────────────

govulncheck:
  extends:
    - .go-base
    - .mr-or-main
  stage: validate
  script:
    - go install golang.org/x/vuln/cmd/govulncheck@latest
    - "${GOPATH}/bin/govulncheck" ./...
  allow_failure: true    # не блокировать MR, но видеть результат

# ── RELEASE ───────────────────────────────────────────────────────────

release:
  stage: deploy
  image: registry.gitlab.com/gitlab-org/release-cli:latest
  needs:
    - job: deploy-prod
  rules:
    - if: $CI_COMMIT_TAG =~ /^v\d+\.\d+\.\d+$/
  script:
    - echo "Creating release for ${CI_COMMIT_TAG}"
  release:
    tag_name: $CI_COMMIT_TAG
    description: "Release ${CI_COMMIT_TAG}"
    assets:
      links:
        - name: "Docker image"
          url: "https://${CI_REGISTRY_IMAGE}/app:${CI_COMMIT_SHA}"
```

---

## MR Pipeline vs Main Pipeline

GitLab поддерживает два режима пайплайнов для merge requests:

### Branch pipeline (push в ветку MR)

Запускается при push в ветку. Обычный `CI_PIPELINE_SOURCE == "push"`. Используется по умолчанию если не настроен merge request pipeline.

### Merge request pipeline (detached pipeline)

Запускается специально для MR. `CI_PIPELINE_SOURCE == "merge_request_event"`. Работает на "merged result" (как будто ветка уже смержена).

Настроить в GitLab: Settings → Merge requests → Pipelines for merge requests.

```yaml
# Рекомендуется: явно различать источник
.for-mr:
  rules:
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"

.for-main:
  rules:
    - if: $CI_COMMIT_REF_NAME == $CI_DEFAULT_BRANCH
      when: on_success
    - when: never

# Не запускать дважды (и в branch pipeline, и в MR pipeline)
workflow:
  rules:
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
    - if: $CI_COMMIT_REF_NAME == $CI_DEFAULT_BRANCH
    - if: $CI_COMMIT_TAG
    - when: never    # пропустить обычные branch pushes если есть open MR
```

---

## Разбор ключевых решений

**GOPATH внутри проекта** — `${CI_PROJECT_DIR}/.cache/go`. GitLab cache работает с путями относительно project dir. Если GOPATH вне проекта (`~/go`) — cache не работает.

**`policy: pull-push`** (default) — восстанавливает И сохраняет кеш. Для read-only jobs (deploy) использовать `policy: pull`.

**`interruptible: true`** — pipeline можно прервать при новом push в ту же ветку. Без этого старый pipeline продолжит работать впустую.

**`needs` + `artifacts: true`** — явная передача артефактов между jobs из разных stages без ожидания всего stage.

**`CI_REGISTRY_IMAGE`** — автоматически указывает на GitLab Container Registry текущего проекта. Не нужно задавать вручную.

**Docker-in-Docker (`services: docker:dind`)** — стандартный способ сборки образов в GitLab. Альтернатива без привилегий — Kaniko (см. [04-docker-and-registry.md](./04-docker-and-registry.md)).

**`allow_failure: true` для govulncheck** — security scan виден в pipeline, но не блокирует MR. Со временем перевести в `false` когда команда разберётся с alertами.
