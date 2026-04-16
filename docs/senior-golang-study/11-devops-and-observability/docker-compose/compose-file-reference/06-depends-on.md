# Depends On

`depends_on` задает зависимости между сервисами и влияет на порядок старта и остановки.

## Содержание

- [Короткий синтаксис](#короткий-синтаксис)
- [Полный синтаксис](#полный-синтаксис)
- [Поля](#поля)
- [Когда какой `condition` выбирать](#когда-какой-condition-выбирать)
- [Типовые примеры](#типовые-примеры)
- [Practical caveat](#practical-caveat)
- [Короткое правило выбора](#короткое-правило-выбора)

## Короткий синтаксис

```yaml
depends_on:
  - postgres
  - redis
```

Что это означает:
- сервис зависит от `postgres` и `redis`;
- Compose создает и удаляет сервисы в dependency order;
- по смыслу это близко к `condition: service_started`.

Short syntax не ждет полноценной readiness:
- dependency контейнер уже запущен;
- но сервис внутри него может еще не принимать соединения.

## Полный синтаксис

```yaml
depends_on:
  postgres:
    condition: service_healthy
    restart: true
    required: true
  migrator:
    condition: service_completed_successfully
```

## Поля

### `condition`

Определяет, когда dependency считается удовлетворенной.

Актуальные значения:

`service_started`:
- dependency просто должна стартовать;
- это самый слабый вариант.

`service_healthy`:
- dependency должна стать healthy;
- требует `healthcheck` у dependency service.

`service_completed_successfully`:
- dependency должна успешно завершиться;
- хорошо подходит для one-shot jobs.

### `restart`

```yaml
restart: true
```

Что это значит:
- если Compose явно перезапускает dependency service;
- dependent service тоже будет перезапущен.

Допустимые значения:
- `true`
- `false`

Важно:
- это про explicit Compose operations;
- это не "магический автоперезапуск всего графа" на любой runtime failure.

### `required`

```yaml
required: false
```

Что это значит:
- dependency считается optional;
- если ее нет, Compose предупреждает, но не блокирует запуск жестко.

Допустимые значения:
- `true`
- `false`

По умолчанию:
- `required: true`

## Когда какой `condition` выбирать

`service_started`:
- только если достаточно факта старта процесса;
- обычно для простых и не критичных зависимостей.

`service_healthy`:
- основной practical default для DB, cache, broker и HTTP services;
- обычно лучший выбор для local dev stack.

`service_completed_successfully`:
- для `migrator`, `seed`, `init`, `bootstrap` сервисов;
- когда основной сервис не должен стартовать, пока job не завершился успешно.

## Типовые примеры

### API зависит от Postgres и Redis

```yaml
depends_on:
  postgres:
    condition: service_healthy
  redis:
    condition: service_healthy
```

### API зависит от мигратора

```yaml
depends_on:
  migrator:
    condition: service_completed_successfully
```

### Optional сервис

```yaml
depends_on:
  jaeger:
    condition: service_started
    required: false
```

## Practical caveat

`depends_on` не заменяет resilience внутри приложения.

Go-сервису все равно нужны:
- retry;
- timeouts;
- нормальный startup behavior;
- понятный health endpoint.

## Короткое правило выбора

Для local Go-стека обычно достаточно такого heuristics:
- база, кэш, брокер: `service_healthy`
- мигратор, seed job: `service_completed_successfully`
- optional debug tooling: `service_started` и `required: false`
