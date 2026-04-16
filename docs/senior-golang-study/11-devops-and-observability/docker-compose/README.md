# Docker Compose

Этот подпакет про локальные multi-service стеки: как быстро поднять Go API вместе с базой, кэшем, брокером и observability-инструментами.

Материалы:
- [Docker Compose Anatomy](./docker-compose-anatomy.md)
- [Compose File Reference](./compose-file-reference/README.md)
- [Docker Compose For Go Projects](./docker-compose-for-go-projects.md)
- [Complex Compose Example](./compose-go-stack.example.yaml)

Что важно уметь объяснить:
- когда `docker compose` достаточно, а когда уже нужен `Kubernetes`;
- зачем нужны `profiles`, `healthcheck`, `depends_on`, `volumes`, `networks`;
- как собирать удобный local dev stack для Go-сервиса;
- почему compose полезен для разработки и integration testing, но не заменяет production orchestration.
