# GitHub Actions: Go Pipeline

Два готовых пайплайна: PR-проверка и build+deploy после мержа в main.

## Содержание

- [PR Pipeline: lint, test, build](#pr-pipeline-lint-test-build)
- [Main Pipeline: build image + deploy](#main-pipeline-build-image--deploy)
- [Makefile-цели, используемые в пайплайне](#makefile-цели-используемые-в-пайплайне)
- [Разбор ключевых решений](#разбор-ключевых-решений)

---

## PR Pipeline: lint, test, build

Файл: `.github/workflows/pr-ci.yml`

```yaml
name: pr-ci

on:
  pull_request:
    branches: [main]
    types: [opened, synchronize, reopened]

permissions:
  contents: read
  pull-requests: read

concurrency:
  group: pr-ci-${{ github.event.pull_request.number }}
  cancel-in-progress: true   # новый push в PR отменяет предыдущий run

jobs:
  # ── fmt ─────────────────────��──────────────────────────────────────
  fmt:
    name: Format Check
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: false                  # управляем кешом вручную

      - name: Restore Go cache
        id: cache
        uses: actions/cache/restore@v4
        with:
          path: |
            ~/go/pkg/mod
            ~/.cache/go-build
          key: go-${{ runner.os }}-${{ hashFiles('go.sum') }}-fmt
          restore-keys: |
            go-${{ runner.os }}-${{ hashFiles('go.sum') }}-
            go-${{ runner.os }}-

      - name: Check formatting
        run: |
          unformatted=$(gofmt -l ./...)
          if [[ -n "$unformatted" ]]; then
            echo "Unformatted files:"
            echo "$unformatted"
            exit 1
          fi

      - name: Save Go cache
        if: steps.cache.outputs.cache-hit != 'true'
        uses: actions/cache/save@v4
        with:
          path: |
            ~/go/pkg/mod
            ~/.cache/go-build
          key: go-${{ runner.os }}-${{ hashFiles('go.sum') }}-fmt

  # ── lint ───────────────────────��───────────────────────────────��───
  lint:
    name: Lint
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: false

      - name: Restore Go cache
        id: cache
        uses: actions/cache/restore@v4
        with:
          path: |
            ~/go/pkg/mod
            ~/.cache/go-build
          key: go-${{ runner.os }}-${{ hashFiles('go.sum') }}-lint
          restore-keys: |
            go-${{ runner.os }}-${{ hashFiles('go.sum') }}-
            go-${{ runner.os }}-

      - name: golangci-lint
        uses: golangci/golangci-lint-action@v6
        with:
          version: v1.59
          args: --timeout=5m

      - name: Save Go cache
        if: steps.cache.outputs.cache-hit != 'true'
        uses: actions/cache/save@v4
        with:
          path: |
            ~/go/pkg/mod
            ~/.cache/go-build
          key: go-${{ runner.os }}-${{ hashFiles('go.sum') }}-lint

  # ── test ─────────────────────────────────────────────────────��─────
  test:
    name: Test
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: false

      - name: Restore Go cache
        id: cache
        uses: actions/cache/restore@v4
        with:
          path: |
            ~/go/pkg/mod
            ~/.cache/go-build
          key: go-${{ runner.os }}-${{ hashFiles('go.sum') }}-test
          restore-keys: |
            go-${{ runner.os }}-${{ hashFiles('go.sum') }}-
            go-${{ runner.os }}-

      - name: Run tests
        run: go test -race -coverprofile=coverage.out -covermode=atomic ./...

      - name: Check coverage threshold
        run: |
          coverage=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | tr -d '%')
          threshold=70
          if (( $(echo "$coverage < $threshold" | bc -l) )); then
            echo "Coverage ${coverage}% is below threshold ${threshold}%"
            exit 1
          fi
          echo "Coverage: ${coverage}%"

      - name: Upload coverage report
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: coverage-report
          path: coverage.out
          retention-days: 7

      - name: Save Go cache
        if: steps.cache.outputs.cache-hit != 'true'
        uses: actions/cache/save@v4
        with:
          path: |
            ~/go/pkg/mod
            ~/.cache/go-build
          key: go-${{ runner.os }}-${{ hashFiles('go.sum') }}-test

  # ── build ────────────────────────────���─────────────────────────────
  build:
    name: Build
    runs-on: ubuntu-latest
    needs: [fmt, lint, test]    # запускается только после успеха всех
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: false

      - name: Restore Go cache
        id: cache
        uses: actions/cache/restore@v4
        with:
          path: |
            ~/go/pkg/mod
            ~/.cache/go-build
          key: go-${{ runner.os }}-${{ hashFiles('go.sum') }}-build
          restore-keys: |
            go-${{ runner.os }}-${{ hashFiles('go.sum') }}-
            go-${{ runner.os }}-

      - name: Build binary
        run: |
          CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
            go build -ldflags="-s -w -X main.version=${{ github.sha }}" \
            -o ./bin/app ./cmd/app

      - name: Save Go cache
        if: steps.cache.outputs.cache-hit != 'true'
        uses: actions/cache/save@v4
        with:
          path: |
            ~/go/pkg/mod
            ~/.cache/go-build
          key: go-${{ runner.os }}-${{ hashFiles('go.sum') }}-build

      - name: govulncheck
        run: |
          go install golang.org/x/vuln/cmd/govulncheck@latest
          govulncheck ./...
```

---

## Main Pipeline: build image + deploy

Файл: `.github/workflows/build-main.yml`

```yaml
name: build-main

on:
  push:
    branches: [main]
  workflow_dispatch:
    inputs:
      force_deploy:
        description: Force deploy even without code changes
        type: boolean
        default: false

permissions:
  contents: read
  packages: write        # push в GitHub Container Registry
  id-token: write        # OIDC (если деплоим в cloud)

concurrency:
  group: build-main
  cancel-in-progress: true   # отменять если уже идёт другой push

env:
  REGISTRY: ghcr.io
  IMAGE_NAME: ${{ github.repository }}/app

jobs:
  # ── quality gates (fmt + lint + test) ────────────��────────────────
  quality:
    name: Quality Gates
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: false

      - name: Restore Go cache
        id: cache
        uses: actions/cache/restore@v4
        with:
          path: |
            ~/go/pkg/mod
            ~/.cache/go-build
          key: go-${{ runner.os }}-${{ hashFiles('go.sum') }}-quality
          restore-keys: |
            go-${{ runner.os }}-${{ hashFiles('go.sum') }}-
            go-${{ runner.os }}-

      - name: Format check
        run: test -z "$(gofmt -l ./...)"

      - name: Lint
        uses: golangci/golangci-lint-action@v6
        with:
          version: v1.59

      - name: Test
        run: go test -race ./...

      - name: Save Go cache
        if: steps.cache.outputs.cache-hit != 'true'
        uses: actions/cache/save@v4
        with:
          path: |
            ~/go/pkg/mod
            ~/.cache/go-build
          key: go-${{ runner.os }}-${{ hashFiles('go.sum') }}-quality

  # ── build и push Docker image ──────────────────────────────��───────
  build-image:
    name: Build & Push Image
    runs-on: ubuntu-latest
    needs: quality
    outputs:
      image_digest: ${{ steps.push.outputs.digest }}
      image_tag: ${{ steps.meta.outputs.tags }}

    steps:
      - uses: actions/checkout@v4

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3

      - name: Log in to GHCR
        uses: docker/login-action@v3
        with:
          registry: ${{ env.REGISTRY }}
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Extract metadata (tags, labels)
        id: meta
        uses: docker/metadata-action@v5
        with:
          images: ${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}
          tags: |
            type=sha,prefix=,format=long
            type=raw,value=main-latest

      - name: Build and push
        id: push
        uses: docker/build-push-action@v6
        with:
          context: .
          push: true
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
          cache-from: |
            type=gha,scope=app
            type=registry,ref=${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}:main-latest
          cache-to: type=gha,mode=max,scope=app,ignore-error=true

      - name: Write build summary
        run: |
          {
            echo "## Build Result"
            echo "- image: \`${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}@${{ steps.push.outputs.digest }}\`"
            echo "- commit: \`${{ github.sha }}\`"
          } >> "${GITHUB_STEP_SUMMARY}"

  # ── deploy dev ─────────────────────────────────────────────────────
  deploy-dev:
    name: Deploy → Dev
    runs-on: ubuntu-latest
    needs: build-image
    environment: dev               # protection rules, secrets scope

    steps:
      - uses: actions/checkout@v4

      - name: Deploy
        run: ./scripts/deploy.sh
        env:
          IMAGE_DIGEST: ${{ needs.build-image.outputs.image_digest }}
          DEPLOY_ENV: dev
          DEPLOY_TOKEN: ${{ secrets.DEV_DEPLOY_TOKEN }}

  # ── deploy prod (manual approve) ─────────────────────────��─────────
  deploy-prod:
    name: Deploy → Prod
    runs-on: ubuntu-latest
    needs: [build-image, deploy-dev]
    environment: prod              # требует approve в GitHub UI

    steps:
      - uses: actions/checkout@v4

      - name: Deploy
        run: ./scripts/deploy.sh
        env:
          IMAGE_DIGEST: ${{ needs.build-image.outputs.image_digest }}
          DEPLOY_ENV: prod
          DEPLOY_TOKEN: ${{ secrets.PROD_DEPLOY_TOKEN }}
```

---

## Makefile-цели, используемые в пайплайне

```makefile
.PHONY: fmt-check lint test build vuln

fmt-check:
	@unformatted=$$(gofmt -l ./...); \
	if [ -n "$$unformatted" ]; then \
		echo "Unformatted: $$unformatted"; exit 1; \
	fi

lint:
	golangci-lint run --timeout=5m

test:
	go test -race -coverprofile=coverage.out ./...

test-only:          # без линтера, для CI
	go test -race ./...

build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o ./bin/app ./cmd/app

vuln:
	govulncheck ./...
```

---

## Разбор ключевых решений

**`cache: false` + manual cache/restore** вместо встроенного `cache: true` в `setup-go`:
- Позволяет использовать `restore-keys` (fallback на частичный кеш).
- Явный контроль: не сохранять кеш если он уже был (экономия на upload).
- Позволяет иметь разные ключи для разных jobs (lint vs test).

**`cancel-in-progress: true` на PR** — каждый новый push отменяет предыдущий run, не тратим runners впустую.

**`cancel-in-progress: true` на main** — если два push пришли быстро, не деплоим старый поверх нового. Но осторожно: если deploy job уже начался — его тоже отменит. Для production лучше `false`.

**`environment:` на deploy jobs** — скоупит secrets, добавляет protection rules, показывает историю деплоев в GitHub UI.

**Image по digest, не по тегу** — `main-latest` тег меняется, digest иммутабелен. Deploy всегда использует `outputs.digest`.

**`govulncheck`** — официальный инструмент от Go team, проверяет зависимости на известные уязвимости из Go vulnerability database.
