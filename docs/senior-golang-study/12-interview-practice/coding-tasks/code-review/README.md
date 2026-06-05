# Code Review Tasks

Задачи в формате **"найди проблемы и исправь"** — другой подход чем "напиши с нуля". Кандидату показывают рабочий-на-вид код с **скрытыми** проблемами (deadlock, race condition, leak, неправильная архитектура), и просят:

1. Найти все проблемы
2. Описать что может произойти в каждом случае
3. Переписать правильно

Это часто встречается на собеседованиях в **bigtech** — потому что лучше показывает реальные навыки. Каждый может выучить "как написать worker pool". Не каждый видит почему **этот конкретный** worker pool сломается на проде.

## Задачи

1. [Fetcher с кэшем — bug hunt](./01-fetcher-with-cache.md) — **batch** concurrent fetcher с 12+ проблемами: deadlock, race, panic on nil map, неправильный lifecycle, stampede, отсутствие отмены
2. [Background Task Processor](./02-background-task-processor.md) — **long-running** task pool с retry: WaitGroup race, send on closed channel, infinite retry, no drain mode, no panic recovery, busy loop

## Сравнение задач

| | 01: Fetcher | 02: Task Processor |
|---|---|---|
| Тип pool'а | Batch (process IDs → return) | Long-running (Submit/Stop API) |
| Фокус | Lifecycle close, cache stampede | Retry policy, graceful shutdown |
| Главные баги | nil channels, race on cache | wg.Add race, send to closed, infinite retry |
| Расширения | singleflight + LRU | dead letter, drain mode, separate retry queue |

## Формат

Каждая задача:
1. **Изначальный код** — выглядит plausible, но broken
2. **Симптомы** — что происходит при запуске
3. **Анализ проблем** — каждая по отдельности с rationale
4. **Решение Level 1** — quick fixes чтобы работало
5. **Решение Level 2** — production-grade с обоснованиями
6. **Senior-level расширения** — singleflight, LRU, retry policy, метрики
7. **Тесты** — обязательно `-race`
8. **Чек-лист ответа** — что должен сказать senior за 30 минут

## На что смотрит интервьюер

Это формат для проверки **production thinking**, не "знания паттернов":

- Видит ли кандидат **скрытые баги** (nil channel, nil map, race)?
- Объясняет ли **что именно** произойдёт (deadlock vs panic vs race)?
- Знает ли **стандартные средства защиты** (sync.Mutex, errgroup, singleflight)?
- Думает ли про **lifecycle** (когда close, когда cancel)?
- Учитывает ли **context cancellation** во всех веток?
- Видит ли проблемы **API** (нет ошибок, нет порядка, нет конфига)?

Слабый кандидат говорит "вижу гонку" — и всё.
Сильный — перечисляет 8-12 пунктов с приоритетом (что упадёт сразу, что выстрелит на проде).

## Когда такие задачи встречаются

- Senior+ интервью в bigtech (Google, Yandex, AWS, Stripe)
- Pair programming sessions
- "Pre-screening" в формате take-home: "вот PR, оставь review"
- Internal review challenges (некоторые компании используют для promotions)
