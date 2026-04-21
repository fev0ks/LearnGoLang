# Map Internals

Внутренности `map` в Go: архитектура до Go 1.24 и новая реализация на Swiss Tables начиная с Go 1.24.

## Материалы

- [01. hmap + bmap (до Go 1.24)](./01-hmap-before-1.24.md) — hmap struct, bmap bucket layout, tophash, lookup, overflow chains, инкрементальная эвакуация
- [02. Swiss Tables (с Go 1.24)](./02-swiss-tables-since-1.24.md) — open addressing, ctrl bytes, matchH2 bitset, tombstones, batch copy рост, directory

## Порядок чтения

1. Начни с `01-hmap-before-1.24.md` — понять, что было до
2. Затем `02-swiss-tables-since-1.24.md` — что заменили и почему

## Вопросы senior-уровня

- как устроен bucket в hmap: сколько слотов, как хранятся ключи и значения
- зачем tophash и почему ключи хранятся отдельно от значений
- как работает инкрементальная эвакуация при росте map
- почему порядок итерации по map случаен
- что изменилось в Go 1.24: Swiss Tables vs hmap
- как ctrl-байт в Swiss Tables позволяет проверить 8 слотов одной операцией
- почему при delete в Swiss Tables нужны tombstones (ctrlDeleted)
- H1 vs H2: как разделяется хэш в Swiss Tables
- почему рост через полное копирование (Swiss Tables) не хуже инкрементальной эвакуации по latency
- что такое directory в Swiss Tables для больших map

## Перекрёстные ссылки

- [Memory Internals: Allocator](../memory-internals/02-allocator.md) — как Go аллоцирует память под map elements
- [Memory Internals: GC](../memory-internals/04-garbage-collector.md) — write barrier при записи в map
- [07. Scheduler](../07-scheduler-and-preemption.md) — GMP, почему concurrent map access не thread-safe
