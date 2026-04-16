# Docker

Этот подпакет про практическое использование `Docker` для Go-сервисов: не только как собрать image, но и как понимать runtime, volumes, networking и ограничения контейнера.

Материалы:
- [Docker For Go Services](./docker-for-go-services.md)
- [Container vs Virtual Machine](./container-vs-virtual-machine.md)

Что важно уметь объяснить:
- чем image отличается от container;
- почему container не равен VM;
- как работают volumes и networks;
- как контейнеры получают конфиги и переменные окружения;
- что важно для сигналов, PID 1 и graceful shutdown в Go-сервисе.
