# GitHub Actions: синтаксис и концепции

## Содержание

- [Структура workflow файла](#структура-workflow-файла)
- [Triggers (on)](#triggers-on)
- [Permissions](#permissions)
- [Concurrency](#concurrency)
- [Jobs](#jobs)
- [Steps](#steps)
- [Contexts и выражения](#contexts-и-выражения)
- [Передача данных между steps и jobs](#передача-данных-между-steps-и-jobs)
- [Reusable workflows и composite actions](#reusable-workflows-и-composite-actions)

---

## Структура workflow файла

Файлы хранятся в `.github/workflows/*.yml`. Каждый файл — отдельный workflow.

```yaml
name: pr-ci                          # отображаемое имя в UI

run-name: "PR #${{ github.event.pull_request.number }}: ${{ github.event.pull_request.title }}"

on:                                  # triggers
  pull_request:
    branches: [main]

env:                                 # глобальные переменные для всех jobs
  GO_VERSION: "1.22"

permissions:                         # минимальные права GITHUB_TOKEN
  contents: read
  pull-requests: read

concurrency:                         # группа для отмены дублирующих запусков
  group: pr-${{ github.event.pull_request.number }}
  cancel-in-progress: true

jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: make lint
```

---

## Triggers (on)

### push и pull_request

```yaml
on:
  push:
    branches: [main, "release/*"]
    paths:                           # только если изменились эти пути
      - "services/**"
      - "libs/**"
      - "go.work"
    tags: ["v*"]                     # или при пуше тега

  pull_request:
    branches: [main]
    types: [opened, synchronize, reopened]
    paths-ignore:
      - "docs/**"
      - "*.md"
```

### workflow_dispatch: ручной запуск с параметрами

```yaml
on:
  workflow_dispatch:
    inputs:
      environment:
        description: "Deploy target"
        required: true
        type: choice
        options: [staging, prod]
      force_rebuild:
        description: "Rebuild all services"
        type: boolean
        default: false
      version:
        description: "Version tag (e.g. v1.2.3)"
        required: false
        type: string
```

Доступ: `${{ inputs.environment }}`, `${{ inputs.force_rebuild }}`.

### schedule: cron

```yaml
on:
  schedule:
    - cron: "0 3 * * 1-5"    # в 3:00 UTC по будням
```

### workflow_call: вызов из другого workflow

```yaml
# reusable workflow
on:
  workflow_call:
    inputs:
      service:
        type: string
        required: true
    secrets:
      DEPLOY_TOKEN:
        required: true
```

---

## Permissions

`GITHUB_TOKEN` — автоматически создаётся для каждого запуска. По умолчанию имеет широкие права. Принцип минимальных привилегий:

```yaml
permissions:
  contents: read        # читать код репо
  pull-requests: write  # комментировать PR
  packages: write       # пушить в GHCR
  id-token: write       # OIDC (для получения токена cloud провайдера)
  actions: read         # читать artifacts из других runs
```

Права можно задавать на уровне workflow (глобально) и переопределять на уровне job.

```yaml
permissions: read-all   # все read, ничего write

jobs:
  deploy:
    permissions:
      id-token: write   # только этот job получает write
      contents: read
```

---

## Concurrency

Предотвращает одновременный запуск нескольких пайплайнов в одной группе.

```yaml
# для PR: отменить предыдущий run того же PR при новом push
concurrency:
  group: pr-ci-${{ github.event.pull_request.number }}
  cancel-in-progress: true

# для deploy: не отменять — дождаться завершения текущего
concurrency:
  group: deploy-prod
  cancel-in-progress: false
```

---

## Jobs

```yaml
jobs:
  test:
    name: "Run Tests"              # отображаемое имя
    runs-on: ubuntu-latest         # runner
    timeout-minutes: 15            # максимальное время
    if: github.actor != 'dependabot[bot]'  # условие запуска

    needs: [lint]                  # зависимость от других jobs

    outputs:                       # передача данных следующим jobs
      image_tag: ${{ steps.build.outputs.tag }}

    env:                           # переменные только для этого job
      CGO_ENABLED: 0

    strategy:                      # matrix build
      fail-fast: false
      max-parallel: 4
      matrix:
        go: ["1.21", "1.22"]
        os: [ubuntu-latest, macos-latest]

    steps: [...]
```

### needs и условия

```yaml
jobs:
  build:
    needs: [lint, test]            # запустится только если оба успешны
    if: needs.lint.result == 'success' && needs.test.result == 'success'

  notify:
    needs: [build, deploy]
    if: always()                   # запустится даже если предыдущие упали
```

---

## Steps

```yaml
steps:
  # использовать action
  - name: Checkout
    uses: actions/checkout@v4
    with:
      fetch-depth: 0               # вся история (нужна для git diff)
      ref: ${{ github.sha }}

  # запустить команду
  - name: Run Tests
    id: tests                      # id нужен для обращения к outputs
    run: |
      go test -race -coverprofile=coverage.out ./...
      go tool cover -func=coverage.out
    env:
      GOFLAGS: "-mod=readonly"

  # условный step
  - name: Upload Coverage
    if: success() && github.event_name == 'pull_request'
    uses: actions/upload-artifact@v4
    with:
      name: coverage
      path: coverage.out

  # step с продолжением при ошибке
  - name: Lint
    continue-on-error: true
    run: golangci-lint run
```

---

## Contexts и выражения

Контексты доступны через `${{ context.property }}`:

| Контекст | Примеры |
|---|---|
| `github` | `.sha`, `.ref`, `.event_name`, `.actor`, `.repository`, `.run_number`, `.run_id` |
| `env` | Переменные из `env:` блоков |
| `secrets` | `secrets.MY_SECRET` |
| `vars` | `vars.MY_VAR` (non-secret variables) |
| `steps` | `steps.step_id.outputs.key`, `steps.step_id.outcome` |
| `needs` | `needs.job_id.outputs.key`, `needs.job_id.result` |
| `inputs` | `inputs.param_name` (workflow_dispatch / workflow_call) |
| `runner` | `.os`, `.arch`, `.temp` |

Встроенные функции:

```yaml
# hashFiles: хеш содержимого файлов
key: cache-${{ hashFiles('go.sum', 'go.work.sum') }}

# fromJson / toJson
matrix: ${{ fromJson(needs.detect.outputs.matrix) }}

# contains, startsWith, endsWith
if: contains(github.ref, 'release/')
if: startsWith(github.ref, 'refs/tags/v')

# format
run: echo "Image tag: ${{ format('{0}-{1}', github.sha, github.run_number) }}"
```

---

## Передача данных между steps и jobs

### GITHUB_OUTPUT: step → step (в одном job)

```yaml
- id: version
  run: echo "tag=v1.2.3" >> "${GITHUB_OUTPUT}"

- run: echo "Tag is ${{ steps.version.outputs.tag }}"
```

### GITHUB_STEP_SUMMARY: markdown в UI

```yaml
- run: |
    {
      echo "## Build Summary"
      echo "- commit: ${{ github.sha }}"
      echo "- image: ${IMAGE_TAG}"
    } >> "${GITHUB_STEP_SUMMARY}"
```

### Job outputs: job → job

```yaml
jobs:
  build:
    outputs:
      image_digest: ${{ steps.push.outputs.digest }}
    steps:
      - id: push
        run: echo "digest=sha256:abc123" >> "${GITHUB_OUTPUT}"

  deploy:
    needs: build
    steps:
      - run: echo "Deploying ${{ needs.build.outputs.image_digest }}"
```

### Artifacts: между jobs или для скачивания

```yaml
# upload
- uses: actions/upload-artifact@v4
  with:
    name: build-binary
    path: ./bin/myapp
    retention-days: 7

# download (в другом job)
- uses: actions/download-artifact@v4
  with:
    name: build-binary
    path: ./bin/
```

---

## Reusable workflows и composite actions

### Reusable workflow: целый workflow как подпрограмма

```yaml
# .github/workflows/deploy.yml
on:
  workflow_call:
    inputs:
      environment:
        type: string
        required: true
      image_digest:
        type: string
        required: true
    secrets:
      DEPLOY_TOKEN:
        required: true

jobs:
  deploy:
    runs-on: ubuntu-latest
    environment: ${{ inputs.environment }}
    steps:
      - run: ./scripts/deploy.sh ${{ inputs.image_digest }}
        env:
          TOKEN: ${{ secrets.DEPLOY_TOKEN }}
```

```yaml
# вызов из другого workflow
jobs:
  deploy-prod:
    uses: ./.github/workflows/deploy.yml
    with:
      environment: prod
      image_digest: ${{ needs.build.outputs.digest }}
    secrets:
      DEPLOY_TOKEN: ${{ secrets.PROD_DEPLOY_TOKEN }}
```

### Composite action: переиспользуемый набор steps

```yaml
# .github/actions/setup-go-cache/action.yml
name: Setup Go with cache
inputs:
  go-version-file:
    default: go.mod

runs:
  using: composite
  steps:
    - uses: actions/setup-go@v5
      with:
        go-version-file: ${{ inputs.go-version-file }}
        cache: false
    - uses: actions/cache@v4
      with:
        path: |
          ~/go/pkg/mod
          ~/.cache/go-build
        key: go-${{ runner.os }}-${{ hashFiles('**/go.sum') }}
```

```yaml
# использование
- uses: ./.github/actions/setup-go-cache
  with:
    go-version-file: go.work
```
