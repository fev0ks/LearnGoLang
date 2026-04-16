# Compose File Reference

Этот подпакет разбирает compose-файл по секциям. Цель не в том, чтобы переписать всю Compose specification, а в том, чтобы собрать понятный backend-oriented reference: что означает каждая часть файла, какие там бывают параметры и какие значения реально встречаются на практике.

Сверено с официальной Docker Compose documentation на апрель 2026:
- [Compose file reference](https://docs.docker.com/reference/compose-file/)
- [Services](https://docs.docker.com/reference/compose-file/services/)
- [Compose Build Specification](https://docs.docker.com/reference/compose-file/build/)
- [Networks](https://docs.docker.com/reference/compose-file/networks/)
- [Volumes](https://docs.docker.com/reference/compose-file/volumes/)
- [Configs](https://docs.docker.com/reference/compose-file/configs/)
- [Secrets](https://docs.docker.com/reference/compose-file/secrets/)
- [Profiles](https://docs.docker.com/reference/compose-file/profiles/)

Материалы:
- [01 Top-Level Structure](./01-top-level-structure.md)
- [02 Service Definition](./02-service-definition.md)
- [03 Build And Image](./03-build-and-image.md)
- [04 Environment And Env File](./04-environment-and-env-file.md)
- [05 Ports And Expose](./05-ports-and-expose.md)
- [06 Depends On](./06-depends-on.md)
- [07 Healthcheck](./07-healthcheck.md)
- [08 Networks](./08-networks.md)
- [09 Volumes](./09-volumes.md)
- [10 Configs And Secrets](./10-configs-and-secrets.md)
- [11 Command Entrypoint And Restart](./11-command-entrypoint-and-restart.md)
- [12 Profiles](./12-profiles.md)

Как читать:
- сначала пройти [`01 Top-Level Structure`](./01-top-level-structure.md), чтобы понять карту файла;
- потом [`02 Service Definition`](./02-service-definition.md), чтобы увидеть, как собирается один service;
- затем пройти отдельные секции по мере важности: `build`, `env`, `ports`, `depends_on`, `healthcheck`, `networks`, `volumes`;
- после этого вернуться к [`compose-go-stack.example.yaml`](../compose-go-stack.example.yaml) и прочитать уже реальный пример целиком.

Что не покрыто специально:
- редкие low-level keys вроде `blkio_config`, `oom_score_adj`, `device_cgroup_rules`;
- swarm-centric или platform-specific детали, если они почти не встречаются в backend local dev.
