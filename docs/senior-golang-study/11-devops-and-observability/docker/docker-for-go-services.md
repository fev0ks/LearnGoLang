# Docker For Go Services

## Базовая ментальная модель

`Image`:
- шаблон файловой системы и команд;
- immutable build artifact.

`Container`:
- запущенный экземпляр image;
- с собственным lifecycle, process tree, network и mounts.

## Что важно для Go-сервиса

### 1. Контейнер не должен тащить лишнее

Обычно production image для Go:
- multi-stage;
- минимальный runtime layer;
- только бинарь и необходимые runtime-файлы.

### 2. Логи лучше писать в stdout/stderr

Это стандартный путь для контейнерной среды:
- проще собирать через runtime и collector;
- не надо городить локальные log files внутри контейнера.

### 3. Graceful shutdown обязателен

Контейнеры регулярно:
- останавливаются;
- пересоздаются;
- выкатываются;
- мигрируют по нодам.

Поэтому Go-сервис должен корректно обрабатывать сигналы и завершать:
- HTTP server;
- background workers;
- DB connections;
- Kafka/Redis clients.

### 4. Container != VM

Частая ошибка:
- пытаться вести себя внутри контейнера так, будто это полноценная машина.

На практике:
- root filesystem часто ephemeral;
- локальные файлы могут исчезнуть при пересоздании;
- процессы и ресурсы ограничены cgroups и runtime config.

## Volumes

`Volumes` нужны, когда данные не должны жить только внутри контейнера.

Типичные use cases:
- postgres data;
- local dev cache;
- mounted config;
- live-reload в development.

Для stateless Go API volume обычно не нужен вообще, кроме dev-сценариев.

## Networks

Docker network дает сервисам:
- DNS-имена;
- связность между контейнерами;
- изоляцию от внешнего мира.

Практически:
- `api` может ходить в `postgres:5432`;
- `worker` может ходить в `redis:6379`;
- это особенно удобно в `docker compose`.

## ENV и runtime config

Обычно конфиг передают через:
- environment variables;
- mounted files;
- secrets management поверх оркестрации.

Плохая практика:
- зашивать environment-specific config прямо в образ.

## Что могут спросить на интервью

- чем image отличается от container;
- почему контейнер не равен VM;
- зачем сервису писать логи в stdout;
- как container shutdown влияет на Go runtime;
- когда нужны volumes, а когда нет.
