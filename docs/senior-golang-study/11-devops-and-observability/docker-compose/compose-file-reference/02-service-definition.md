# Service Definition

`services` это ядро compose-файла. Каждый entry внутри `services` описывает одну runtime-роль.

Пример:

```yaml
services:
  api:
    build:
      context: .
      dockerfile: Dockerfile
    environment:
      APP_ENV: local
    ports:
      - "8080:8080"
```

## Что важно понимать про имя сервиса

Ключ внутри `services` это не специальное слово и не "тип сервиса". Это просто имя роли.

Например:

```yaml
services:
  api:
  worker:
  postgres:
```

Практически имя сервиса часто играет сразу несколько ролей:
- logical role name в compose-модели;
- часть имени контейнера и других ресурсов;
- DNS-имя внутри compose network.

Поэтому если сервис называется `postgres`, то другой контейнер обычно ходит в `postgres:5432`.

## Базовый skeleton сервиса

```yaml
services:
  api:
    build:
      context: .
    command: ["./app"]
    environment:
      APP_ENV: local
    ports:
      - "127.0.0.1:8080:8080"
    depends_on:
      postgres:
        condition: service_healthy
    networks:
      - app-net
```

## Самые частые service keys

### `image`

Использует готовый образ.

Типично:
- `postgres:16-alpine`
- `redis:7-alpine`
- `grafana/grafana`

Подробно:
- [Build And Image](./03-build-and-image.md)

### `build`

Позволяет собрать образ из исходников или из Git context.

Подробно:
- [Build And Image](./03-build-and-image.md)

### `environment`, `env_file`

Определяют env vars контейнера.

Подробно:
- [Environment And Env File](./04-environment-and-env-file.md)

### `ports`, `expose`

Определяют, какие порты доступны хосту или только другим контейнерам.

Подробно:
- [Ports And Expose](./05-ports-and-expose.md)

### `depends_on`

Определяет зависимости и условия старта.

Подробно:
- [Depends On](./06-depends-on.md)

### `healthcheck`

Определяет, как Docker понимает, что контейнер healthy.

Подробно:
- [Healthcheck](./07-healthcheck.md)

### `networks`

Подключает сервис к одной или нескольким сетям.

Подробно:
- [Networks](./08-networks.md)

### `volumes`

Монтирует bind mounts, named volumes, tmpfs и другие mount types.

Подробно:
- [Volumes](./09-volumes.md)

### `configs`, `secrets`

Дают сервису доступ к mounted config files и secrets.

Подробно:
- [Configs And Secrets](./10-configs-and-secrets.md)

### `command`, `entrypoint`, `restart`

Управляют startup behavior контейнера.

Подробно:
- [Command Entrypoint And Restart](./11-command-entrypoint-and-restart.md)

### `profiles`

Делают сервис optional.

Подробно:
- [Profiles](./12-profiles.md)

## Еще несколько полезных service keys

### `container_name`

Явно задает имя контейнера.

```yaml
container_name: shortener-api
```

Полезно:
- для локальной отладки;
- когда хочется совсем явное имя.

Минус:
- ухудшает гибкость project scoping;
- чаще мешает запускать несколько одинаковых стеков параллельно.

### `hostname`

Явно задает hostname внутри контейнера.

В обычном local dev нужен редко. Чаще хватает service name и обычного Docker DNS.

### `platform`

Задает target platform, например:

```yaml
platform: linux/amd64
platform: linux/arm64/v8
```

Полезно:
- при разработке на ARM Mac, когда зависимость или образ ожидает другой target;
- когда хочешь стабилизировать поведение build/pull.

### `user`

Переопределяет пользователя, от которого запускается процесс контейнера.

Полезно:
- когда важны права на mounted files;
- когда не хочется запускать приложение от `root`.

### `working_dir`

Переопределяет рабочую директорию контейнера, аналог `WORKDIR` из Dockerfile.

### `read_only`

Делает root filesystem контейнера read-only.

Полезно:
- как security hardening;
- когда хочешь явно отделить write paths в `tmpfs` или volume.

## Practical rule

Читать service definition лучше так:
- откуда берется образ;
- что реально стартует;
- какие env vars нужны;
- какие зависимости и healthchecks стоят;
- какие порты публикуются наружу;
- какие volumes и networks подключены.
