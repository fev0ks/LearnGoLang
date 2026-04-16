# Docker Compose And Container Secrets

Этот файл про практический вопрос: как передавать секреты в контейнеры, особенно в local dev и production-like container setups.

## Самая частая ошибка

Вот так делать плохо:

```yaml
environment:
  DB_PASSWORD: super-secret-prod-password
  JWT_SECRET: prod-jwt-secret
```

Проблемы:
- секрет прямо лежит в compose-файле;
- его легко случайно закоммитить;
- он будет жить в истории git;
- его увидит каждый, у кого есть доступ к репозиторию.

## Что можно делать в local dev

### `env_file`

Пример:

```yaml
services:
  api:
    env_file:
      - .env.local
```

Плюсы:
- удобно;
- compose-файл остается чистым;
- local значения можно держать вне git.

Минусы:
- это local convenience, а не полноценная production secret strategy.

### `environment` + подстановка из shell

Пример:

```yaml
environment:
  DB_PASSWORD: ${DB_PASSWORD}
  JWT_SECRET: ${JWT_SECRET}
```

Плюсы:
- compose не хранит сами значения;
- можно подставлять из shell, `direnv`, CI variables.

Минусы:
- секрет все еще идет через env;
- удобство зависит от shell setup.

### Mounted secret files

Пример:

```yaml
volumes:
  - ./secrets/dev-jwt.pem:/run/secrets/jwt.pem:ro
```

Это особенно полезно для:
- certs;
- private keys;
- multiline secrets.

## Когда env vars нормальны

Для local dev и части production setups env vars это нормальный practical choice, если:
- секреты не зашиты в image;
- значения не лежат в git;
- логирование и debug tooling не сливают env;
- секреты можно заменять при deploy.

## Когда env vars уже не лучший вариант

- большие multiline secrets;
- TLS private keys;
- JSON credentials;
- секреты, которые хочется меньше светить в process environment;
- сценарии с частой ротацией и runtime refresh.

В этих случаях файлы или внешний secret manager часто лучше.

## Compose `secrets`

У `docker compose` есть отдельная концепция `secrets`, но на практике:
- в local dev многие команды чаще используют `env_file` или mount files;
- для production orchestration обычно уходят в `Kubernetes`, Swarm или внешний secret manager.

То есть знать `secrets:` полезно, но не стоит строить на этом единственную mental model.

## Practical rule

Для local compose:
- `.env.local` в `.gitignore`;
- `env_file` или `${VAR}` substitutions;
- file mounts для certs/keys;
- никаких реальных production secrets в compose yaml.

Для production-like containers:
- секреты должны приходить при deploy/run time;
- image не должен содержать секреты;
- лучше отделять config, artifact и secrets.

## Что могут спросить на интервью

- почему плохо держать секреты прямо в compose yaml;
- чем `env_file` отличается от хардкода;
- когда лучше использовать mounted file вместо env var;
- почему container image должен быть одинаковым для разных сред, а секреты нет.
