# GitHub Actions: Монорепо и Matrix Builds

## Содержание

- [Path filters: запускать только нужное](#path-filters-запускать-только-нужное)
- [Static matrix](#static-matrix)
- [Dynamic matrix: detect-changed-services](#dynamic-matrix-detect-changed-services)
- [Matrix options: fail-fast, max-parallel](#matrix-options-fail-fast-max-parallel)
- [Concurrency в монорепо](#concurrency-в-монорепо)
- [Реальный пример: monorepo build pipeline](#реальный-пример-monorepo-build-pipeline)

---

## Path filters: запускать только нужное

### Trigger-level filters

```yaml
on:
  push:
    branches: [main]
    paths:
      - "services/**"
      - "libs/**"
      - "go.work"
      - "go.work.sum"
    paths-ignore:
      - "docs/**"
      - "**/*.md"
      - ".github/workflows/frontend-*.yml"
```

Ограничение: если все изменения в `docs/` — workflow не запустится вообще. Для PR это может заблокировать merge если workflow обязателен (required status check).

Решение: добавить отдельный job, который всегда проходит:

```yaml
jobs:
  # Этот job запускается только если нет изменений в paths
  skip-ci:
    if: github.event_name == 'push'    # только для push без paths
    runs-on: ubuntu-latest
    steps:
      - run: echo "No service changes, skipping CI"
```

### Job-level path check

Для более гибкого контроля — использовать `dorny/paths-filter`:

```yaml
jobs:
  changes:
    runs-on: ubuntu-latest
    outputs:
      api: ${{ steps.filter.outputs.api }}
      worker: ${{ steps.filter.outputs.worker }}
      shared: ${{ steps.filter.outputs.shared }}
    steps:
      - uses: actions/checkout@v4
      - uses: dorny/paths-filter@v3
        id: filter
        with:
          filters: |
            api:
              - 'services/api/**'
              - 'libs/shared/**'
            worker:
              - 'services/worker/**'
              - 'libs/shared/**'
            shared:
              - 'libs/shared/**'

  test-api:
    needs: changes
    if: needs.changes.outputs.api == 'true'
    runs-on: ubuntu-latest
    steps:
      - run: make test-api

  test-worker:
    needs: changes
    if: needs.changes.outputs.worker == 'true'
    runs-on: ubuntu-latest
    steps:
      - run: make test-worker
```

---

## Static matrix

```yaml
jobs:
  test:
    strategy:
      matrix:
        go-version: ["1.21", "1.22", "1.23"]
        os: [ubuntu-latest, macos-latest]
        # exclude: убрать конкретные комбинации
        exclude:
          - os: macos-latest
            go-version: "1.21"
        # include: добавить дополнительные комбинации
        include:
          - os: ubuntu-latest
            go-version: "1.22"
            extra-flag: "-race"

    runs-on: ${{ matrix.os }}
    name: "Test Go ${{ matrix.go-version }} on ${{ matrix.os }}"

    steps:
      - uses: actions/setup-go@v5
        with:
          go-version: ${{ matrix.go-version }}
      - run: go test ${{ matrix.extra-flag }} ./...
```

---

## Dynamic matrix: detect-changed-services

Мощный паттерн для монорепо: скрипт определяет какие сервисы изменились и генерирует matrix.

### Структура монорепо

```
services/
├── api/
│   ├── Dockerfile
│   └── ...
├── worker/
│   ├── Dockerfile
│   └── ...
└── cron/
    ├── Dockerfile
    └── ...
libs/
├── shared/
└── auth/
service-manifest.json    ← метаданные сервисов
```

### service-manifest.json

```json
{
  "services": [
    {
      "name": "api",
      "path": "services/api",
      "dockerfile": "services/api/Dockerfile",
      "artifact": "api",
      "cloud_run_service": "my-api"
    },
    {
      "name": "worker",
      "path": "services/worker",
      "dockerfile": "services/worker/Dockerfile",
      "artifact": "worker",
      "cloud_run_job": "my-worker"
    }
  ]
}
```

### detect-changed-services.sh

```bash
#!/usr/bin/env bash
set -euo pipefail

# Использование:
# ./detect-changed-services.sh --base SHA --head SHA --format matrix
# ./detect-changed-services.sh --all --format names

BASE=""
HEAD=""
FORMAT="matrix"
ALL=false
MANIFEST="service-manifest.json"

while [[ $# -gt 0 ]]; do
  case $1 in
    --base)  BASE="$2"; shift 2 ;;
    --head)  HEAD="$2"; shift 2 ;;
    --format) FORMAT="$2"; shift 2 ;;
    --all)   ALL=true; shift ;;
    *) echo "Unknown arg: $1" >&2; exit 1 ;;
  esac
done

# Получить все сервисы из манифеста
all_services=$(jq -r '.services[].name' "${MANIFEST}")

if [[ "${ALL}" == "true" ]]; then
  changed_services="${all_services}"
else
  # Получить изменённые файлы между двумя SHA
  changed_files=$(git diff --name-only "${BASE}" "${HEAD}")

  changed_services=""
  while IFS= read -r service; do
    service_path=$(jq -r --arg name "${service}" '.services[] | select(.name == $name) | .path' "${MANIFEST}")

    # Проверить есть ли изменения в пути сервиса или в libs/
    if echo "${changed_files}" | grep -qE "^(${service_path}/|libs/)"; then
      changed_services="${changed_services} ${service}"
    fi
  done <<< "${all_services}"

  changed_services=$(echo "${changed_services}" | xargs)
fi

if [[ -z "${changed_services}" ]]; then
  case "${FORMAT}" in
    matrix) echo '{"include":[]}' ;;
    count)  echo "0" ;;
    names)  echo "" ;;
  esac
  exit 0
fi

case "${FORMAT}" in
  matrix)
    # Генерируем JSON matrix для GitHub Actions
    includes="[]"
    for service in ${changed_services}; do
      entry=$(jq --arg name "${service}" \
        '.services[] | select(.name == $name)' "${MANIFEST}")
      includes=$(echo "${includes}" | jq --argjson entry "${entry}" '. + [$entry]')
    done
    echo "{\"include\":${includes}}"
    ;;
  count)
    echo "${changed_services}" | wc -w | xargs
    ;;
  names)
    echo "${changed_services}"
    ;;
esac
```

### Workflow с dynamic matrix

```yaml
jobs:
  # ── 1. Определить что изменилось ──────────────────────────────────
  detect-changes:
    runs-on: ubuntu-latest
    outputs:
      matrix: ${{ steps.detect.outputs.matrix }}
      count:  ${{ steps.detect.outputs.count }}
      names:  ${{ steps.detect.outputs.names }}
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0    # нужна вся история для git diff

      - id: detect
        run: |
          if [[ "${{ github.event_name }}" == "workflow_dispatch" && "${{ inputs.force_all }}" == "true" ]]; then
            matrix=$(./scripts/ci/detect-changed-services.sh --all --format matrix)
            count=$(./scripts/ci/detect-changed-services.sh  --all --format count)
            names=$(./scripts/ci/detect-changed-services.sh  --all --format names)
          else
            base="${{ github.event.before }}"
            head="${{ github.sha }}"
            matrix=$(./scripts/ci/detect-changed-services.sh --base "${base}" --head "${head}" --format matrix)
            count=$(./scripts/ci/detect-changed-services.sh  --base "${base}" --head "${head}" --format count)
            names=$(./scripts/ci/detect-changed-services.sh  --base "${base}" --head "${head}" --format names)
          fi

          echo "matrix=${matrix}" >> "${GITHUB_OUTPUT}"
          echo "count=${count}"   >> "${GITHUB_OUTPUT}"
          echo "names=${names}"   >> "${GITHUB_OUTPUT}"

          {
            echo "### Affected Services"
            echo "- count: ${count}"
            echo "- names: ${names:-<none>}"
          } >> "${GITHUB_STEP_SUMMARY}"

  # ── 2. Сборка только изменившихся сервисов ────────────────────────
  build:
    needs: detect-changes
    if: needs.detect-changes.outputs.count != '0'
    runs-on: ubuntu-latest
    strategy:
      fail-fast: false       # один упавший не убивает остальных
      max-parallel: 6
      matrix: ${{ fromJson(needs.detect-changes.outputs.matrix) }}
    name: "build-${{ matrix.name }}"

    steps:
      - uses: actions/checkout@v4

      - uses: docker/build-push-action@v6
        with:
          context: .
          file: ${{ matrix.dockerfile }}
          push: true
          tags: |
            ghcr.io/myorg/myrepo/${{ matrix.artifact }}:${{ github.sha }}
            ghcr.io/myorg/myrepo/${{ matrix.artifact }}:main-latest
          cache-from: type=gha,scope=${{ matrix.name }}
          cache-to: type=gha,mode=max,scope=${{ matrix.name }},ignore-error=true

  # ── 3. Если ничего не изменилось ─────────────────────────────────
  no-changes:
    needs: detect-changes
    if: needs.detect-changes.outputs.count == '0'
    runs-on: ubuntu-latest
    steps:
      - run: echo "No service changes on this push."
```

---

## Matrix options: fail-fast, max-parallel

```yaml
strategy:
  fail-fast: false      # НЕ отменять другие jobs если один упал
                        # default: true — при одном падении все остальные отменяются
  max-parallel: 4       # максимум параллельных jobs
                        # default: все параллельно
  matrix:
    service: [api, worker, cron, scheduler]
```

**`fail-fast: false`** критически важен для монорепо: если упал `worker`, не нужно отменять `api`. Каждый сервис независим.

**`max-parallel`** — ограничить параллелизм если runner pool ограничен или нет смысла гонять 20 jobs одновременно.

---

## Concurrency в монорепо

```yaml
# PR: отменять предыдущий run того же PR
concurrency:
  group: pr-${{ github.event.pull_request.number }}
  cancel-in-progress: true

# main branch: отменять предыдущий build если ещё не начался deploy
concurrency:
  group: build-main
  cancel-in-progress: true

# deploy per environment: не отменять (дождаться завершения)
concurrency:
  group: deploy-${{ inputs.environment }}
  cancel-in-progress: false
```

Для matrix jobs в монорепо — concurrency на уровне workflow достаточен. Каждый matrix job является частью одного run.

---

## Реальный пример: monorepo build pipeline

Схема полного пайплайна из skibookers:

```
push to main
    │
    ├── detect-changes ──────────────────────────────────────┐
    │   (выдаёт matrix: [{api, worker, cron}])               │
    │                                                         │
    ├── quality-gates (lint + test + proto-check)             │
    │                                                         │
    └── build-images (зависит от обоих выше) ─── matrix ──── ┘
        ├── build-api      → upload artifact: manifest-api.json
        ├── build-worker   → upload artifact: manifest-worker.json
        └── build-cron     → upload artifact: manifest-cron.json
                │
                ▼
        deploy-dev (matrix, parallel)
        ├── deploy-api-dev
        ├── deploy-worker-dev
        └── deploy-cron-dev
                │
                ▼
        publish-release-manifest
        (скачивает все manifest-*.json, собирает release-manifest.json)
```

Затем отдельный workflow `promote-release`:
```
workflow_dispatch (build_run_number, target_env)
    │
    ├── validate-inputs (проверить номер run, статус)
    └── promote (скачать release-manifest, задеплоить все сервисы)
        └── [если prod] tag-and-release
```

Ключевые идеи этого паттерна:
1. **Release manifest** — JSON с digest всех образов конкретного build. Деплой всегда использует точный список образов, не latest теги.
2. **Promote, не rebuild** — staging и prod деплоятся из уже собранных образов, не пересобираются.
3. **Matrix per service** — каждый сервис независим, параллельная сборка и деплой.
