# Rate Limiting Examples

Здесь лежат локальные reference-реализации limiter'ов, чтобы не искать их в других проектах.

Состав:
- [Limiter Interface](./limiter.go)
- [Fixed Window In Memory](./fixed_window_memory.go)
- [Sliding Window In Memory](./sliding_window_memory.go)
- [Token Bucket In Memory](./token_bucket_memory.go)
- [Tests](./implementations_test.go)

Как использовать:
- читать вместе с [Rate Limiting](../rate-limiting.md);
- смотреть на contract `Decision` и `Limiter`;
- сравнивать fairness, burst behavior и сложность state management;
- держать в голове, что это in-memory reference implementations, а не distributed production limiters.

Что важно помнить:
- in-memory варианты хороши для понимания алгоритмов и unit tests;
- для multi-instance deployment обычно нужен Redis или другой shared atomic backend;
- особенно для `sliding window` и `token bucket` атомарность обновления состояния критична.

## Общая идея

Все примеры построены вокруг одного контракта:
- [`Decision`](./limiter.go) возвращает `Allowed` и `RetryAfter`;
- [`Limiter`](./limiter.go) скрывает конкретный алгоритм от middleware и handlers.

Это полезно по двум причинам:
- HTTP-слой не зависит от того, fixed window это, sliding window или token bucket;
- алгоритм можно сначала держать in-memory, а потом заменить на Redis-backed без переписывания boundary-кода.

Практически limiter почти всегда отвечает на три вопроса:
- кого ограничиваем: `ip`, `user`, `api_key`, `tenant`, `route`;
- насколько честным должно быть ограничение;
- готовы ли мы платить памятью и сложностью за более точный алгоритм.

## Быстрый выбор

`Fixed window`:
- самый простой и дешевый;
- подходит для грубого throttling;
- плохо ведет себя на границе окна.

`Sliding window`:
- честнее для клиента;
- дороже по памяти и CPU;
- хорошо подходит для anti-abuse и user-facing API.

`Token bucket`:
- обычно лучший production-компромисс;
- позволяет короткие burst;
- хорошо контролирует sustained traffic.

Если нет сильной причины выбрать другое, для API чаще всего начинают думать в сторону `token bucket`.

## 1. Limiter Interface

Файл: [limiter.go](./limiter.go)

Что здесь важно:
- интерфейс узкий;
- алгоритм можно подменять;
- `RetryAfter` уже встроен в результат и легко мапится на HTTP `429 Too Many Requests`.

Типичный middleware-flow:

```go
decision, err := limiter.Allow(ctx, key)
if err != nil {
	// fail-open или fail-closed
}
if !decision.Allowed {
	// HTTP 429 + Retry-After
}
```

Это хороший шаблон, потому что он не смешивает:
- бизнес-логику;
- HTTP-ответ;
- внутреннюю механику лимитера.

## 2. Fixed Window

Файл: [fixed_window_memory.go](./fixed_window_memory.go)

### Как работает

Окно фиксированное, например `1 minute`.
Для каждого ключа считаем количество попаданий в текущем bucket.

В примере:
- bucket считается через время и размер окна;
- если bucket сменился, счетчик сбрасывается;
- `RetryAfter` равен времени до конца текущего окна.

### Когда применять

Подходит, когда:
- нужен очень простой limiter;
- точность не критична;
- это internal/admin endpoint;
- нужен MVP или временная защита до более точной реализации.

Плохой выбор, когда:
- публичный API чувствителен к fairness;
- важны ровные ограничения без burst на границе окна;
- abuse может приходить волнами на стыке окон.

### Плюсы

- минимальная сложность;
- дешевый по CPU и памяти;
- очень легко переносится в Redis.

### Минусы

- boundary burst problem;
- лимит "календарный", а не по реально последним `N` секундам;
- не самый честный алгоритм для клиентов.

### Redis-псевдокод

Идея ключа:

```text
ratelimit:{subject}:{window_bucket}
```

Упрощенный flow:

```text
bucket = floor(now / window)
key = "ratelimit:" + subject + ":" + bucket

count = INCR key
if count == 1:
  EXPIRE key window_seconds

if count > limit:
  deny with retry_after = seconds_until_window_end
else:
  allow
```

Пример команд Redis:

```text
INCR ratelimit:user:42:28517920
EXPIRE ratelimit:user:42:28517920 60
TTL ratelimit:user:42:28517920
```

Для production лучше делать это атомарно через Lua, чтобы `INCR` и `EXPIRE` не расходились.

## 3. Sliding Window

Файл: [sliding_window_memory.go](./sliding_window_memory.go)

### Как работает

Смотрим не на текущую "минуту по календарю", а на последние `N` секунд от текущего момента.

В примере:
- на каждый запрос удаляются старые timestamps;
- если внутри окна уже есть `limit` событий, запрос блокируется;
- `RetryAfter` считается по самому старому событию, которое еще не вышло из окна.

### Когда применять

Подходит, когда:
- нужна более честная защита для клиентов;
- лимит виден наружу и его поведение должно быть предсказуемым;
- важно убрать burst на границе окна.

Плохой выбор, когда:
- очень высокий QPS и нужно дешевое решение;
- память и операции на каждый hit критичны;
- fairness не так важен, как простота.

### Плюсы

- заметно честнее fixed window;
- нет резкого скачка на стыке окон;
- удобно объяснять клиентское поведение.

### Минусы

- дороже fixed window;
- in-memory реализация хранит timestamps;
- Redis-реализация обычно требует `sorted set` и Lua.

### Redis-псевдокод

Идея ключа:

```text
ratelimit:{subject}:events
```

Упрощенный flow:

```text
key = "ratelimit:" + subject + ":events"
cutoff = now_ms - window_ms

ZREMRANGEBYSCORE key -inf cutoff
count = ZCARD key

if count >= limit:
  oldest = ZRANGE key 0 0 WITHSCORES
  deny with retry_after = oldest + window - now
else:
  ZADD key now_ms request_id
  EXPIRE key window_seconds
  allow
```

Пример команд Redis:

```text
ZREMRANGEBYSCORE ratelimit:user:42:events -inf 1712563140000
ZCARD ratelimit:user:42:events
ZRANGE ratelimit:user:42:events 0 0 WITHSCORES
ZADD ratelimit:user:42:events 1712563200000 req-123
EXPIRE ratelimit:user:42:events 60
```

На практике это почти всегда оборачивают в Lua script, чтобы prune, count, add и расчет `retry_after` были единым атомарным действием.

## 4. Token Bucket

Файл: [token_bucket_memory.go](./token_bucket_memory.go)

### Как работает

У каждого ключа есть bucket:
- с максимальной емкостью `capacity`;
- с пополнением `refillRate` токенов в секунду.

В примере:
- при каждом запросе сначала пересчитываются токены по elapsed time;
- если токен есть, он списывается;
- если токенов нет, возвращается `RetryAfter` до ближайшего пополнения.

### Когда применять

Подходит, когда:
- нужны небольшие burst;
- долгий поток нужно держать ровным;
- limiter стоит перед API или edge endpoint;
- важно понятное поведение для sustained traffic.

Плохой выбор, когда:
- нужна максимально простая реализация "на вчера";
- команда не хочет поддерживать более сложную state machine;
- в distributed-варианте нет надежной атомарности.

### Плюсы

- хороший баланс между fairness и practical burst handling;
- часто лучше всего ложится на API semantics;
- `RetryAfter` получается естественно.

### Минусы

- сложнее fixed window;
- требует аккуратной работы со временем;
- при distributed-состоянии нужен atomic backend.

### Redis-псевдокод

Идея ключа:

```text
ratelimit:{subject}:bucket
```

Храним:
- `tokens`;
- `last_refill_ms`.

Упрощенный flow:

```text
state = HMGET key tokens last_refill_ms
if state missing:
  tokens = capacity
  last_refill_ms = now_ms

elapsed = now_ms - last_refill_ms
tokens = min(capacity, tokens + elapsed * refill_rate_per_ms)

if tokens >= 1:
  tokens = tokens - 1
  HMSET key tokens tokens last_refill_ms now_ms
  EXPIRE key ttl_seconds
  allow
else:
  retry_after = ceil((1 - tokens) / refill_rate_per_ms)
  HMSET key tokens tokens last_refill_ms now_ms
  EXPIRE key ttl_seconds
  deny
```

Пример команд Redis:

```text
HMGET ratelimit:user:42:bucket tokens last_refill_ms
HMSET ratelimit:user:42:bucket tokens 3.4 last_refill_ms 1712563200000
EXPIRE ratelimit:user:42:bucket 120
```

Здесь почти всегда нужен Lua script, потому что обычный `HMGET -> calc -> HMSET` под конкуренцией легко приведет к oversell токенов.

## In-Memory vs Redis

### In-memory

Лучше применять, когда:
- это local dev;
- нужны unit tests;
- сервис один и не масштабируется горизонтально;
- задача учебная или временная.

Не подходит, когда:
- трафик идет через несколько pod/replica;
- лимит должен быть общим для tenant или API key;
- после рестарта нельзя терять состояние.

### Redis-backed

Лучше применять, когда:
- лимит общий для всех реплик;
- нужен shared state;
- limiter стоит в критичном API path и должен работать одинаково на всех инстансах.

Нужно помнить:
- Redis становится частью critical path;
- нужен timeout;
- нужна стратегия fail-open или fail-closed;
- нужны метрики по latency, errors, denied count и hot keys.

## Что обычно выбрать

`Fixed window`:
- для простого throttling;
- для внутренних инструментов;
- для дешевой первой версии.

`Sliding window`:
- для anti-abuse;
- для user-facing API, где важна честность;
- когда window-edge burst нежелателен.

`Token bucket`:
- для публичного API;
- для edge limiting;
- для случаев, где допустим небольшой burst, но нужен контроль sustained rate.

## Что смотреть в тестах

Файл: [implementations_test.go](./implementations_test.go)

Проверять стоит не только happy path:
- блокировку после достижения лимита;
- корректный `RetryAfter`;
- поведение после смены окна;
- refill токенов по времени;
- race behavior, если реализация станет concurrent-heavy.

## Что важно уметь проговорить на интервью

- почему fixed window прост, но не честен;
- почему sliding window дороже, но предсказуемее для клиента;
- почему token bucket часто самый practical production choice;
- почему Redis limiter почти всегда должен быть атомарным;
- когда limiter должен fail-open, а когда fail-closed.
