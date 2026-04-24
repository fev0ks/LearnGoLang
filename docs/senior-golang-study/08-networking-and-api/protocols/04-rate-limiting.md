# Rate Limiting

Rate limiting почти всегда выглядит простой темой только до первого production-инцидента. На senior-уровне важно уметь объяснить не только алгоритм, но и:
- где limiter должен жить;
- как он ведет себя в multi-instance deployment;
- что произойдет при Redis outage;
- насколько решение честное по отношению к клиентам и burst traffic.

## Содержание

- [Где это применять](#где-это-применять)
- [Пример контракта](#пример-контракта)
- [1. Fixed Window](#1-fixed-window)
- [2. Sliding Window](#2-sliding-window)
- [3. Token Bucket](#3-token-bucket)
- [In-Memory vs Redis](#in-memory-vs-redis)
- [Что выбрать на практике](#что-выбрать-на-практике)
- [Redis-specific trade-offs](#redis-specific-trade-offs)
- [Что мониторить](#что-мониторить)
- [Что могут спросить на интервью](#что-могут-спросить-на-интервью)
- [Связанные темы](#связанные-темы)

## Где это применять

Частые сценарии:
- ограничение запросов по IP на public endpoint;
- ограничение по API key, user ID, tenant ID;
- защита login, password reset, OTP, webhook endpoints;
- ограничение дорогих операций вроде shorten/generate/export/search;
- quota для внешних интеграций и платных тарифов.

Практически rate limiter отвечает на два вопроса:
- можно ли пропустить запрос сейчас;
- если нельзя, когда можно повторить.

Именно поэтому contract вида `Allowed + RetryAfter` обычно удачнее, чем просто `bool`.

## Пример контракта

В `sandbox_url_shortener` limiter вынесен в узкий интерфейс:

```go
type Decision struct {
	Allowed    bool
	RetryAfter time.Duration
}

type Limiter interface {
	Allow(ctx context.Context, key string) (Decision, error)
}
```

Почему это хороший дизайн:
- middleware не знает, fixed window это, Redis или token bucket;
- boundary сам решает fail-open или fail-closed;
- алгоритмы можно менять без переписывания HTTP слоя.

Локальные примеры лежат рядом с этой заметкой:
- [Limiter Interface](./rate-limiting-examples/limiter.go)
- [Fixed Window](./rate-limiting-examples/fixed_window_memory.go)
- [Sliding Window](./rate-limiting-examples/sliding_window_memory.go)
- [Token Bucket](./rate-limiting-examples/token_bucket_memory.go)
- [Tests](./rate-limiting-examples/implementations_test.go)

## 1. Fixed Window

### Идея

Берем окно, например `1 minute`, и считаем запросы внутри него.

Если лимит `100 req/min`, то:
- в текущей минуте можно пропустить первые 100;
- 101-й блокируем до начала следующего окна.

### Почему это просто

Это самый дешевый и простой вариант:
- мало состояния;
- легко реализовать в памяти;
- легко положить в Redis через счетчик и TTL.

### Пример из твоего кода

У тебя in-memory fixed window устроен через:
- `bucket = now.UnixNano() / window`;
- `count` на ключ внутри bucket;
- `RetryAfter` до конца окна.

Упрощенная идея:

```go
bucket := now.UnixNano() / window.Nanoseconds()
st := state[key]
if st.bucket != bucket {
	st = fixedWindowState{bucket: bucket}
}
st.count++

if st.count > limit {
	return Decision{Allowed: false, RetryAfter: untilWindowEnd(now)}, nil
}
```

### Плюсы

- минимальная сложность;
- хороший вариант для MVP, admin endpoints, внутренних сервисов;
- Redis-реализация очень дешевая.

### Минусы

- burst problem на границе окна;
- клиент может сделать лимит в конце окна и еще раз сразу в начале следующего;
- "честность" для клиентов хуже, чем у sliding window.

### Как это делают в Redis

Обычно:
- ключ вида `ratelimit:{user}:{bucket}`;
- `INCR`;
- при первом запросе `EXPIRE` на длину окна.

Проблема:
- если делать `INCR` и `EXPIRE` неатомарно, можно получить неконсистентность.

Поэтому в production часто используют:
- Lua script;
- или `MULTI/EXEC`, если хватает его гарантий;
- или готовую библиотеку, которая уже закрывает race conditions.

### Когда подходит

- простые лимиты per-IP;
- coarse throttling;
- когда важнее дешевизна, чем fairness.

## 2. Sliding Window

### Идея

Считаем не "текущую календарную минуту", а последние `N` секунд от текущего момента.

Если лимит `100 / 60s`, то на каждом запросе нужно знать:
- сколько событий было за последние 60 секунд;
- когда старейшее из них выйдет из окна.

### Пример из твоего кода

Твой in-memory вариант хранит список timestamps на ключ:
- старые hits вычищаются;
- если после prune осталось `>= limit`, запрос блокируется;
- `RetryAfter` считается от самого старого события, которое еще держит окно.

Упрощенная идея:

```go
cutoff := now.Add(-window)
hits := pruneOld(hits[key], cutoff)

if len(hits) >= limit {
	return Decision{
		Allowed:    false,
		RetryAfter: hits[0].Add(window).Sub(now),
	}, nil
}

hits = append(hits, now)
```

### Плюсы

- заметно честнее fixed window;
- нет резкого window-boundary burst;
- лучше подходит для user-facing API, где важна предсказуемость.

### Минусы

- дороже по CPU и памяти;
- наивная реализация плохо масштабируется на высоком QPS;
- в Redis требует более сложной структуры данных.

### Как это делают в Redis

Самый типичный вариант:
- sorted set на ключ;
- score = timestamp;
- перед проверкой удалить записи старше окна;
- посмотреть `ZCARD`;
- если лимит не превышен, добавить новый timestamp;
- выставить TTL.

Обычно это делают через Lua, чтобы весь цикл был атомарным:
- prune old events;
- count current events;
- conditionally add current event;
- вернуть `allowed` и `retry_after`.

Цена такого подхода:
- больше памяти на каждый hit;
- больше нагрузки на Redis;
- нужен контроль cardinality и TTL, иначе ключи будут пухнуть.

### Когда подходит

- API gateway;
- публичные методы с требованием fairness;
- anti-abuse сценарии, где burst на границе окна нежелателен.

## 3. Token Bucket

### Идея

Есть bucket емкостью `capacity`, который пополняется со скоростью `refillRate`.

Каждый запрос:
- забирает токен, если он есть;
- блокируется, если токенов нет;
- `RetryAfter` равен времени до следующего токена.

### Пример из твоего кода

Твой in-memory token bucket хранит:
- текущее число токенов;
- `last` момент последнего refill/calc.

На каждом запросе:

```go
elapsed := now.Sub(st.last).Seconds()
st.tokens += elapsed * refillRate
if st.tokens > capacity {
	st.tokens = capacity
}

if st.tokens >= 1 {
	st.tokens--
	return Decision{Allowed: true}, nil
}
```

### Плюсы

- лучший компромисс между burst handling и ровным sustained traffic;
- естественно моделирует "разрешаем небольшие всплески, но не бесконечно";
- часто самый практичный production-вариант для API.

### Минусы

- сложнее fixed window;
- нужны аккуратные расчеты времени и атомарность;
- распределенная реализация без Redis script или другого atomic backend быстро ломается.

### Как это делают в Redis

Обычно в ключе хранят:
- текущее число токенов;
- timestamp последнего обновления.

Дальше атомарно:
- читают состояние;
- пересчитывают refill по elapsed time;
- решают allow/deny;
- обновляют токены и timestamp;
- возвращают `retry_after`.

На практике это почти всегда Lua script.

Почему не обычный `GET`/`SET`:
- concurrent requests от нескольких реплик легко перетрут состояние друг друга;
- появится oversell токенов и limiter станет мягче, чем ожидалось.

### Когда подходит

- edge throttling;
- per-client API limits;
- сервисы, где допустим короткий burst, но важен ровный долгий поток;
- лимиты на дорогие операции, где нужен понятный `Retry-After`.

## In-Memory vs Redis

### In-memory limiter

Плюсы:
- минимальная latency;
- нулевая внешняя зависимость;
- отличный вариант для unit tests, local dev, single-instance apps.

Минусы:
- state process-local;
- в multi-instance deployment каждый pod лимитирует независимо;
- после рестарта состояние теряется;
- нельзя честно ограничивать общий tenant-wide traffic.

### Redis-backed limiter

Плюсы:
- общее состояние для всех реплик;
- лучше подходит для API gateway и distributed systems;
- TTL и atomic scripts хорошо ложатся на rate limit use case.

Минусы:
- появляется network hop;
- Redis становится частью critical path;
- нужны timeout, fallback policy и observability;
- под высоким QPS limiter сам может стать bottleneck.

## Что выбрать на практике

### Fixed window

Бери, когда:
- нужен простой и дешевый limiter;
- допустим burst на границе окна;
- лимит coarse-grained и business risk небольшой.

### Sliding window

Бери, когда:
- нужна честность для клиента;
- boundary bursts неприемлемы;
- можно заплатить за более дорогую реализацию.

### Token bucket

Бери, когда:
- нужно разрешать небольшие burst;
- хочется контролировать sustained rate;
- нужен удобный и понятный `Retry-After`.

Если нужно выбрать "дефолтно production-friendly вариант", чаще всего это token bucket.

## Redis-specific trade-offs

На senior-интервью полезно проговорить:
- limiter в Redis должен быть атомарным;
- TTL обязателен, иначе ключи и структуры будут накапливаться;
- важно выбрать ключ: `ip`, `user`, `api_key`, `tenant`, `route`, их комбинация;
- нужно решить, что делать при недоступности Redis: fail-open или fail-closed;
- стоит отделить жесткие security limits от мягких traffic shaping limits.

Типичный компромисс:
- login/OTP/abuse protection часто fail-closed;
- обычные public read APIs чаще fail-open, чтобы не превратить Redis outage в полный outage продукта.

## Что мониторить

- количество `429`;
- долю denied requests по ключам и endpoint'ам;
- latency самого limiter;
- ошибки Redis/Lua;
- cardinality ключей;
- memory usage Redis;
- skew по tenants и hot keys.

## Что могут спросить на интервью

- почему fixed window плохо ведет себя на границе окна;
- чем sliding window честнее, но дороже;
- почему token bucket часто удобнее для API;
- как реализовать limiter в Redis без race conditions;
- когда limiter должен fail-open, а когда fail-closed.

## Связанные темы

- [Networking And API README](./README.md)
- [Redis Docs](https://redis.io/docs/latest/)
