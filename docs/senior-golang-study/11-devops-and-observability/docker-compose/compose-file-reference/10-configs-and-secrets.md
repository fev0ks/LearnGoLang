# Configs And Secrets

`configs` и `secrets` позволяют доставлять данные в контейнер как files, а не только как env vars.

## Содержание

- [Top-level `configs`](#top-level-configs)
- [Service-level `configs`](#service-level-configs)
- [Top-level `secrets`](#top-level-secrets)
- [Service-level `secrets`](#service-level-secrets)
- [Build secrets](#build-secrets)
- [Когда использовать `configs`](#когда-использовать-configs)
- [Когда использовать `secrets`](#когда-использовать-secrets)
- [Practical rules](#practical-rules)

## Top-level `configs`

`configs` описывает источники config data, которые потом можно явно выдать сервисам.

Источник config может быть:
- `file`
- `environment`
- `content`
- `external`

Дополнительно:
- `name`

### Примеры

Из файла:

```yaml
configs:
  app_config:
    file: ./config/app.yaml
```

Из environment:

```yaml
configs:
  simple_config:
    environment: "SIMPLE_CONFIG_VALUE"
```

Inline content:

```yaml
configs:
  app_config:
    content: |
      debug=${DEBUG}
      app.name=${COMPOSE_PROJECT_NAME}
```

External config:

```yaml
configs:
  app_config:
    external: true
```

External config с явным lookup name:

```yaml
configs:
  app_config:
    external: true
    name: "${HTTP_CONFIG_KEY}"
```

Важный нюанс:
- если `external: true`, остальные атрибуты кроме `name` уже не имеют смысла.

## Service-level `configs`

Сервис получает доступ к config только если это явно указано.

### Short syntax

```yaml
services:
  api:
    configs:
      - app_config
```

По умолчанию config монтируется как файл:
- Linux: `/<config_name>`
- Windows: `C:\\<config_name>`

### Long syntax

```yaml
services:
  api:
    configs:
      - source: app_config
        target: /etc/app/config.yaml
        uid: "103"
        gid: "103"
        mode: 0440
```

Поля:
- `source`
- `target`
- `uid`
- `gid`
- `mode`

`mode` по умолчанию:
- `0444`

## Top-level `secrets`

`secrets` описывает чувствительные данные.

Источник секрета может быть:
- `file`
- `environment`

Примеры:

```yaml
secrets:
  db_password:
    file: ./secrets/db_password.txt
```

```yaml
secrets:
  oauth_token:
    environment: "OAUTH_TOKEN"
```

Compose монтирует secret в контейнер как file, обычно под:
- `/run/secrets/<secret_name>`

## Service-level `secrets`

Как и с `configs`, доступ надо выдавать явно.

### Short syntax

```yaml
services:
  api:
    secrets:
      - db_password
```

### Long syntax

```yaml
services:
  api:
    secrets:
      - source: db_password
        target: db_password
        uid: "103"
        gid: "103"
        mode: 0o440
```

Поля:
- `source`
- `target`
- `uid`
- `gid`
- `mode`

Практический нюанс:
- для secret с источником `file` Docker Compose не реализует remapping `uid`, `gid`, `mode` так же полноценно, как для platform-managed secret, потому что под капотом используется bind mount.

## Build secrets

Top-level `secrets` можно использовать и на этапе build:

```yaml
services:
  api:
    build:
      context: .
      secrets:
        - npm_token

secrets:
  npm_token:
    environment: NPM_TOKEN
```

Это лучше, чем:
- передавать секрет через `ARG`;
- зашивать токен в Dockerfile.

## Когда использовать `configs`

Подходит для:
- YAML, JSON, TOML конфигов;
- nginx, prometheus, otel config files;
- app config, который удобнее монтировать файлом.

## Когда использовать `secrets`

Подходит для:
- паролей;
- токенов;
- сертификатов;
- приватных ключей.

## Practical rules

- для local dev `env_file` часто проще и привычнее;
- когда секрет не должен болтаться в process environment, лучше `secrets`;
- для file-oriented приложений `configs` обычно понятнее, чем giant env var blob;
- top-level declaration сама по себе не дает доступ сервису: доступ нужно выдать явно через service-level `configs` или `secrets`.
