# Caching

Кэш — слой между приложением и медленным источником данных. Позволяет сократить latency и нагрузку на БД, ценой инфраструктурной сложности и риска стейл-данных.

## Материалы

- [01. Redis как кэш](./01-redis-as-cache.md) — cache-aside, TTL, инвалидация, cache stampede и singleflight, прогрев, метрики, когда НЕ кэшировать

## Что должен знать senior

- разница между cache-aside, write-through, write-behind
- как выбирать TTL и почему нужен jitter
- что такое cache stampede и три способа защиты (singleflight, probabilistic refresh, stale-while-revalidate)
- почему DEL безопаснее SET при инвалидации
- зачем версионировать ключи кэша при смене схемы
- как degradation на отказ Redis должен работать

## Связанные разделы

- [Redis cache testing](../../10-testing-and-quality/08-redis-and-cache-testing.md) — testcontainers, miniredis, FastForward для TTL
- [Background workers](../../04-architecture-and-patterns/patterns/04-background-workers.md) — async обновление кэша
- [Reliability patterns](../../05-system-design/reliability-patterns/) — circuit breaker для Redis fallback
