# GitLab CI: синтаксис и концепции

## Содержание

- [Структура .gitlab-ci.yml](#структура-gitlab-ciyml)
- [Stages и jobs](#stages-и-jobs)
- [Variables](#variables)
- [Rules: когда запускать job](#rules-когда-запускать-job)
- [needs vs dependencies vs stages](#needs-vs-dependencies-vs-stages)
- [extends и !reference](#extends-и-reference)
- [include: шаблоны и внешние файлы](#include-шаблоны-и-внешние-файлы)
- [Runners: shared vs specific](#runners-shared-vs-specific)
- [Artifacts и cache](#artifacts-и-cache)

---

## Структура .gitlab-ci.yml

```yaml
# ── Глобальные настройки ─────────────────────────────────────────────
default:
  image: golang:1.22-alpine        # образ по умолчанию для всех jobs
  tags:
    - docker                       # runner tag
  interruptible: true              # можно прерывать при новом push
  timeout: 30 minutes

variables:
  GO_VERSION: "1.22"
  CGO_ENABLED: "0"
  GOFLAGS: "-mod=readonly"

# ── Порядок stages ──────────────────────────────────────────────────
stages:
  - validate    # fmt, lint
  - test
  - build
  - deploy-dev
  - deploy-prod

# ── Шаблоны ────────────────────────────────────────────────────────
.go-cache: &go-cache               # YAML anchor — переиспользуемый блок
  cache:
    key:
      files:
        - go.sum
    paths:
      - .cache/go/pkg/mod
      - .cache/go/build
    policy: pull-push

# ── Jobs ─────────────────────────────────────────────────────────────
fmt:
  stage: validate
  <<: *go-cache
  script:
    - test -z "$(gofmt -l ./...)"

lint:
  stage: validate
  <<: *go-cache
  script:
    - golangci-lint run --timeout=5m

test:
  stage: test
  <<: *go-cache
  script:
    - go test -race -coverprofile=coverage.txt ./...
  coverage: '/total:\s+\(statements\)\s+(\d+\.\d+)%/'   # regex для coverage badge
  artifacts:
    reports:
      coverage_report:
        coverage_format: cobertura
        path: coverage.xml
    expire_in: 1 week
```

---

## Stages и jobs

Stages выполняются **последовательно**. Jobs внутри одного stage — **параллельно**.

```yaml
stages:
  - build      # все jobs stage build параллельно
  - test       # только после завершения всех build jobs
  - deploy     # только после завершения всех test jobs

build-api:
  stage: build
  script: make build-api

build-worker:
  stage: build    # параллельно с build-api
  script: make build-worker

test-api:
  stage: test     # ждёт build-api AND build-worker
  script: make test-api
```

Job может принадлежать только одному stage.

---

## Variables

```yaml
variables:
  # Глобальные переменные
  REGISTRY: registry.gitlab.com
  IMAGE_NAME: $CI_PROJECT_PATH/app    # CI_ переменные встроены в GitLab

  # Переменные с expand
  IMAGE_TAG: $REGISTRY/$IMAGE_NAME:$CI_COMMIT_SHA

job:
  variables:
    # Локальные переменные (только для этого job)
    GOFLAGS: "-mod=vendor"
  script:
    - echo $IMAGE_TAG
```

### Встроенные переменные GitLab

| Переменная | Значение |
|---|---|
| `CI_COMMIT_SHA` | Полный SHA коммита |
| `CI_COMMIT_SHORT_SHA` | Первые 8 символов SHA |
| `CI_COMMIT_REF_NAME` | Имя ветки или тега |
| `CI_COMMIT_TAG` | Тег (пусто если не тег) |
| `CI_PROJECT_PATH` | `group/repo` |
| `CI_PROJECT_ID` | ID проекта |
| `CI_REGISTRY` | Адрес GitLab Container Registry |
| `CI_REGISTRY_IMAGE` | Адрес образа в GCR |
| `CI_REGISTRY_USER` / `CI_REGISTRY_PASSWORD` | Credentials для GCR |
| `CI_ENVIRONMENT_NAME` | Имя environment (если задан) |
| `CI_PIPELINE_ID` | ID пайплайна |
| `CI_JOB_TOKEN` | Токен для API GitLab |

---

## Rules: когда запускать job

`rules` заменяет устаревшие `only`/`except`. Более гибкий и явный.

```yaml
deploy-prod:
  stage: deploy
  rules:
    # Запускать вручную только из main
    - if: $CI_COMMIT_REF_NAME == "main"
      when: manual
      allow_failure: false    # блокирует pipeline пока не нажали

    # Пропустить для всего остального
    - when: never

build:
  rules:
    # Автоматически для push в main или в MR
    - if: $CI_COMMIT_REF_NAME == "main"
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"

    # Только если изменились Go файлы
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
      changes:
        - "**/*.go"
        - go.sum

    # Вручную в других случаях
    - when: manual
      allow_failure: true
```

### when значения

| Значение | Описание |
|---|---|
| `on_success` | Default: запустить если предыдущие stages успешны |
| `always` | Запустить всегда (включая при падении) |
| `never` | Не запускать |
| `manual` | Только по кнопке в UI |
| `delayed` | Запустить с задержкой (`start_in: 5 minutes`) |

### CI_PIPELINE_SOURCE значения

| Значение | Когда |
|---|---|
| `push` | Обычный push |
| `merge_request_event` | MR pipeline |
| `schedule` | Scheduled pipeline |
| `web` | Запуск из UI |
| `api` | Запуск через API |
| `trigger` | Запуск через trigger token |
| `pipeline` | Запуск из другого pipeline (child/parent) |

---

## needs vs dependencies vs stages

### stages (default): sequential

```
[validate] → [test] → [build] → [deploy]
```

Все jobs одного stage ждут завершения всех jobs предыдущего stage. Простой, но медленный.

### needs: DAG (Directed Acyclic Graph)

```yaml
build-api:
  stage: build
  needs: []    # запустить немедленно, не ждать validate stage

test-api:
  stage: test
  needs:
    - job: build-api
      artifacts: true    # скачать artifacts из build-api

deploy:
  stage: deploy
  needs:
    - test-api
    - test-worker
```

С `needs` jobs из разных stages могут выполняться параллельно:
```
fmt ──────────────────────────────┐
lint ─────────────────────────────┤
                                   ↓
build-api ──→ test-api ──→ deploy
build-worker ──→ test-worker ──┘
```

### dependencies: контроль скачивания artifacts

```yaml
deploy:
  stage: deploy
  dependencies:
    - build-api     # скачать только artifacts из build-api
    # без dependencies — скачиваются artifacts всех jobs выше
    # dependencies: [] — не скачивать ничего
```

`dependencies` только контролирует какие artifacts скачивать, не порядок выполнения (это делает `needs`).

---

## extends и !reference

### extends: наследование конфигурации

```yaml
.base-go-job:
  image: golang:1.22-alpine
  variables:
    CGO_ENABLED: "0"
  before_script:
    - export GOPATH="${CI_PROJECT_DIR}/.cache/go"
    - export GOCACHE="${CI_PROJECT_DIR}/.cache/go/build"

lint:
  extends: .base-go-job    # наследует image, variables, before_script
  stage: validate
  script:
    - golangci-lint run

test:
  extends: .base-go-job
  stage: test
  script:
    - go test ./...
```

Несколько `extends`:
```yaml
job:
  extends:
    - .base-go-job
    - .with-cache
    - .deploy-rules
```

Конфликты решаются: последний выигрывает.

### !reference: точечное заимствование

```yaml
.go-before-script:
  before_script:
    - export GOPATH="${CI_PROJECT_DIR}/.cache/go"

.deploy-rules:
  rules:
    - if: $CI_COMMIT_REF_NAME == "main"

job:
  before_script:
    - !reference [.go-before-script, before_script]
    - echo "additional step"    # добавить к унаследованному
  rules:
    - !reference [.deploy-rules, rules]
```

---

## include: шаблоны и внешние файлы

```yaml
include:
  # Локальный файл в репо
  - local: .gitlab/ci/go-templates.yml

  # Файл из другого проекта GitLab
  - project: mygroup/ci-templates
    ref: main
    file: /templates/go.yml

  # Внешний URL
  - remote: https://example.com/ci-template.yml

  # Встроенные GitLab шаблоны
  - template: Security/SAST.gitlab-ci.yml
  - template: Jobs/Code-Quality.gitlab-ci.yml
```

Пример вынесения шаблонов в `.gitlab/ci/templates.yml`:

```yaml
# .gitlab/ci/templates.yml
.go-job:
  image: golang:1.22-alpine
  variables:
    CGO_ENABLED: "0"
    GOPATH: "${CI_PROJECT_DIR}/.cache/go"
    GOCACHE: "${CI_PROJECT_DIR}/.cache/go/build"
  before_script:
    - mkdir -p "${GOPATH}" "${GOCACHE}"
  cache:
    key:
      files: [go.sum]
    paths:
      - .cache/go/pkg/mod
      - .cache/go/build
```

```yaml
# .gitlab-ci.yml
include:
  - local: .gitlab/ci/templates.yml

lint:
  extends: .go-job
  stage: validate
  script:
    - golangci-lint run
```

---

## Runners: shared vs specific

**Shared runners** (gitlab.com) — доступны всем проектам. Для публичных: бесплатные минуты. Для приватных: платные.

**Group runners** — доступны всем проектам в группе.

**Project-specific runners** — только для одного проекта. Self-hosted.

Выбор runner через `tags`:

```yaml
build:
  tags:
    - docker      # runner с тегом docker (обычно self-hosted с Docker)
    - linux
```

Без `tags` — использует любой доступный runner.

Self-hosted runner регистрируется:
```bash
gitlab-runner register \
  --url https://gitlab.com \
  --registration-token TOKEN \
  --executor docker \
  --docker-image alpine \
  --tag-list "docker,linux,self-hosted"
```

---

## Artifacts и cache

Подробно в [03-caching-and-artifacts.md](./03-caching-and-artifacts.md). Кратко:

**Cache** — ускорение (go modules, npm packages). Не гарантированно восстановится. Не передаётся между jobs.

**Artifacts** — результат работы job. Передаётся другим jobs через `dependencies`/`needs`. Доступен для скачивания из UI. Expire автоматически.

```yaml
build:
  artifacts:
    paths:
      - bin/app
    expire_in: 1 day

deploy:
  needs:
    - job: build
      artifacts: true    # скачать bin/app
  script:
    - ./bin/app --version  # файл доступен
```
