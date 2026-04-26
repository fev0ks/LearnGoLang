# GitLab CI

## Материалы

- [01 Синтаксис и концепции](./01-syntax-and-concepts.md) — stages, jobs, variables, rules, needs, extends, include
- [02 Go Pipeline](./02-go-pipeline.md) — готовый `.gitlab-ci.yml`: fmt, lint, test, build image, deploy
- [03 Кеш и Артефакты](./03-caching-and-artifacts.md) — cache key стратегии, policy, artifacts reports (JUnit, coverage)
- [04 Docker и Registry](./04-docker-and-registry.md) — GitLab Container Registry, DinD, Kaniko, buildkitd, layer cache
- [05 Secrets и Environments](./05-secrets-and-environments.md) — protected/masked variables, environments, OIDC с AWS/GCP, Vault

## Официальная документация

- [GitLab CI/CD Docs](https://docs.gitlab.com/ee/ci/)
- [.gitlab-ci.yml reference](https://docs.gitlab.com/ee/ci/yaml/)
- [Predefined variables](https://docs.gitlab.com/ee/ci/variables/predefined_variables.html)
- [OIDC with GitLab CI](https://docs.gitlab.com/ee/ci/cloud_services/)
