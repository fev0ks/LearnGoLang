# Interview Practice

Раздел для подготовки к техническим собеседованиям: типичные задачи на live-coding, их решения и обсуждения trade-offs.

## Структура

- **coding-tasks/** — задачи на код, которые часто просят на собеседовании
  - **concurrency/** — задачи на горутины, каналы, синхронизацию (самая частая категория для Go)
  - **data-structures/** — LRU cache, top-K (heap), bloom filter, trie, sliding window counter
  - **system-primitives/** — connection pool, retry с backoff, circuit breaker, distributed lock, idempotency
  - **code-review/** — задачи формата "найди баги": broken-looking code, найти все проблемы, переписать
  - **streams/** — обработка потоков: deduplication, batching writer, streaming aggregation, backpressure

- _Behavioral_ _(planned)_ — STAR-кейсы, рассказ о себе, leadership примеры
- _Design drills_ _(planned)_ — system design на 30-45 минут

## Как использовать

Каждая задача оформлена по единому шаблону:
1. **Формулировка** — как могут спросить
2. **Уточняющие вопросы** — что важно уточнить (signal seniority)
3. **Базовое решение** — минимально рабочее
4. **Production-grade решение** — с расширениями
5. **Подводные камни** — типичные ошибки
6. **Расширения** — что ещё могут попросить
7. **Связки** — где ещё в материалах есть детали

Это не leetcode. Это **инженерные задачи** уровня "напишите worker pool" или "реализуйте rate limiter" — то что реально просят senior'ов в bigtech и serious startup'ах.

## Что важно показать на собеседовании

### Технически

- **Уточняющие вопросы** — какие constraints? нагрузка? как используется? Без уточнений идеальное решение не написать.
- **Trade-offs** — у любого решения есть цена. "Использую mutex, потому что N=100 потоков и contention низкий. Если бы было 10к — поменял бы на per-shard locking."
- **Edge cases** — что если ctx отменили? что если producer медленнее consumer'а? что если worker упал?
- **Тесты** — даже упрощённые, показывают что ты думаешь о corner cases.
- **Производительность когда уместно** — не оптимизируй преждевременно, но знай где bottleneck может быть.

### Поведенчески

- **Объяснение хода мыслей** — "сейчас я подумаю про channel buffer size" лучше чем тихое программирование 10 минут.
- **Признание неоднозначности** — "не уверен, лучше mutex или channel здесь" — нормально, обсуждай с интервьюером.
- **Готовность переделать** — если интервьюер указал на проблему, не защищайся, переделай.

## Подборка

- [Effective Go](https://go.dev/doc/effective_go)
- [Go FAQ](https://go.dev/doc/faq)
- [Go Concurrency Patterns (Pike)](https://go.dev/blog/pipelines)
- [System Design Primer](https://github.com/donnemartin/system-design-primer)
- [Google SRE Resources](https://sre.google/resources/)

## Что есть рядом

- [Algorithms And Data Structures](../16-algorithms-and-data-structures/README.md) — leetcode-style алгоритмы (два указателя, бинарный поиск, графы)
- [System Design Interview Cases](../05-system-design/interview-cases/README.md) — большие задачи (URL shortener, chat, ride-sharing)
- [Hands-On Labs](../13-hands-on-labs/README.md) — практические лабы для самостоятельной работы
- [Concurrency and Performance](../01-go-core/concurrency-and-performance/README.md) — глубокая теория concurrency

## Вопросы для самопроверки

- какой production incident ты разберёшь как пример ownership;
- какие 3 архитектурных решения из прошлого опыта ты можешь защитить аргументами;
- где ты ошибся технически и что поменял в подходе после этого;
- какие вопросы ты задашь компании про масштаб, команду и инженерную культуру.
