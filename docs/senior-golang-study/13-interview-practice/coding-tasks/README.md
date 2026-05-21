# Coding Tasks

Задачи на live-coding, которые часто просят на собеседованиях для senior Go разработчиков.

## Категории

- [Concurrency](./concurrency/) — горутины, каналы, синхронизация. **Самое частое для Go.**
- [Data Structures](./data-structures/) — LRU cache, top-K (heap), bloom filter, trie, sliding window counter
- _System Primitives_ _(planned)_ — connection pool, retry, circuit breaker, distributed lock
- _Streams_ _(planned)_ — дедупликация, batching

## Шаблон задачи

Каждая задача оформлена так:

```
1. Формулировка задачи (как могут спросить)
2. Уточняющие вопросы (signal seniority — узнать constraints перед кодом)
3. Базовое решение (минимальное MVP)
4. Production-grade решение (расширенное с context, метриками, backpressure)
5. Тесты (минимум — пример)
6. Подводные камни (типичные ошибки)
7. Возможные расширения (что ещё могут попросить)
8. Связки с другими темами материалов
```

## Как тренироваться

**Шаг 1: Прочитай формулировку, не смотри решение.**
Засеки время. Попробуй сам — на бумаге или в редакторе.

**Шаг 2: Сравни своё решение с базовым.**
Не код-к-коду, а **подход**. Что упустил? Какие edge cases не учёл?

**Шаг 3: Изучи production-grade версию.**
Что добавлено? Зачем эти расширения? Когда их попросят на собеседовании?

**Шаг 4: Объясни решение вслух.**
Представь что объясняешь интервьюеру. Это совсем другой навык, чем просто писать код.

## Принципы

**1. Не пиши код сразу.**
Сначала уточни вопросы. Потом нарисуй структуру (типы, интерфейсы, сигнатуры). Только потом — реализация.

**2. Начни с простого.**
Часто интервьюеры просят "сначала минимальную версию, потом расширим". Не пиши сразу production-grade — начни с MVP за 15 минут, потом улучшай.

**3. Думай вслух.**
Молчаливое программирование интервьюер не оценит. Объясняй что делаешь и почему.

**4. Знай свои паттерны.**
- Worker pool, fan-in/out, pipeline — must know
- Token bucket, leaky bucket — для rate limit
- Mutex vs channel — когда что
- Context propagation для cancellation
- errgroup для координации

**5. Не путай "сложное" с "хорошим".**
Часто простое решение лучше. Senior должен **отвергать** избыточную сложность, а не добавлять её "чтобы выглядело умно".

## Когда что использовать (cheatsheet)

| Задача | Решение |
|---|---|
| Ограничить параллелизм | Worker pool (buffered channel + N workers) |
| Лимитировать частоту | Rate limiter (token bucket) |
| Разделить работу между worker'ами | Fan-out |
| Собрать результаты от worker'ов | Fan-in (через `select` или `errgroup`) |
| Цепочка обработки | Pipeline (stages, каждый = goroutine + channel) |
| Один event → много subscriber'ов | Pub/Sub in-memory |
| Дедупликация concurrent calls | Singleflight |
| Безопасный счётчик | atomic.Int64 |
| Безопасный map | sync.Map (если read-heavy) или mutex+map |
| Координация N goroutines + ошибка | errgroup |
| Wait for completion | sync.WaitGroup |
| Lazy initialization | sync.Once |
| Resource pool | sync.Pool (для аллокаций) или buffered channel |

## Связки с материалами

- [Concurrency и channels](../../09-concurrency-and-performance/01-goroutines-and-channels.md) — теория
- [Sync primitives](../../09-concurrency-and-performance/02-sync-primitives.md) — mutex, atomic, RWMutex
- [Worker pool patterns](../../09-concurrency-and-performance/03-worker-pool.md) — отдельный файл с разными вариантами
- [Context patterns](../../09-concurrency-and-performance/04-context-patterns.md) — context cancellation
- [Background workers](../../04-architecture-and-patterns/patterns/04-background-workers.md) — практические patterns
- [Reliability patterns](../../05-system-design/reliability-patterns/) — rate limit, circuit breaker, retry
