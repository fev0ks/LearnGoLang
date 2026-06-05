# CI/CD

Автоматизация сборки, тестирования и деплоя Go-сервисов.

## Материалы

- [01 Концепции](./01-concepts.md) — CI vs CD, анатомия пайплайна, triggers, runners, secrets, deployment strategies

### GitHub Actions

- [GitHub Actions →](./github-actions/README.md)
  - Синтаксис и концепции
  - Go pipeline (PR + main)
  - Кеширование Go modules и Docker
  - Docker и реестры образов
  - Secrets и OIDC (без long-lived keys)
  - Монорепо и dynamic matrix

### GitLab CI

- [GitLab CI →](./gitlab-ci/README.md)
  - Синтаксис и концепции
  - Go pipeline
  - Кеш и артефакты
  - Docker и Container Registry (DinD, Kaniko)
  - Secrets, environments, OIDC, Vault
