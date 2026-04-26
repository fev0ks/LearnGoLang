# GitLab CI: Кеш и Артефакты

## Содержание

- [Cache vs Artifacts: разница](#cache-vs-artifacts-разница)
- [Cache: конфигурация](#cache-конфигурация)
- [Cache key стратегии](#cache-key-стратегии)
- [Cache policy: pull-push vs pull](#cache-policy-pull-push-vs-pull)
- [Artifacts: конфигурация](#artifacts-конфигурация)
- [Artifacts reports: встроенные типы](#artifacts-reports-встроенные-типы)
- [Передача артефактов между jobs](#передача-артефактов-между-jobs)
- [Антипаттерны](#антипаттерны)

---

## Cache vs Artifacts: разница

| | Cache | Artifacts |
|---|---|---|
| Назначение | Ускорение (зависимости, build cache) | Результат работы (бинари, отчёты) |
| Гарантия восстановления | Нет (best-effort) | Да |
| Передача между jobs | Нет (только внутри branch/pipeline) | Да (через `needs`/`dependencies`) |
| Автоудаление | По `key` (при инвалидации) | По `expire_in` |
| Скачивание из UI | Нет | Да |
| Хранение | GitLab Runner cache (S3 или local) | GitLab server |

**Правило**: если без этих данных следующий job не сможет работать — артефакт. Если это просто ускорение — кеш.

---

## Cache: конфигурация

```yaml
job:
  cache:
    # ── Ключ ────────────────────────────────────────────────────────
    key: "$CI_COMMIT_REF_SLUG"    # по умолчанию: одна строка

    # Ключ по содержимому файлов (инвалидация при изменении go.sum)
    key:
      files:
        - go.sum
        - go.work.sum
      prefix: "go-v1"    # префикс для версионирования кеша

    # ── Пути ────────────────────────────────────────────────────────
    paths:
      - .cache/go/pkg/mod
      - .cache/go/build

    # ── Политика ────────────────────────────────────────────────────
    policy: pull-push    # default

    # ── Поведение при miss ──────────────────────────────────────────
    when: on_success     # default: сохранять только при успехе
    # when: always       # сохранять даже при падении
    # when: on_failure   # сохранять только при падении

    # ── Несколько кешей ─────────────────────────────────────────────
    - key:
        files: [go.sum]
      paths: [.cache/go/pkg/mod]
      policy: pull-push
    - key: "tools-v1-$CI_RUNNER_ID"
      paths: [.cache/tools]
      policy: pull-push
```

### GOPATH внутри проекта

GitLab Runner кеширует пути **относительно project directory**. Стандартный `~/go/pkg/mod` недоступен.

```yaml
variables:
  GOPATH: "${CI_PROJECT_DIR}/.cache/go"
  GOCACHE: "${CI_PROJECT_DIR}/.cache/go/build"

before_script:
  - mkdir -p "${GOPATH}/pkg/mod" "${GOCACHE}"

cache:
  key:
    files: [go.sum]
  paths:
    - .cache/go/pkg/mod
    - .cache/go/build
```

---

## Cache key стратегии

### По содержимому файла (рекомендуется для зависимостей)

```yaml
cache:
  key:
    files:
      - go.sum
  paths:
    - .cache/go/pkg/mod
```

При изменении `go.sum` — старый кеш не используется, создаётся новый.

### По ветке (изолировать per feature branch)

```yaml
cache:
  key: "$CI_COMMIT_REF_SLUG"    # main, feature-auth, etc.
  paths:
    - .cache/
```

Каждая ветка имеет свой кеш. Кеш из main не наследуется в feature branch.

### Сложный составной ключ

```yaml
cache:
  key:
    files:
      - go.sum
    prefix: "go-$CI_RUNNER_ID"    # изолировать по runner (разные OS)
  paths:
    - .cache/go/pkg/mod
    - .cache/go/build
```

### "Вечный" кеш для инструментов

```yaml
# Инструменты (golangci-lint, etc.) меняются редко
tools-cache:
  cache:
    key: "tools-v2"    # v2 — ручное версионирование
    paths:
      - .cache/tools
    policy: pull-push
  before_script:
    - |
      if [[ ! -f ".cache/tools/golangci-lint" ]]; then
        curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh \
          | sh -s -- -b .cache/tools v1.59.0
      fi
  script:
    - .cache/tools/golangci-lint run
```

---

## Cache policy: pull-push vs pull

```yaml
# pull-push (default): восстановить + сохранить
# Используется в jobs, которые могут обновить кеш (добавить новые модули)
build:
  cache:
    policy: pull-push
    key:
      files: [go.sum]
    paths: [.cache/go]

# pull: только восстановить, не сохранять
# Используется в jobs-потребителях (deploy, publish) — они не меняют зависимости
deploy:
  cache:
    policy: pull
    key:
      files: [go.sum]
    paths: [.cache/go]

# push: только сохранить (редко нужен)
# Например, отдельный "cache warming" job
warm-cache:
  cache:
    policy: push
```

`pull` вместо `pull-push` в read-only jobs:
- Не тратит время на upload кеша в конце job.
- Не перезаписывает кеш "пустым" если job не скачивал новых модулей.

---

## Artifacts: конфигурация

```yaml
build:
  script:
    - go build -o bin/app ./cmd/app
    - go test -coverprofile=coverage.out ./...
  artifacts:
    # ── Пути ──────────────────────────────────────────────────────
    paths:
      - bin/app
      - coverage.out

    # ── Время жизни ───────────────────────────────────────────────
    expire_in: 1 week      # 1 day, 1 week, 1 month, never

    # ── Когда сохранять ───────────────────────────────────────────
    when: on_success       # default
    # when: always         # включая при падении (для test отчётов)
    # when: on_failure     # только при падении (для debug)

    # ── Имя архива в UI ───────────────────────────────────────────
    name: "build-${CI_COMMIT_SHORT_SHA}"

    # ── Исключить файлы ───────────────────────────────────────────
    exclude:
      - bin/**/*.tmp
      - "**/*.test"

    # ── Reports (специальные типы) ────────────────────────────────
    reports:
      coverage_report:
        coverage_format: cobertura
        path: coverage.xml
      junit: test-results.xml
```

---

## Artifacts reports: встроенные типы

GitLab обрабатывает некоторые артефакты специально — показывает в UI, комментирует MR.

### JUnit test results

```yaml
test:
  script:
    - go install gotest.tools/gotestsum@latest
    - gotestsum --junitfile test-results.xml -- -race ./...
  artifacts:
    when: always
    reports:
      junit: test-results.xml
```

В MR UI появится: список упавших тестов, сравнение с main.

### Coverage

```yaml
test:
  script:
    - go test -coverprofile=coverage.out ./...
    - go install github.com/boumenot/gocover-cobertura@latest
    - gocover-cobertura < coverage.out > coverage.xml
  coverage: '/total:\s+\(statements\)\s+(\d+\.\d+)%/'
  artifacts:
    reports:
      coverage_report:
        coverage_format: cobertura
        path: coverage.xml
```

`coverage:` regex — GitLab извлекает процент из stdout и показывает badge.

### SAST (Static Application Security Testing)

```yaml
include:
  - template: Security/SAST.gitlab-ci.yml

# Кастомный govulncheck как SAST report
govulncheck:
  script:
    - govulncheck -json ./... > gl-sast-report.json || true
  artifacts:
    reports:
      sast: gl-sast-report.json
```

### Container scanning

```yaml
include:
  - template: Security/Container-Scanning.gitlab-ci.yml

container_scanning:
  variables:
    CS_IMAGE: "$CI_REGISTRY_IMAGE/app:$CI_COMMIT_SHA"
```

---

## Передача артефактов между jobs

### Через needs (рекомендуется)

```yaml
build:
  stage: build
  script:
    - go build -o bin/app ./cmd/app
    - docker build -t "$IMAGE:$CI_COMMIT_SHA" .
    - docker inspect --format='{{index .RepoDigests 0}}' "$IMAGE:$CI_COMMIT_SHA" > image-digest.txt
  artifacts:
    paths:
      - bin/app
      - image-digest.txt
    expire_in: 1 day

deploy-dev:
  stage: deploy
  needs:
    - job: build
      artifacts: true    # скачает bin/app и image-digest.txt
  script:
    - IMAGE_DIGEST=$(cat image-digest.txt)
    - ./scripts/deploy.sh --image "${IMAGE_DIGEST}"

deploy-prod:
  stage: deploy
  needs:
    - job: build
      artifacts: true    # тот же artifact, но уже другой stage
    - job: deploy-dev    # ждём dev, но не нужен его artifact
      artifacts: false
  when: manual
  script:
    - IMAGE_DIGEST=$(cat image-digest.txt)
    - ./scripts/deploy.sh --env prod --image "${IMAGE_DIGEST}"
```

### Через dependencies (legacy, менее гибко)

```yaml
deploy:
  stage: deploy
  dependencies:
    - build             # скачать artifacts только из build
    # без dependencies — скачиваются artifacts из ВСЕХ jobs выше
    # dependencies: [] — не скачивать ничего
```

Разница `needs` vs `dependencies`:
- `needs` управляет **порядком** (DAG) и **скачиванием**.
- `dependencies` управляет **только скачиванием** (без влияния на порядок).

---

## Антипаттерны

**GOPATH вне project dir** — `~/.go/pkg/mod` не кешируется GitLab Runner. Всегда ставить `GOPATH: "${CI_PROJECT_DIR}/.cache/go"`.

**`policy: pull-push` везде** — deploy jobs не должны обновлять кеш зависимостей. Это замедляет и может испортить кеш.

**Не задавать `expire_in`** — артефакты копятся, занимают место. По умолчанию хранятся вечно (зависит от настроек GitLab instance).

**Хранить секреты в артефактах** — конфигурационные файлы с credentials, `.env` файлы. Артефакты доступны для скачивания любому участнику проекта.

**`when: always` для всех артефактов** — огромные бинари сохраняются даже при падении lint. Использовать `when: always` только для test reports (нужны для диагностики при падении).

**Один кеш на весь pipeline** — если lint, test и build имеют одинаковый ключ и все `pull-push`, возникает race condition при параллельном выполнении. Добавлять `$CI_JOB_NAME` в ключ или разделять кеши.
