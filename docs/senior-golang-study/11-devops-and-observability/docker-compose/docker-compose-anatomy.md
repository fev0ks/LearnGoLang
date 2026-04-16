# Docker Compose Anatomy

Эта заметка теперь служит обзором раздела. Подробные параметры и допустимые значения вынесены в [`compose-file-reference`](./compose-file-reference/README.md), чтобы anatomy не превращался в свалку из YAML-опций.

## Что такое Docker Compose

`docker compose` описывает локальное multi-service окружение в одном `compose.yaml`.

Обычно через него поднимают:
- Go API и worker;
- Postgres, Redis, Kafka или Redpanda;
- one-shot job вроде мигратора;
- optional tooling вроде Grafana, Prometheus, Jaeger.

Практический смысл:
- вместо набора длинных `docker run` у команды один декларативный файл;
- onboarding и local dev становятся воспроизводимыми;
- integration testing проще запускать в одинаковом окружении.

## Базовая модель Compose файла

Если упростить, compose-файл отвечает на пять вопросов:

- какие роли надо запустить: `services`
- как они видят друг друга: `networks`
- какие данные надо сохранить: `volumes`
- как доставить конфиг и секреты: `configs`, `secrets`
- как назвать проект и заизолировать ресурсы: `name`

Минимальный каркас:

```yaml
name: shortener

services:
  api:
    build: .

networks:
  default:
    driver: bridge

volumes:
  pg-data:
```

## Как читать Compose на Go-проекте

Обычно я читаю файл в таком порядке:

1. `services`
Посмотреть, какие runtime-роли вообще существуют: `api`, `worker`, `migrator`, `postgres`, `redis`.

2. Конфигурация сервиса
Понять, откуда берется образ, какая команда стартует контейнер, какие env vars и порты заданы.

3. Dependencies и readiness
Проверить `depends_on`, `healthcheck`, `restart`, чтобы понять startup story.

4. Storage и networking
Посмотреть `volumes` и `networks`, чтобы понять, где сохраняются данные и какие DNS-имена используются между контейнерами.

5. Optional tooling
Проверить `profiles`, чтобы понять, что является always-on, а что включается только по запросу.

## Что важно помнить

`services`:
- имя сервиса не специальное слово, а просто logical role name;
- оно же обычно становится DNS-именем внутри сети;
- поэтому `postgres` в compose часто означает, что приложение ходит в `postgres:5432`.

`depends_on`:
- помогает с порядком старта;
- не заменяет retry logic в приложении.

`healthcheck`:
- нужен, если хочешь осмысленно ждать `service_healthy`;
- без него dependency часто считается "стартовавшей", но еще не готовой.

`ports`:
- нужны только когда сервис должен быть доступен с хоста;
- для общения контейнеров между собой они обычно не нужны.

`volumes`:
- bind mount чаще для исходников и dev workflow;
- named volume чаще для данных Postgres, Redis и других stateful сервисов.

`profiles`:
- помогают держать один compose-файл на несколько сценариев;
- тяжелое tooling удобно выносить в optional profiles.

## Что не решает Compose

Compose не заменяет production orchestration.

Он не решает полноценно:
- rolling updates;
- cross-host scheduling;
- self-healing кластера;
- secret management production-уровня;
- policy-driven service discovery и ingress.

Поэтому для local dev и небольших integration-сценариев compose отличен, но это не замена `Kubernetes`.

## Куда идти за деталями

Подробный reference по секциям файла:

- [`01 Top-Level Structure`](./compose-file-reference/01-top-level-structure.md)
- [`02 Service Definition`](./compose-file-reference/02-service-definition.md)
- [`03 Build And Image`](./compose-file-reference/03-build-and-image.md)
- [`04 Environment And Env File`](./compose-file-reference/04-environment-and-env-file.md)
- [`05 Ports And Expose`](./compose-file-reference/05-ports-and-expose.md)
- [`06 Depends On`](./compose-file-reference/06-depends-on.md)
- [`07 Healthcheck`](./compose-file-reference/07-healthcheck.md)
- [`08 Networks`](./compose-file-reference/08-networks.md)
- [`09 Volumes`](./compose-file-reference/09-volumes.md)
- [`10 Configs And Secrets`](./compose-file-reference/10-configs-and-secrets.md)
- [`11 Command Entrypoint And Restart`](./compose-file-reference/11-command-entrypoint-and-restart.md)
- [`12 Profiles`](./compose-file-reference/12-profiles.md)

После этого лучше вернуться к [`compose-go-stack.example.yaml`](./compose-go-stack.example.yaml) и читать уже реальный пример целиком.
