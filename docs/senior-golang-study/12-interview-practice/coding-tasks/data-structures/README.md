# Data Structures Tasks

Задачи на реализацию структур данных, которые дают на собеседованиях. Это не leetcode (вопросы алгоритмов вынесены в [16-algorithms-and-data-structures/](../../../16-algorithms-and-data-structures/)), а **инженерные** реализации — с thread safety, метриками, edge cases.

## Задачи

1. [LRU Cache](./01-lru-cache.md) — Least Recently Used, O(1) get/put через map + doubly linked list
2. [Top-K Elements](./02-top-k.md) — top-K через min-heap, streaming, parallel
3. [Bloom Filter](./03-bloom-filter.md) — probabilistic membership check, false positive rate
4. [Trie](./04-trie.md) — prefix tree для autocomplete, search
5. [Sliding Window Counter](./05-sliding-window-counter.md) — count events за окно времени
6. [External Merge Sort](./06-external-merge-sort.md) — сортировка датасета, не влезающего в память (диск как scratch, k-way merge)

## Какие выбирать темы для подготовки

| Задача | Частота на собесах | Когда спрашивают |
|---|---|---|
| LRU cache | ★★★★★ | Почти всегда. Must know. |
| Top-K | ★★★★ | Когда речь про "топ запросов", "топ покупателей" |
| Bloom filter | ★★★ | На senior — обязательно. Hot tech-companies. |
| Sliding window | ★★★ | Rate limiting, аналитика real-time |
| Trie | ★★ | Реже, но в search/autocomplete контексте |
| External merge sort | ★★★ | «Данные не влезают в RAM» — big data, системные интервью |

## Общие принципы

### O(1) — это главное

Все эти структуры **должны быть O(1)** или O(log N) на основных операциях. Если решение O(N) — это red flag.

| Структура | Get | Put | Delete |
|---|---|---|---|
| LRU cache | O(1) | O(1) | O(1) |
| Top-K (heap) | O(1) peek | O(log K) | O(log K) |
| Bloom filter | O(K) hash funcs | O(K) hash funcs | — |
| Trie | O(L) где L = длина string | O(L) | O(L) |
| Sliding window | O(1) | O(1) | — |

### Thread safety

В production-коде structure обычно используется из нескольких goroutines. Варианты:

- **mutex** — простое решение, OK для большинства случаев
- **RWMutex** — read-heavy workload (LRU чаще читает чем пишет)
- **sharded** — разделить на N independent maps, lock per shard
- **lock-free** — atomic операции, сложно, редко нужно

### Eviction strategies

Для cache structures:
- **LRU** (Least Recently Used) — выбросить давно не используемый
- **LFU** (Least Frequently Used) — выбросить редко используемый
- **FIFO** — выбросить старейший по добавлению
- **TTL-based** — по истечению срока
- **Random** — иногда полезно (защита от cache pollution)

## Связки

- [Algorithms](../../../16-algorithms-and-data-structures/README.md) — теория структур данных (sort, heap, graph)
- [Redis as cache](../../../06-databases/caching/01-redis-as-cache.md) — production cache patterns
- [Reliability patterns](../../../05-system-design/reliability-patterns/) — rate limiting через sliding window
