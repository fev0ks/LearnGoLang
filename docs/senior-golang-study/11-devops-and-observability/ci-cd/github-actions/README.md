# GitHub Actions

## Материалы

- [01 Синтаксис и концепции](./01-syntax-and-concepts.md) — workflow structure, triggers, permissions, concurrency, contexts, outputs
- [02 Go Pipeline](./02-go-pipeline.md) — готовый PR pipeline (fmt, lint, test, build) и main pipeline (build image + deploy)
- [03 Кеширование](./03-caching.md) — Go modules cache, build cache, Docker layer cache, restore-keys стратегия
- [04 Docker и реестры](./04-docker-and-registry.md) — Buildx, GHCR, Docker Hub, metadata-action, multi-platform, Dockerfile для Go
- [05 Secrets и OIDC](./05-secrets-and-oidc.md) — secrets vs vars, environments, OIDC с GCP и AWS (без long-lived keys)
- [06 Монорепо и Matrix](./06-monorepo-matrix.md) — path filters, dynamic matrix, detect-changed-services паттерн

## Официальная документация

- [GitHub Actions Docs](https://docs.github.com/en/actions)
- [Workflow syntax](https://docs.github.com/en/actions/writing-workflows/workflow-syntax-for-github-actions)
- [Contexts and expressions](https://docs.github.com/en/actions/writing-workflows/choosing-what-your-workflow-does/accessing-contextual-information-about-workflow-runs)
- [Security hardening](https://docs.github.com/en/actions/security-for-github-actions/security-guides/security-hardening-for-github-actions)
