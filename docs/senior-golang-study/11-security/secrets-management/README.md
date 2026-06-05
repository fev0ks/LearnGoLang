# Secrets Management

Этот подпакет про практическую работу с секретами в backend-сервисах: где их хранить, как передавать в приложение и какие подходы подходят для local dev, `docker compose`, `Kubernetes` и CI/CD.

Как читать:
- сначала понять общие правила и threat model;
- затем сравнить способы доставки секретов;
- после этого посмотреть отдельные сценарии для local dev, containers и `Kubernetes`.

Материалы:
- [Secrets Delivery Options](./01-secrets-delivery-options.md)
- [Local Development Secrets](./02-local-development-secrets.md)
- [Docker Compose And Container Secrets](./03-docker-compose-and-container-secrets.md)
- [Kubernetes Secrets And External Managers](./04-kubernetes-secrets-and-external-managers.md)

Что важно уметь объяснить:
- почему нельзя просто хардкодить секреты в код, image и compose-файлы;
- когда `env vars` нормальны, а когда лучше file mounts или external secret manager;
- как держать local dev удобным без превращения репозитория в свалку `.env`;
- как CI/CD и runtime должны разделять build artifact и secret delivery.
