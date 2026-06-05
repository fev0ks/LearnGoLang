# Healthcheck

`healthcheck` говорит Docker, как понять, что контейнер жив и готов.

## Содержание

- [Базовый пример](#базовый-пример)
- [Поля](#поля)
- [Когда использовать `CMD` vs `CMD-SHELL`](#когда-использовать-cmd-vs-cmd-shell)
- [Типовые примеры](#типовые-примеры)
- [Practical rule](#practical-rule)
- [Что обычно проверяют](#что-обычно-проверяют)

## Базовый пример

```yaml
healthcheck:
  test: ["CMD", "redis-cli", "ping"]
  interval: 5s
  timeout: 3s
  retries: 20
```

## Поля

### `test`

Главное поле проверки.

Частые формы:

```yaml
test: ["CMD", "redis-cli", "ping"]
```

```yaml
test: ["CMD-SHELL", "pg_isready -U app -d shortener"]
```

```yaml
test: ["NONE"]
```

```yaml
test: curl -f http://localhost:8080/healthz || exit 1
```

Что это значит:
- `CMD` запускает команду напрямую;
- `CMD-SHELL` запускает через shell;
- строковая форма эквивалентна `CMD-SHELL`;
- `NONE` отключает healthcheck.

Есть еще отдельный способ отключить inherited healthcheck:

```yaml
healthcheck:
  disable: true
```

Это полезно, если базовый image уже содержит `HEALTHCHECK`, но для твоего сервиса он не подходит.

### `interval`

Как часто запускать проверку.

Пример:

```yaml
interval: 10s
```

Это duration string.

Типичные значения:
- `5s`
- `10s`
- `30s`
- `1m30s`

### `timeout`

Сколько максимум ждать завершения одной проверки.

Пример:

```yaml
timeout: 3s
```

### `retries`

Сколько неудачных попыток подряд нужно, чтобы контейнер считался unhealthy.

Пример:

```yaml
retries: 10
```

### `start_period`

Льготный период на старте контейнера, когда неудачные проверки не считаются фатальными.

Пример:

```yaml
start_period: 20s
```

Полезно для:
- медленно стартующих приложений;
- тяжелых DB;
- сервисов, которые долго прогреваются.

### `start_interval`

Более частый интервал проверок именно на раннем этапе старта.

Полезно:
- когда хочется быстро поймать readiness на старте;
- но не держать такой же частый polling потом постоянно.

Нужно помнить:
- поддержка зависит от актуальной Compose и Engine версии;
- это менее базовый параметр, чем `interval`, `timeout`, `retries`, `start_period`.

### `disable`

```yaml
disable: true
```

Допустимые значения:
- `true`
- `false`

Полезно, когда:
- image уже содержит неудачный `HEALTHCHECK`;
- ты хочешь полностью отключить inherited проверку.

## Когда использовать `CMD` vs `CMD-SHELL`

`CMD`:
- лучше для простых команд;
- меньше shell-specific поведения.

`CMD-SHELL`:
- удобен для shell expressions;
- полезен, когда нужен pipe, логика или shell builtins.

## Типовые примеры

### Go API

```yaml
healthcheck:
  test: ["CMD", "wget", "-qO-", "http://localhost:8080/healthz"]
  interval: 10s
  timeout: 3s
  retries: 10
```

### Postgres

```yaml
healthcheck:
  test: ["CMD-SHELL", "pg_isready -U app -d shortener"]
  interval: 5s
  timeout: 3s
  retries: 20
```

### Redis

```yaml
healthcheck:
  test: ["CMD", "redis-cli", "ping"]
  interval: 5s
  timeout: 3s
  retries: 20
```

## Practical rule

Если сервис важен как dependency:
- почти всегда стоит добавить `healthcheck`;
- иначе `depends_on: service_healthy` не сможет работать как надо.

## Что обычно проверяют

Go API:
- `GET /healthz`
- `GET /readyz`

Postgres:
- `pg_isready`

Redis:
- `redis-cli ping`

Broker:
- очень простой ping или metadata call, если у клиента есть удобная CLI-команда.
